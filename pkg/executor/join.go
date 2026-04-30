package executor

import (
	"fmt"
	"strings"

	"agent-db/pkg/parser"
	"agent-db/pkg/storage"
)

// JoinedRow — строка после JOIN (данные из двух таблиц)
type JoinedRow struct {
	LeftValues  []interface{}
	RightValues []interface{}
	LeftName    string
	RightName   string
}

// executeJoin — выполняет SELECT с JOIN
func (e *Executor) executeJoin(stmt *parser.SelectStatement) (string, error) {
	leftTable, ok := e.Tables[stmt.Table]
	if !ok {
		return "", fmt.Errorf("таблица '%s' не найдена", stmt.Table)
	}

	rightTable, ok := e.Tables[stmt.Join.Table]
	if !ok {
		return "", fmt.Errorf("таблица '%s' не найдена", stmt.Join.Table)
	}

	leftRows, err := leftTable.ScanFull()
	if err != nil {
		return "", err
	}

	rightRows, err := rightTable.ScanFull()
	if err != nil {
		return "", err
	}

	// Nested Loop Join
	var joined []JoinedRow

	for _, leftRow := range leftRows {
		matched := false

		for _, rightRow := range rightRows {
			if evaluateJoinCondition(leftRow, rightRow, leftTable.Schema, rightTable.Schema, stmt.Join.Condition) {
				joined = append(joined, JoinedRow{
					LeftValues:  leftRow.Values,
					RightValues: rightRow.Values,
					LeftName:    stmt.Table,
					RightName:   stmt.Join.Table,
				})
				matched = true
			}
		}

		// LEFT JOIN — если не нашли совпадений, добавляем с NULL справа
		if !matched && stmt.Join.Type == parser.LeftJoin {
			nullRight := make([]interface{}, len(rightTable.Schema.Columns))
			for i := range nullRight {
				nullRight[i] = nil
			}
			joined = append(joined, JoinedRow{
				LeftValues:  leftRow.Values,
				RightValues: nullRight,
				LeftName:    stmt.Table,
				RightName:   stmt.Join.Table,
			})
		}
	}

	// WHERE фильтрация после JOIN
	if stmt.Condition != nil {
		joined = filterJoinedRows(joined, stmt.Condition)
	}

	// Вывод
	return formatJoinResult(joined, stmt), nil
}

// evaluateJoinCondition — проверяет условие JOIN для двух строк
func evaluateJoinCondition(
	leftRow *storage.Row,
	rightRow *storage.Row,
	leftSchema *storage.TableSchema,
	rightSchema *storage.TableSchema,
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

	var leftVal, rightVal interface{}

	leftIdx := findColumnIndex(leftSchema, leftCol.Name)
	rightIdx := findColumnIndex(rightSchema, rightCol.Name)

	if leftIdx >= 0 && rightIdx >= 0 {
		leftVal = leftRow.Values[leftIdx]
		rightVal = rightRow.Values[rightIdx]
	} else {
		// Может наоборот
		leftIdx = findColumnIndex(leftSchema, rightCol.Name)
		rightIdx = findColumnIndex(rightSchema, leftCol.Name)
		if leftIdx >= 0 && rightIdx >= 0 {
			leftVal = leftRow.Values[leftIdx]
			rightVal = rightRow.Values[rightIdx]
		} else {
			return false
		}
	}

	return fmt.Sprintf("%v", leftVal) == fmt.Sprintf("%v", rightVal)
}

// filterJoinedRows — фильтрует JOIN-строки по WHERE
func filterJoinedRows(rows []JoinedRow, cond *parser.BinaryOp) []JoinedRow {
	// Упрощённо: пропускаем все (можно доработать)
	return rows
}

// formatJoinResult — форматирует результат JOIN
func formatJoinResult(rows []JoinedRow, stmt *parser.SelectStatement) string {
	joinType := "INNER"
	if stmt.Join.Type == parser.LeftJoin {
		joinType = "LEFT"
	}

	result := fmt.Sprintf("\n┌──────────────────────────────────────────────────────────┐\n")
	result += fmt.Sprintf("│ %s JOIN: %s ⋈ %s\n", joinType, stmt.Table, stmt.Join.Table)
	result += fmt.Sprintf("├──────────────────────────────────────────────────────────┤\n")

	for _, row := range rows {
		leftStr := formatRowValues(row.LeftValues)
		rightStr := formatRowValues(row.RightValues)
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