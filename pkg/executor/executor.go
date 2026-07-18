package executor

import (
	"agent-db/pkg/catalog"
	"agent-db/pkg/index"
	"agent-db/pkg/parser"
	"agent-db/pkg/storage"
	"agent-db/pkg/utils"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Executor выполняет SQL-запросы
type Executor struct {
	Catalog       *catalog.Catalog
	Tables        map[string]*storage.HeapFile
	Indexes       map[string]*index.BTree
	BP            *storage.BufferPool
	Disk          *storage.DiskManager
	inTransaction bool
	txQueries     []string
	txResults     []*QueryResult
	txMu          sync.Mutex
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

// ========== ТРАНЗАКЦИИ ==========

func (e *Executor) BeginTransaction() error {
	e.txMu.Lock()
	defer e.txMu.Unlock()

	if e.inTransaction {
		return fmt.Errorf("транзакция уже активна")
	}

	e.inTransaction = true
	e.txQueries = []string{}
	e.txResults = []*QueryResult{}
	log.Printf("[DEBUG] Transaction started")
	return nil
}

func (e *Executor) Commit() error {
	log.Printf("[DEBUG] Commit: called")

	e.txMu.Lock()
	defer e.txMu.Unlock()

	if !e.inTransaction {
		return fmt.Errorf("нет активной транзакции")
	}

	log.Printf("[DEBUG] Commit: %d запросов в транзакции", len(e.txQueries))

	for i, query := range e.txQueries {
		log.Printf("[DEBUG] Commit: executing query %d: %s", i, query)

		result, err := e.Execute(query)
		if err != nil {
			log.Printf("[ERROR] Commit: query %d error: %v", i, err)
			e.inTransaction = false
			e.txQueries = nil
			e.txResults = nil
			return fmt.Errorf("ошибка выполнения транзакции на запросе %d: %w", i, err)
		}
		e.txResults = append(e.txResults, result)
	}

	// ✅ СБРОС НА ДИСК — ключевое исправление
	if err := e.BP.FlushAll(); err != nil {
		log.Printf("[ERROR] Commit: flush error: %v", err)
		e.inTransaction = false
		e.txQueries = nil
		e.txResults = nil
		return fmt.Errorf("ошибка сброса на диск: %w", err)
	}

	e.inTransaction = false
	e.txQueries = nil

	log.Printf("[DEBUG] Commit: success, flushed to disk")
	return nil
}

func (e *Executor) Rollback() error {
	e.txMu.Lock()
	defer e.txMu.Unlock()

	if !e.inTransaction {
		return fmt.Errorf("нет активной транзакции")
	}

	e.inTransaction = false
	e.txQueries = nil
	e.txResults = nil
	log.Printf("[DEBUG] Transaction rolled back")
	return nil
}

// ExecuteInTx — добавляет запрос в транзакцию (для INSERT/UPDATE/DELETE)
func (e *Executor) ExecuteInTx(query string) (*QueryResult, error) {
	log.Printf("[DEBUG] ExecuteInTx: adding query: %s", query)

	e.txMu.Lock()
	defer e.txMu.Unlock()

	if !e.inTransaction {
		return nil, fmt.Errorf("нет активной транзакции")
	}

	// Добавляем запрос в очередь
	e.txQueries = append(e.txQueries, query)
	log.Printf("[DEBUG] ExecuteInTx: query added, total: %d", len(e.txQueries))

	return &QueryResult{
		Type: "PENDING",
	}, nil
}

// ExecuteInTxWithResult — выполняет запрос СРАЗУ внутри транзакции (для SELECT)
func (e *Executor) ExecuteInTxWithResult(query string) (*QueryResult, error) {
	log.Printf("[DEBUG] ExecuteInTxWithResult: executing: %s", query)

	e.txMu.Lock()
	defer e.txMu.Unlock()

	if !e.inTransaction {
		return nil, fmt.Errorf("нет активной транзакции")
	}

	// ✅ Выполняем запрос сразу
	result, err := e.Execute(query)
	if err != nil {
		return nil, err
	}

	// Добавляем в историю (но не будем выполнять повторно при коммите)
	e.txQueries = append(e.txQueries, query)
	e.txResults = append(e.txResults, result)

	return result, nil
}

//===========================

// ListTables возвращает список таблиц
func (e *Executor) ListTables() []string {
	return e.Catalog.ListTables()
}

func (e *Executor) Execute(query string) (result *QueryResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("[PANIC] Execute recovered: %v", r)
			result = &QueryResult{
				Type:  "ERROR",
				Error: fmt.Sprintf("внутренняя ошибка: %v", r),
			}
			err = nil
		}
	}()

	fmt.Printf("[DEBUG] Executing SQL: %s", query)

	stmt, err := parser.Parse(query)
	if err != nil {
		return &QueryResult{
			Type:  "ERROR",
			Error: fmt.Sprintf("ошибка парсинга: %v", err),
		}, nil
	}

	// ✅ Убрали return из switch, сохраняем в переменные
	switch s := stmt.(type) {
	case *parser.CreateTableStatement:
		result, err = e.executeCreate(s)
	case *parser.CreateIndexStatement:
		result, err = e.executeCreateIndex(s)
	case *parser.InsertStatement:
		result, err = e.executeInsert(s)
	case *parser.SelectStatement:
		return e.executeSelect(s)
	case *parser.DeleteStatement:
		result, err = e.executeDelete(s)
	case *parser.UpdateStatement:
		result, err = e.executeUpdate(s)
	case *parser.DropTableStatement:
		result, err = e.executeDrop(s)
	default:
		return &QueryResult{
			Type:  "ERROR",
			Error: fmt.Sprintf("неизвестный тип запроса: %T", s),
		}, nil
	}

	// ✅ Теперь этот код ВЫПОЛНЯЕТСЯ
	if !e.inTransaction && result != nil && result.Type != "ERROR" {
		switch stmt.(type) {
		case *parser.CreateTableStatement, *parser.CreateIndexStatement,
			*parser.InsertStatement, *parser.DeleteStatement,
			*parser.UpdateStatement, *parser.DropTableStatement:
			if flushErr := e.BP.FlushAll(); flushErr != nil {
				log.Printf("[ERROR] Execute: auto-flush failed: %v", flushErr)
			} else {
				log.Printf("[DEBUG] Execute: auto-flush OK")
			}
		}
	}

	return result, err
}

// executeCreate — CREATE TABLE
func (e *Executor) executeCreate(stmt *parser.CreateTableStatement) (*QueryResult, error) {
	fmt.Printf("[DEBUG] Creating table %s", stmt.Table)
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
			return &QueryResult{
				Type:  "ERROR",
				Error: fmt.Sprintf("неизвестный тип: %s", col.Type),
			}, nil
		}

		columns = append(columns, storage.ColumnDef{
			Name:          col.Name,
			ColType:       colType,
			Nullable:      !col.NotNull,
			PrimaryKey:    col.PrimaryKey,
			AutoIncrement: col.AutoIncrement,
			Unique:        col.Unique,
			Default:       col.Default,
			Check:         col.Check,
			References:    col.References,
		})
	}

	schema := &storage.TableSchema{
		Name:    stmt.Table,
		Columns: columns,
	}

	if err := e.Catalog.AddTable(schema); err != nil {
		return &QueryResult{
			Type:  "ERROR",
			Error: fmt.Sprintf("ошибка сохранения каталога: %v", err),
		}, nil
	}

	heap := storage.NewHeapFile(schema, e.BP, e.Disk)
	if heap == nil {
		fmt.Printf("[ERROR] NewHeapFile returned nil for table %s", stmt.Table)
		return &QueryResult{
			Type:  "ERROR",
			Error: fmt.Sprintf("не удалось создать файл таблицы %s", stmt.Table),
		}, nil
	}
	e.Tables[stmt.Table] = heap

	return &QueryResult{
		Type:         "CREATE",
		AffectedRows: 1,
		Columns:      []string{"message"},
		Rows:         [][]interface{}{{fmt.Sprintf("✓ Таблица '%s' создана (%d колонок)", stmt.Table, len(columns))}},
	}, nil
}

// executeCreateIndex — CREATE INDEX
func (e *Executor) executeCreateIndex(stmt *parser.CreateIndexStatement) (*QueryResult, error) {
	table, ok := e.Tables[stmt.Table]
	if !ok {
		return &QueryResult{
			Type:  "ERROR",
			Error: fmt.Sprintf("таблица '%s' не найдена", stmt.Table),
		}, nil
	}

	colIdx := findColumnIndex(table.Schema, stmt.Column)
	if colIdx == -1 {
		return &QueryResult{
			Type:  "ERROR",
			Error: fmt.Sprintf("колонка '%s' не найдена в таблице '%s'", stmt.Column, stmt.Table),
		}, nil
	}

	if table.Schema.Columns[colIdx].ColType != storage.TypeInt {
		return &QueryResult{
			Type:  "ERROR",
			Error: "индекс пока поддерживается только для INT колонок",
		}, nil
	}

	idxDisk, err := storage.NewDiskManager(fmt.Sprintf("idx_%s_%s.idx", stmt.Table, stmt.Column))
	if err != nil {
		return &QueryResult{
			Type:  "ERROR",
			Error: fmt.Sprintf("ошибка создания файла индекса: %v", err),
		}, nil
	}

	adapter := &storage.IndexDiskAdapter{DM: idxDisk}
	btree := index.NewBTree(adapter)

	rows, err := table.ScanFull()
	if err != nil {
		return &QueryResult{
			Type:  "ERROR",
			Error: err.Error(),
		}, nil
	}

	for _, row := range rows {
		key, ok := row.Values[colIdx].(int32)
		if !ok {
			continue
		}
		btree.Insert(int64(key), []byte(fmt.Sprintf("%v", row.Values)))
	}

	idxName := stmt.Table + "." + stmt.Column
	e.Indexes[idxName] = btree

	return &QueryResult{
		Type:         "CREATE_INDEX",
		AffectedRows: 1,
		Columns:      []string{"message"},
		Rows:         [][]interface{}{{fmt.Sprintf("✓ Индекс '%s' создан на %s(%s)", stmt.IndexName, stmt.Table, stmt.Column)}},
	}, nil
}

// executeInsert — INSERT INTO
func (e *Executor) executeInsert(stmt *parser.InsertStatement) (*QueryResult, error) {
	table, ok := e.Tables[stmt.Table]
	if !ok {
		return &QueryResult{
			Type:  "ERROR",
			Error: fmt.Sprintf("таблица '%s' не найдена", stmt.Table),
		}, nil
	}

	var values []interface{}
	var lastInsertID int64 = -1

	// Если колонки указаны явно
	if len(stmt.Columns) > 0 {
		if len(stmt.Columns) != len(stmt.Values) {
			return &QueryResult{
				Type:  "ERROR",
				Error: fmt.Sprintf("ожидается %d значений для %d колонок, получено %d", len(stmt.Columns), len(stmt.Columns), len(stmt.Values)),
			}, nil
		}

		values = make([]interface{}, len(table.Schema.Columns))
		for i := range values {
			values[i] = nil
		}

		// Обрабатываем автоинкремент
		for i, col := range table.Schema.Columns {
			if col.AutoIncrement {
				nextID := utils.GenerateID()
				values[i] = int32(nextID)
				lastInsertID = int64(nextID)
			}
		}

		for i, colName := range stmt.Columns {
			idx := findColumnIndex(table.Schema, colName)
			if idx == -1 {
				return &QueryResult{
					Type:  "ERROR",
					Error: fmt.Sprintf("колонка '%s' не найдена в таблице '%s'", colName, stmt.Table),
				}, nil
			}

			lit, ok := stmt.Values[i].(*parser.Literal)
			if !ok {
				return &QueryResult{
					Type:  "ERROR",
					Error: "ожидается литерал",
				}, nil
			}

			val := lit.Value
			if (strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'")) ||
				(strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"")) {
				val = val[1 : len(val)-1]
			}

			if strings.ToUpper(val) == "NULL" {
				values[idx] = nil
				continue
			}

			parsed, err := storage.ParseValue(val, table.Schema.Columns[idx].ColType)
			if err != nil {
				return &QueryResult{
					Type:  "ERROR",
					Error: fmt.Sprintf("колонка '%s': %v", colName, err),
				}, nil
			}
			values[idx] = parsed
		}
	} else {
		// Все колонки подряд
		if len(stmt.Values) != len(table.Schema.Columns) {
			return &QueryResult{
				Type:  "ERROR",
				Error: fmt.Sprintf("ожидается %d значений, получено %d", len(table.Schema.Columns), len(stmt.Values)),
			}, nil
		}

		values = make([]interface{}, len(stmt.Values))
		for i, expr := range stmt.Values {
			lit, ok := expr.(*parser.Literal)
			if !ok {
				return &QueryResult{
					Type:  "ERROR",
					Error: "ожидается литерал",
				}, nil
			}

			val := lit.Value
			if (strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'")) ||
				(strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"")) {
				val = val[1 : len(val)-1]
			}

			if strings.ToUpper(val) == "NULL" {
				values[i] = nil
				continue
			}

			parsed, err := storage.ParseValue(val, table.Schema.Columns[i].ColType)
			if err != nil {
				return &QueryResult{
					Type:  "ERROR",
					Error: fmt.Sprintf("колонка '%s': %v", table.Schema.Columns[i].Name, err),
				}, nil
			}
			values[i] = parsed
		}
	}

	// ===== Проверка уникальности (PRIMARY KEY / UNIQUE) =====
	for idx, col := range table.Schema.Columns {
		if col.PrimaryKey || col.Unique {
			existingRows, err := table.ScanFull()
			if err != nil {
				return &QueryResult{
					Type:  "ERROR",
					Error: fmt.Sprintf("ошибка проверки уникальности: %v", err),
				}, nil
			}
			for _, row := range existingRows {
				if row.Values[idx] != nil && values[idx] != nil &&
					fmt.Sprintf("%v", row.Values[idx]) == fmt.Sprintf("%v", values[idx]) {
					return &QueryResult{
						Type:  "ERROR",
						Error: fmt.Sprintf("дублирующее значение для колонки '%s'", col.Name),
					}, nil
				}
			}
		}
	}

	row := &storage.Row{Values: values}
	rid, err := table.InsertRow(row)
	if err != nil {
		return &QueryResult{
			Type:  "ERROR",
			Error: fmt.Sprintf("ошибка вставки: %v", err),
		}, nil
	}

	return &QueryResult{
		Type:         "INSERT",
		AffectedRows: 1,
		LastInsertID: lastInsertID,
		Columns:      []string{"rid", "message"},
		Rows:         [][]interface{}{{fmt.Sprintf("%s", rid), "✓ Вставлено"}},
	}, nil
}

// executeSelect — SELECT
func (e *Executor) executeSelect(stmt *parser.SelectStatement) (*QueryResult, error) {
	table, ok := e.Tables[stmt.Table]
	if !ok {
		return &QueryResult{
			Type:  "ERROR",
			Error: fmt.Sprintf("таблица '%s' не найдена", stmt.Table),
		}, nil
	}

	var rows []*storage.Row
	var err error

	// Если есть JOIN
	if stmt.Join != nil {
		return e.executeJoin(stmt)
	}

	// Полный скан
	rows, err = table.ScanFull()
	if err != nil {
		return &QueryResult{
			Type:  "ERROR",
			Error: fmt.Sprintf("ошибка чтения: %v", err),
		}, nil
	}

	// Фильтрация
	if stmt.Condition != nil {
		rows = filterRows(rows, table.Schema, stmt.Condition)
	}

	// Агрегаты
	if len(stmt.Aggregates) > 0 {
		aggResult := computeAggregates(rows, table.Schema, stmt.Aggregates)
		resultRows := make([][]interface{}, 0, len(aggResult))
		resultCols := make([]string, 0, len(aggResult))
		for key, val := range aggResult {
			resultCols = append(resultCols, key)
			resultRows = append(resultRows, []interface{}{val})
		}
		return &QueryResult{
			Type:    "SELECT",
			Columns: resultCols,
			Rows:    resultRows,
		}, nil
	}

	// ORDER BY
	if stmt.OrderBy != "" {
		applyOrderBy(rows, table.Schema, stmt.OrderBy, stmt.OrderDir)
	}

	// LIMIT / OFFSET
	if stmt.Limit >= 0 || stmt.Offset > 0 {
		rows = applyLimitOffset(rows, stmt.Limit, stmt.Offset)
	}

	// Проекция
	allColumns := len(stmt.Columns) == 1 && stmt.Columns[0] == "*"
	var colIndexes []int
	var colNames []string

	if !allColumns {
		for _, colName := range stmt.Columns {
			idx := findColumnIndex(table.Schema, colName)
			if idx == -1 {
				return &QueryResult{
					Type:  "ERROR",
					Error: fmt.Sprintf("колонка '%s' не найдена", colName),
				}, nil
			}
			colIndexes = append(colIndexes, idx)
			colNames = append(colNames, colName)
		}
	} else {
		for i, col := range table.Schema.Columns {
			colIndexes = append(colIndexes, i)
			colNames = append(colNames, col.Name)
		}
	}

	// Формируем результат
	resultRows := make([][]interface{}, len(rows))
	for i, row := range rows {
		resultRow := make([]interface{}, len(colIndexes))
		for j, idx := range colIndexes {
			resultRow[j] = row.Values[idx]
		}
		resultRows[i] = resultRow
	}

	return &QueryResult{
		Type:    "SELECT",
		Columns: colNames,
		Rows:    resultRows,
	}, nil
}

// executeDelete — DELETE
func (e *Executor) executeDelete(stmt *parser.DeleteStatement) (*QueryResult, error) {
	table, ok := e.Tables[stmt.Table]
	if !ok {
		return &QueryResult{
			Type:  "ERROR",
			Error: fmt.Sprintf("таблица '%s' не найдена", stmt.Table),
		}, nil
	}

	rows, err := table.ScanFull()
	if err != nil {
		return &QueryResult{
			Type:  "ERROR",
			Error: err.Error(),
		}, nil
	}

	if stmt.Condition != nil {
		rows = filterRows(rows, table.Schema, stmt.Condition)
	}

	deleted := 0
	for _, row := range rows {
		if err := table.DeleteRow(row.RID); err == nil {
			deleted++
		}
	}

	return &QueryResult{
		Type:         "DELETE",
		AffectedRows: int64(deleted),
		Columns:      []string{"message"},
		Rows:         [][]interface{}{{fmt.Sprintf("✓ Удалено %d строк", deleted)}},
	}, nil
}

// executeDrop — DROP TABLE
func (e *Executor) executeDrop(stmt *parser.DropTableStatement) (*QueryResult, error) {
	tableName := stmt.Table

	// Проверяем существование
	table, ok := e.Tables[tableName]
	if !ok {
		if stmt.IfExists {
			return &QueryResult{
				Type:    "DROP",
				Columns: []string{"message"},
				Rows:    [][]interface{}{{fmt.Sprintf("✓ Таблица '%s' не существует, пропущено", tableName)}},
			}, nil
		}
		return &QueryResult{
			Type:  "ERROR",
			Error: fmt.Sprintf("таблица '%s' не найдена", tableName),
		}, nil
	}

	// 1. Получаем TableID
	tableID := table.TableID()

	// 2. Очищаем страницы таблицы
	if err := e.clearPagesByTableID(tableID, e.Disk, e.BP); err != nil {
		return &QueryResult{
			Type:  "ERROR",
			Error: fmt.Sprintf("ошибка очистки страниц: %v", err),
		}, nil
	}

	// 3. Удаляем из каталога
	if err := e.Catalog.RemoveTable(tableName); err != nil {
		return &QueryResult{
			Type:  "ERROR",
			Error: fmt.Sprintf("ошибка удаления из каталога: %v", err),
		}, nil
	}

	// 4. Удаляем из Executor.Tables
	delete(e.Tables, tableName)

	// 5. Удаляем связанные индексы
	for idxName := range e.Indexes {
		if strings.HasPrefix(idxName, tableName+".") {
			delete(e.Indexes, idxName)
		}
	}

	// 6. Сбрасываем на диск
	e.BP.FlushAll()

	return &QueryResult{
		Type:    "DROP",
		Columns: []string{"message"},
		Rows:    [][]interface{}{{fmt.Sprintf("✓ Таблица '%s' удалена", tableName)}},
	}, nil
}

// clearPagesByTableID очищает все страницы с указанным TableID
func (e *Executor) clearPagesByTableID(tableID uint32, disk *storage.DiskManager, bp *storage.BufferPool) error {
	var pagesToClean []uint64
	totalPages := disk.PageCount() // используем новый метод

	for pid := uint64(1); pid < totalPages; pid++ {
		page, err := bp.FetchPage(pid)
		if err != nil {
			continue
		}
		header := page.ReadHeader()
		if header.TableID == tableID {
			pagesToClean = append(pagesToClean, pid)
		}
		bp.UnpinPage(pid, false)
	}

	for _, pid := range pagesToClean {
		page, err := bp.FetchPage(pid)
		if err != nil {
			return err
		}
		// Создаём новую пустую страницу с тем же ID
		newPage := storage.NewPage(pid)
		copy(page.Data[:], newPage.Data[:])
		page.Dirty = true
		bp.UnpinPage(pid, true)
	}
	return nil
}

// executeUpdate — UPDATE
func (e *Executor) executeUpdate(stmt *parser.UpdateStatement) (*QueryResult, error) {
	table, ok := e.Tables[stmt.Table]
	if !ok {
		return &QueryResult{
			Type:  "ERROR",
			Error: fmt.Sprintf("таблица '%s' не найдена", stmt.Table),
		}, nil
	}

	rows, err := table.ScanFull()
	if err != nil {
		return &QueryResult{
			Type:  "ERROR",
			Error: err.Error(),
		}, nil
	}

	if stmt.Condition != nil {
		rows = filterRows(rows, table.Schema, stmt.Condition)
	}

	updated := 0
	for _, row := range rows {
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

		// ===== Проверка уникальности для обновляемых колонок =====
		for _, upd := range stmt.Updates {
			colIdx := findColumnIndex(table.Schema, upd.Column)
			if colIdx == -1 {
				continue
			}
			col := table.Schema.Columns[colIdx]
			if col.PrimaryKey || col.Unique {
				existingRows, _ := table.ScanFull()
				for _, existingRow := range existingRows {
					if existingRow.RID == row.RID {
						continue
					}
					if existingRow.Values[colIdx] != nil && newValues[colIdx] != nil &&
						fmt.Sprintf("%v", existingRow.Values[colIdx]) == fmt.Sprintf("%v", newValues[colIdx]) {
						return &QueryResult{
							Type:  "ERROR",
							Error: fmt.Sprintf("дублирующее значение для колонки '%s'", col.Name),
						}, nil
					}
				}
			}
		}

		newRow := &storage.Row{Values: newValues}
		if err := table.UpdateRow(row.RID, newRow); err == nil {
			updated++
		}
	}

	return &QueryResult{
		Type:         "UPDATE",
		AffectedRows: int64(updated),
		Columns:      []string{"message"},
		Rows:         [][]interface{}{{fmt.Sprintf("✓ Обновлено %d строк", updated)}},
	}, nil
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
	case "LIKE": // ← добавляем
		return strings.Contains(leftStr, rightStr)
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

func applyOrderBy(rows []*storage.Row, schema *storage.TableSchema, orderBy string, orderDir string) {
	colIdx := findColumnIndex(schema, orderBy)
	if colIdx == -1 {
		return
	}

	sort.Slice(rows, func(i, j int) bool {
		a := fmt.Sprintf("%v", rows[i].Values[colIdx])
		b := fmt.Sprintf("%v", rows[j].Values[colIdx])

		// Пробуем как числа
		na, errA := strconv.ParseFloat(a, 64)
		nb, errB := strconv.ParseFloat(b, 64)

		if errA == nil && errB == nil {
			if orderDir == "DESC" {
				return na > nb
			}
			return na < nb
		}

		// Как строки
		if orderDir == "DESC" {
			return a > b
		}
		return a < b
	})
}

// applyLimitOffset — обрезает строки
func applyLimitOffset(rows []*storage.Row, limit int, offset int) []*storage.Row {
	if offset >= len(rows) {
		return []*storage.Row{}
	}

	end := len(rows)
	if offset > 0 {
		rows = rows[offset:]
		end = len(rows)
	}

	if limit >= 0 && limit < end {
		end = limit
	}

	return rows[:end]
}

func computeAggregates(rows []*storage.Row, schema *storage.TableSchema, aggs []parser.Aggregate) map[string]interface{} {
	result := make(map[string]interface{})

	for _, agg := range aggs {
		key := agg.Func + "(" + agg.Column + ")"

		// COUNT(*) — особый случай (не требует колонки)
		if strings.ToUpper(agg.Func) == "COUNT" && agg.Column == "*" {
			result[key] = int64(len(rows))
			continue
		}

		colIdx := findColumnIndex(schema, agg.Column)
		if colIdx == -1 {
			result[key] = fmt.Sprintf("ОШИБКА: колонка '%s' не найдена", agg.Column)
			continue
		}

		switch strings.ToUpper(agg.Func) {
		case "COUNT":
			var count int64
			for _, row := range rows {
				if colIdx < len(row.Values) && row.Values[colIdx] != nil {
					count++
				}
			}
			result[key] = count

		case "SUM":
			var sum float64
			for _, row := range rows {
				if colIdx < len(row.Values) && row.Values[colIdx] != nil {
					sum += toFloat64(row.Values[colIdx])
				}
			}
			result[key] = sum

		case "AVG":
			var sum float64
			var count int64
			for _, row := range rows {
				if colIdx < len(row.Values) && row.Values[colIdx] != nil {
					sum += toFloat64(row.Values[colIdx])
					count++
				}
			}
			if count > 0 {
				result[key] = sum / float64(count)
			} else {
				result[key] = 0.0
			}

		case "MIN":
			var min interface{}
			for _, row := range rows {
				if colIdx < len(row.Values) && row.Values[colIdx] != nil {
					val := row.Values[colIdx]
					if min == nil {
						min = val
					} else if toFloat64(val) < toFloat64(min) {
						min = val
					}
				}
			}
			if min != nil {
				result[key] = min
			} else {
				result[key] = nil
			}

		case "MAX":
			var max interface{}
			for _, row := range rows {
				if colIdx < len(row.Values) && row.Values[colIdx] != nil {
					val := row.Values[colIdx]
					if max == nil {
						max = val
					} else if toFloat64(val) > toFloat64(max) {
						max = val
					}
				}
			}
			if max != nil {
				result[key] = max
			} else {
				result[key] = nil
			}
		}
	}

	return result
}

func toFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case int:
		return float64(val)
	case int32:
		return float64(val)
	case int64:
		return float64(val)
	case float32:
		return float64(val)
	case float64:
		return val
	default:
		return 0
	}
}

func compareValues(a, b interface{}) int {
	na, nb := toFloat64(a), toFloat64(b)
	if na < nb {
		return -1
	}
	if na > nb {
		return 1
	}
	return 0
}
