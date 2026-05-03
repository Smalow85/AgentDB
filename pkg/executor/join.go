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

func (e *Executor) executeJoin(stmt *parser.SelectStatement) (string, error) {
	leftTable, ok := e.Tables[stmt.Table]
	if !ok {
		return "", fmt.Errorf("таблица '%s' не найдена", stmt.Table)
	}

	rightTable, ok := e.Tables[stmt.Join.Table]
	if !ok {
		return "", fmt.Errorf("таблица '%s' не найдена", stmt.Join.Table)
	}

	leftCols := columnNames(leftTable.Schema)
	rightCols := columnNames(rightTable.Schema)

	leftRows, _ := leftTable.ScanFull()
	rightRows, _ := rightTable.ScanFull()

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
				LeftColumns:  leftCols,
				RightValues:  nullRight,
				RightColumns: rightCols,
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

	// LIMIT / OFFSET для JOIN
	if stmt.Limit >= 0 || stmt.Offset > 0 {
		joined = limitJoinedRows(joined, stmt.Limit, stmt.Offset)
	}

	return formatJoinResult(joined, stmt), nil
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

func formatJoinResult(rows []JoinedRow, stmt *parser.SelectStatement) string {
	joinType := "INNER"
	if stmt.Join.Type == parser.LeftJoin {
		joinType = "LEFT"
	}

	result := fmt.Sprintf("\n┌──────────────────────────────────────────────────────────┐\n")
	result += fmt.Sprintf("│ %s JOIN: %s ⋈ %s\n", joinType, stmt.Table, stmt.Join.Table)
	result += fmt.Sprintf("├──────────────────────────────────────────────────────────┤\n")

	for _, row := range rows {
		leftStr := formatJoinRowValues(row.LeftValues)
		rightStr := formatJoinRowValues(row.RightValues)
		result += fmt.Sprintf("│ %s | %s\n", leftStr, rightStr)
	}

	result += fmt.Sprintf("├──────────────────────────────────────────────────────────┤\n")
	result += fmt.Sprintf("│ Строк: %-52d │\n", len(rows))
	result += fmt.Sprintf("└──────────────────────────────────────────────────────────┘\n")
	return result
}

func formatRowValues(values []interface{}) string {
	strs := make([]string, len(values))
	for i, v := range values {
		strs[i] = formatValue(v)
	}
	return strings.Join(strs, ", ")
}

func formatJoinRowValues(values []interface{}) string {
	strs := make([]string, len(values))
	for i, v := range values {
		if v == nil {
			strs[i] = "NULL"
		} else {
			strs[i] = formatValue(v)
		}
	}
	return strings.Join(strs, ", ")
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
	// Извлекаем имя колонки из "table.column"
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
