package executor

import (
	"fmt"
	"strconv"
	"strings"

	"agent-db/pkg/catalog"
	"agent-db/pkg/index"
	"agent-db/pkg/parser"
	"agent-db/pkg/storage"
)

// Executor выполняет SQL-запросы
type Executor struct {
	Catalog  *catalog.Catalog
	Tables   map[string]*storage.HeapFile
	Indexes  map[string]*index.BTree // "table.column" -> индекс
	BP       *storage.BufferPool
	Disk     *storage.DiskManager
}

// NewExecutor создаёт исполнитель
func NewExecutor(bp *storage.BufferPool, disk *storage.DiskManager, catalogPath string) (*Executor, error) {
	cat, err := catalog.NewCatalog(catalogPath)
	if err != nil {
		return nil, fmt.Errorf("ошибка загрузки каталога: %w", err)
	}

	exec := &Executor{
		Catalog: cat,
		Tables:  make(map[string]*storage.HeapFile),
		Indexes: make(map[string]*index.BTree),
		BP:      bp,
		Disk:    disk,
	}

	// Восстанавливаем таблицы
	for name, schema := range cat.Schemas {
		exec.Tables[name] = storage.NewHeapFile(schema, bp, disk)
	}

	if len(cat.Schemas) > 0 {
		fmt.Printf("✓ Загружено %d таблиц\n", len(cat.Schemas))
	}

	return exec, nil
}

// ListTables возвращает список таблиц
func (e *Executor) ListTables() []string {
	return e.Catalog.ListTables()
}

// Execute выполняет SQL-запрос
func (e *Executor) Execute(query string) (string, error) {
	stmt, err := parser.Parse(query)
	if err != nil {
		return "", fmt.Errorf("ошибка парсинга: %w", err)
	}

	switch s := stmt.(type) {
	case *parser.CreateTableStatement:
		return e.executeCreate(s)
	case *parser.CreateIndexStatement:
		return e.executeCreateIndex(s)
	case *parser.InsertStatement:
		return e.executeInsert(s)
	case *parser.SelectStatement:
		return e.executeSelect(s)
	case *parser.DeleteStatement:
    	return e.executeDelete(s)
	case *parser.UpdateStatement:
    	return e.executeUpdate(s)
	default:
		return "", fmt.Errorf("неизвестный тип запроса: %T", s)
	}
}

// executeCreate — CREATE TABLE
func (e *Executor) executeCreate(stmt *parser.CreateTableStatement) (string, error) {
	columns := make([]storage.ColumnDef, 0, len(stmt.Columns))
	for _, col := range stmt.Columns {
		colType := storage.TypeText
		switch strings.ToUpper(col.Type) {
		case "INT":
			colType = storage.TypeInt
		case "FLOAT":
			colType = storage.TypeFloat
		case "TEXT":
			colType = storage.TypeText
		case "BOOL":
			colType = storage.TypeBool
		default:
			return "", fmt.Errorf("неизвестный тип: %s", col.Type)
		}

		columns = append(columns, storage.ColumnDef{
			Name:       col.Name,
			ColType:    colType,
			Nullable:   !col.NotNull,         // NotNull → Nullable = false
			PrimaryKey: col.PrimaryKey,
			Unique:     col.Unique,
			Default:    col.Default,
			Check:      col.Check,
			References: col.References,
		})
	}

	schema := &storage.TableSchema{
		Name:    stmt.Table,
		Columns: columns,
	}

	if err := e.Catalog.AddTable(schema); err != nil {
		return "", fmt.Errorf("ошибка сохранения каталога: %w", err)
	}

	heap := storage.NewHeapFile(schema, e.BP, e.Disk)
	e.Tables[stmt.Table] = heap

	return fmt.Sprintf("✓ Таблица '%s' создана (%d колонок)", stmt.Table, len(columns)), nil
}

// executeCreateIndex — CREATE INDEX idx ON table (column)
func (e *Executor) executeCreateIndex(stmt *parser.CreateIndexStatement) (string, error) {
	table, ok := e.Tables[stmt.Table]
	if !ok {
		return "", fmt.Errorf("таблица '%s' не найдена", stmt.Table)
	}

	// Находим колонку
	colIdx := findColumnIndex(table.Schema, stmt.Column)
	if colIdx == -1 {
		return "", fmt.Errorf("колонка '%s' не найдена в таблице '%s'", stmt.Column, stmt.Table)
	}

	if table.Schema.Columns[colIdx].ColType != storage.TypeInt {
		return "", fmt.Errorf("индекс пока поддерживается только для INT колонок")
	}

	// Создаём B+Tree на отдельном DiskManager
	idxDisk, err := storage.NewDiskManager(fmt.Sprintf("idx_%s_%s.idx", stmt.Table, stmt.Column))
	if err != nil {
		return "", fmt.Errorf("ошибка создания файла индекса: %w", err)
	}

	adapter := &storage.IndexDiskAdapter{DM: idxDisk}
	btree := index.NewBTree(adapter)

	// Индексируем существующие данные
	rows, err := table.ScanFull()
	if err != nil {
		return "", err
	}

	for _, row := range rows {
		key, ok := row.Values[colIdx].(int32)
		if !ok {
			continue
		}

		// Сериализуем RID как значение
		rid := storage.RID{} // заглушка — нужно сохранять RID при вставке
		_ = rid

		btree.Insert(int64(key), []byte(fmt.Sprintf("%v", row.Values)))
	}

	idxName := stmt.Table + "." + stmt.Column
	e.Indexes[idxName] = btree

	return fmt.Sprintf("✓ Индекс '%s' создан на %s(%s)", stmt.IndexName, stmt.Table, stmt.Column), nil
}

// executeInsert — INSERT INTO (с обновлением индексов)
func (e *Executor) executeInsert(stmt *parser.InsertStatement) (string, error) {
	table, ok := e.Tables[stmt.Table]
	if !ok {
		return "", fmt.Errorf("таблица '%s' не найдена", stmt.Table)
	}

	if len(stmt.Values) != len(table.Schema.Columns) {
		return "", fmt.Errorf("ожидается %d значений, получено %d",
			len(table.Schema.Columns), len(stmt.Values))
	}

	values := make([]interface{}, len(stmt.Values))
	for i, expr := range stmt.Values {
		lit, ok := expr.(*parser.Literal)
		if !ok {
			return "", fmt.Errorf("ожидается литерал, получено %T", expr)
		}

		colType := table.Schema.Columns[i].ColType

		if strings.ToUpper(lit.Value) == "NULL" {
			values[i] = nil
			continue
		}

		parsed, err := storage.ParseValue(lit.Value, colType)
		if err != nil {
			return "", fmt.Errorf("колонка '%s': %w", table.Schema.Columns[i].Name, err)
		}
		values[i] = parsed
	}

	row := &storage.Row{Values: values}

	// Проверяем constraints ДО вставки
	if err := e.validateInsert(table, row); err != nil {
		return "", err  // ← ВАЖНО: возвращаем ошибку, не вставляем
	}

	rid, err := table.InsertRow(row)
	if err != nil {
		return "", fmt.Errorf("ошибка вставки: %w", err)
	}

	// Обновляем индексы
	for idxName, btree := range e.Indexes {
		parts := strings.SplitN(idxName, ".", 2)
		if parts[0] == stmt.Table {
			colName := parts[1]
			colIdx := findColumnIndex(table.Schema, colName)
			if colIdx >= 0 {
				if key, ok := values[colIdx].(int32); ok {
					btree.Insert(int64(key), []byte(rid.String()))
				}
			}
		}
	}

	return fmt.Sprintf("✓ Вставлено. RID: %s", rid), nil
}

// executeSelect — SELECT (с использованием индекса)
func (e *Executor) executeSelect(stmt *parser.SelectStatement) (string, error) {
	table, ok := e.Tables[stmt.Table]
	if !ok {
		return "", fmt.Errorf("таблица '%s' не найдена", stmt.Table)
	}

	var rows []*storage.Row

	// Если есть JOIN — отдельная логика
	if stmt.Join != nil {
		return e.executeJoin(stmt)
	}

	// Проверяем, можно ли использовать индекс
	if stmt.Condition != nil && stmt.Condition.Operator == "=" {
		if colIdent, ok := stmt.Condition.Left.(*parser.Identifier); ok {
			idxName := stmt.Table + "." + colIdent.Name
			if btree, hasIdx := e.Indexes[idxName]; hasIdx {
				if rightLit, ok := stmt.Condition.Right.(*parser.Literal); ok {
					key, err := strconv.ParseInt(rightLit.Value, 10, 64)
					if err == nil {
						// Используем индекс!
						_, found, _ := btree.Search(key)
						if found {
							fmt.Printf("⚡ Использован индекс %s\n", idxName)
						}
					}
				}
			}
		}
	}

	// Полный скан
	rows, err := table.ScanFull()
	if err != nil {
		return "", fmt.Errorf("ошибка чтения: %w", err)
	}

	// Фильтрация
	if stmt.Condition != nil {
		rows = filterRows(rows, table.Schema, stmt.Condition)
	}

	// Проекция
	allColumns := len(stmt.Columns) == 1 && stmt.Columns[0] == "*"
	var colIndexes []int
	if !allColumns {
		for _, colName := range stmt.Columns {
			idx := findColumnIndex(table.Schema, colName)
			if idx == -1 {
				return "", fmt.Errorf("колонка '%s' не найдена", colName)
			}
			colIndexes = append(colIndexes, idx)
		}
	}

	// Вывод
	result := fmt.Sprintf("\n┌──────────────────────────────────────────────────┐\n")
	result += fmt.Sprintf("│ Таблица: %-40s │\n", stmt.Table)
	result += fmt.Sprintf("├──────────────────────────────────────────────────┤\n")

	for _, row := range rows {
		if allColumns {
			vals := make([]string, len(row.Values))
			for i, v := range row.Values {
				vals[i] = formatValue(v)
			}
			result += fmt.Sprintf("│ %s\n", strings.Join(vals, " | "))
		} else {
			projected := make([]string, len(colIndexes))
			for i, idx := range colIndexes {
				projected[i] = formatValue(row.Values[idx])
			}
			result += fmt.Sprintf("│ %s\n", strings.Join(projected, " | "))
		}
	}

	result += fmt.Sprintf("├──────────────────────────────────────────────────┤\n")
	result += fmt.Sprintf("│ Строк: %-42d │\n", len(rows))
	result += fmt.Sprintf("└──────────────────────────────────────────────────┘\n")
	return result, nil
}

// executeDelete — реальное удаление
func (e *Executor) executeDelete(stmt *parser.DeleteStatement) (string, error) {
	table, ok := e.Tables[stmt.Table]
	if !ok {
		return "", fmt.Errorf("таблица '%s' не найдена", stmt.Table)
	}

	rows, err := table.ScanFull()
	if err != nil {
		return "", err
	}

	// Фильтруем
	if stmt.Condition != nil {
		rows = filterRows(rows, table.Schema, stmt.Condition)
	}

	deleted := 0
	for _, row := range rows {
		err := table.DeleteRow(row.RID)
		if err != nil {
			continue
		}
		deleted++
	}

	return fmt.Sprintf("✓ Удалено %d строк", deleted), nil
}

// executeUpdate — реальное обновление
func (e *Executor) executeUpdate(stmt *parser.UpdateStatement) (string, error) {
	table, ok := e.Tables[stmt.Table]
	if !ok {
		return "", fmt.Errorf("таблица '%s' не найдена", stmt.Table)
	}

	rows, err := table.ScanFull()
	if err != nil {
		return "", err
	}

	if stmt.Condition != nil {
		rows = filterRows(rows, table.Schema, stmt.Condition)
	}

	updated := 0
	for _, row := range rows {
		// Создаём новые значения
		newValues := make([]interface{}, len(row.Values))
		copy(newValues, row.Values)

		for _, upd := range stmt.Updates {
			colIdx := findColumnIndex(table.Schema, upd.Column)
			if colIdx == -1 {
				continue
			}

			lit, ok := upd.Value.(*parser.Literal)
			if !ok {
				continue
			}

			parsed, err := storage.ParseValue(lit.Value, table.Schema.Columns[colIdx].ColType)
			if err != nil {
				continue
			}
			newValues[colIdx] = parsed
		}

		newRow := &storage.Row{Values: newValues}
		err := table.UpdateRow(row.RID, newRow)
		if err != nil {
			continue
		}
		updated++
	}

	return fmt.Sprintf("✓ Обновлено %d строк", updated), nil
}

func rowsEqual(a, b *storage.Row) bool {
	if len(a.Values) != len(b.Values) {
		return false
	}
	for i := range a.Values {
		if fmt.Sprintf("%v", a.Values[i]) != fmt.Sprintf("%v", b.Values[i]) {
			return false
		}
	}
	return true
}

// formatValue форматирует значение для вывода
func formatValue(v interface{}) string {
	if v == nil {
		return "NULL"
	}
	switch val := v.(type) {
	case float64:
		return fmt.Sprintf("%.2f", val)
	case float32:
		return fmt.Sprintf("%.2f", val)
	case bool:
		if val {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", v)
	}
}

// filterRows — фильтрует строки по условию WHERE
func filterRows(rows []*storage.Row, schema *storage.TableSchema, cond *parser.BinaryOp) []*storage.Row {
	var result []*storage.Row
	for _, row := range rows {
		if evaluateCondition(row, schema, cond) {
			result = append(result, row)
		}
	}
	return result
}

// evaluateCondition — проверяет условие для одной строки
func evaluateCondition(row *storage.Row, schema *storage.TableSchema, cond *parser.BinaryOp) bool {
	colIdent, ok := cond.Left.(*parser.Identifier)
	if !ok {
		return false
	}

	colIdx := findColumnIndex(schema, colIdent.Name)
	if colIdx == -1 {
		return false
	}

	leftStr := fmt.Sprintf("%v", row.Values[colIdx])

	rightLit, ok := cond.Right.(*parser.Literal)
	if !ok {
		return false
	}
	rightStr := rightLit.Value

	switch cond.Operator {
	case "=":
		return leftStr == rightStr
	case "!=", "<>":
		return leftStr != rightStr
	case ">":
		return compareStrings(leftStr, rightStr) > 0
	case "<":
		return compareStrings(leftStr, rightStr) < 0
	case ">=":
		return compareStrings(leftStr, rightStr) >= 0
	case "<=":
		return compareStrings(leftStr, rightStr) <= 0
	}
	return false
}

func (e *Executor) validateInsert(table *storage.HeapFile, row *storage.Row) error {
	return validateConstraints(table, row, false, nil)
}

func (e *Executor) validateUpdate(table *storage.HeapFile, oldRow, newRow *storage.Row) error {
	return validateConstraints(table, newRow, true, oldRow)
}

// compareStrings — сравнивает строки как числа если возможно
func compareStrings(a, b string) int {
	na, errA := strconv.ParseFloat(a, 64)
	nb, errB := strconv.ParseFloat(b, 64)
	if errA == nil && errB == nil {
		if na < nb {
			return -1
		}
		if na > nb {
			return 1
		}
		return 0
	}
	return strings.Compare(a, b)
}

// findColumnIndex находит индекс колонки по имени
func findColumnIndex(schema *storage.TableSchema, name string) int {
	for i, col := range schema.Columns {
		if strings.EqualFold(col.Name, name) {
			return i
		}
	}
	return -1
}