package executor

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"agent-db/pkg/parser"
	"agent-db/pkg/storage"
)

type JoinedRow struct {
	LeftValues   []interface{}
	LeftColumns  []string
	RightValues  []interface{}
	RightColumns []string
	LeftName     string
	RightName    string
}

func (e *Executor) executeJoin(stmt *parser.SelectStatement) (*QueryResult, error) {
	leftTable, ok := e.Tables[stmt.Table]
	if !ok {
		return &QueryResult{
			Type:  "ERROR",
			Error: fmt.Sprintf("таблица '%s' не найдена", stmt.Table),
		}, nil
	}

	rightTable, ok := e.Tables[stmt.Join.Table]
	if !ok {
		return &QueryResult{
			Type:  "ERROR",
			Error: fmt.Sprintf("таблица '%s' не найдена", stmt.Join.Table),
		}, nil
	}

	leftRows, err := leftTable.ScanFull()
	if err != nil {
		return &QueryResult{
			Type:  "ERROR",
			Error: fmt.Sprintf("ошибка чтения левой таблицы: %v", err),
		}, nil
	}

	rightRows, err := rightTable.ScanFull()
	if err != nil {
		return &QueryResult{
			Type:  "ERROR",
			Error: fmt.Sprintf("ошибка чтения правой таблицы: %v", err),
		}, nil
	}

	var joined []JoinedRow

	for _, leftRow := range leftRows {
		matched := false

		for _, rightRow := range rightRows {
			if evaluateJoinCondition(leftRow, rightRow, leftTable.Schema, rightTable.Schema, stmt.Join.Condition) {
				joined = append(joined, JoinedRow{
					LeftValues:   leftRow.Values,
					LeftColumns:  columnNames(leftTable.Schema),
					RightValues:  rightRow.Values,
					RightColumns: columnNames(rightTable.Schema),
					LeftName:     stmt.Table,
					RightName:    stmt.Join.Table,
				})
				matched = true
			}
		}

		if !matched && stmt.Join.Type == parser.LeftJoin {
			nullRight := make([]interface{}, len(rightTable.Schema.Columns))
			for i := range nullRight {
				nullRight[i] = nil
			}
			joined = append(joined, JoinedRow{
				LeftValues:   leftRow.Values,
				LeftColumns:  columnNames(leftTable.Schema),
				RightValues:  nullRight,
				RightColumns: columnNames(rightTable.Schema),
				LeftName:     stmt.Table,
				RightName:    stmt.Join.Table,
			})
		}
	}

	if stmt.Condition != nil {
		joined = filterJoinedRows(joined, stmt.Condition)
	}

	if stmt.OrderBy != "" {
		sortJoinedRows(joined, stmt.OrderBy, stmt.OrderDir)
	}

	if stmt.Limit >= 0 || stmt.Offset > 0 {
		joined = limitJoinedRows(joined, stmt.Limit, stmt.Offset)
	}

	// Формируем результат
	allColumns := len(stmt.Columns) == 1 && stmt.Columns[0] == "*"
	var colNames []string

	if allColumns {
		// Все колонки из обеих таблиц
		for _, col := range leftTable.Schema.Columns {
			colNames = append(colNames, stmt.Table+"."+col.Name)
		}
		for _, col := range rightTable.Schema.Columns {
			colNames = append(colNames, stmt.Join.Table+"."+col.Name)
		}
	} else {
		colNames = stmt.Columns
	}

	resultRows := make([][]interface{}, len(joined))
	for i, row := range joined {
		resultRow := make([]interface{}, len(colNames))
		for j, colName := range colNames {
			resultRow[j] = getJoinValue(row, &parser.Identifier{Name: colName})
		}
		resultRows[i] = resultRow
	}

	return &QueryResult{
		Type:    "SELECT",
		Columns: colNames,
		Rows:    resultRows,
	}, nil
}

func columnNames(schema *storage.TableSchema) []string {
	names := make([]string, len(schema.Columns))
	for i, col := range schema.Columns {
		names[i] = col.Name
	}
	return names
}

func evaluateJoinCondition(
	leftRow *storage.Row, rightRow *storage.Row,
	leftSchema *storage.TableSchema, rightSchema *storage.TableSchema,
	cond *parser.BinaryOp,
) bool {
	if cond == nil {
		return true
	}

	leftCol, ok1 := cond.Left.(*parser.Identifier)
	rightCol, ok2 := cond.Right.(*parser.Identifier)
	if !ok1 || !ok2 {
		return false
	}

	leftColName := stripTable(leftCol.Name)
	rightColName := stripTable(rightCol.Name)

	leftIdx := findColumnIndex(leftSchema, leftColName)
	rightIdx := findColumnIndex(rightSchema, rightColName)

	if leftIdx < 0 || rightIdx < 0 {
		return false
	}

	leftVal := leftRow.Values[leftIdx]
	rightVal := rightRow.Values[rightIdx]

	if leftVal == nil || rightVal == nil {
		return false
	}

	return fmt.Sprintf("%v", leftVal) == fmt.Sprintf("%v", rightVal)
}

func stripTable(name string) string {
	if idx := strings.Index(name, "."); idx >= 0 {
		return name[idx+1:]
	}
	return name
}

func filterJoinedRows(rows []JoinedRow, cond *parser.BinaryOp) []JoinedRow {
	var result []JoinedRow
	for _, row := range rows {
		if evaluateWhereForJoin(row, cond) {
			result = append(result, row)
		}
	}
	return result
}

func evaluateWhereForJoin(row JoinedRow, cond *parser.BinaryOp) bool {
	if cond == nil {
		return true
	}

	leftVal := getJoinValue(row, cond.Left)
	rightVal := getJoinValue(row, cond.Right)

	return compareJoinValues(leftVal, rightVal, cond.Operator)
}

func getJoinValue(row JoinedRow, expr parser.Expression) interface{} {
	switch e := expr.(type) {
	case *parser.Literal:
		return e.Value
	case *parser.Identifier:
		fullName := e.Name
		colName := fullName
		tableName := ""

		if idx := strings.Index(fullName, "."); idx >= 0 {
			tableName = fullName[:idx]
			colName = fullName[idx+1:]
		}

		if tableName == "" || strings.EqualFold(tableName, row.LeftName) {
			for i, name := range row.LeftColumns {
				if strings.EqualFold(name, colName) {
					return row.LeftValues[i]
				}
			}
		}

		if tableName == "" || strings.EqualFold(tableName, row.RightName) {
			for i, name := range row.RightColumns {
				if strings.EqualFold(name, colName) {
					return row.RightValues[i]
				}
			}
		}
	}
	return nil
}

func compareJoinValues(left, right interface{}, op string) bool {
	leftStr := fmt.Sprintf("%v", left)
	rightStr := fmt.Sprintf("%v", right)

	switch op {
	case "=":
		return leftStr == rightStr
	case "!=", "<>":
		return leftStr != rightStr
	case ">":
		return leftStr > rightStr
	case "<":
		return leftStr < rightStr
	case ">=":
		return leftStr >= rightStr
	case "<=":
		return leftStr <= rightStr
	}
	return false
}

func sortJoinedRows(rows []JoinedRow, orderBy string, orderDir string) {
	desc := orderDir == "DESC"
	sort.Slice(rows, func(i, j int) bool {
		valI := getColumnValue(rows[i], orderBy)
		valJ := getColumnValue(rows[j], orderBy)

		aStr := fmt.Sprintf("%v", valI)
		bStr := fmt.Sprintf("%v", valJ)

		// Пробуем как числа
		na, errA := strconv.ParseFloat(aStr, 64)
		nb, errB := strconv.ParseFloat(bStr, 64)

		if errA == nil && errB == nil {
			if desc {
				return na > nb
			}
			return na < nb
		}

		// Как строки
		if desc {
			return aStr > bStr
		}
		return aStr < bStr
	})
}

// getColumnValue ищет значение колонки в JoinedRow
func getColumnValue(row JoinedRow, colName string) interface{} {
	simpleName := colName
	if idx := strings.Index(colName, "."); idx >= 0 {
		simpleName = colName[idx+1:]
	}

	// Ищем в левых колонках
	for i, name := range row.LeftColumns {
		if strings.EqualFold(name, simpleName) || strings.EqualFold(name, colName) {
			return row.LeftValues[i]
		}
	}
	// Ищем в правых колонках
	for i, name := range row.RightColumns {
		if strings.EqualFold(name, simpleName) || strings.EqualFold(name, colName) {
			return row.RightValues[i]
		}
	}
	return nil
}

// limitJoinedRows обрезает слайс по limit и offset
func limitJoinedRows(rows []JoinedRow, limit int, offset int) []JoinedRow {
	if offset >= len(rows) {
		return []JoinedRow{}
	}

	start := offset
	end := len(rows)

	if limit >= 0 && start+limit < end {
		end = start + limit
	}

	return rows[start:end]
}
