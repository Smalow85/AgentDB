package executor

import (
	"fmt"
	"strconv"
	"strings"

	"agent-db/pkg/storage"
)

// validateConstraints проверяет строку на соответствие constraints
func validateConstraints(table *storage.HeapFile, row *storage.Row, isUpdate bool, oldRow *storage.Row) error {
	schema := table.Schema

	for i, col := range schema.Columns {
		val := row.Values[i]

		// NOT NULL
		if !col.Nullable && val == nil {
			return fmt.Errorf("колонка '%s' не может быть NULL", col.Name)
		}

		// DEFAULT
		if val == nil && col.Default != "" {
			parsed, err := storage.ParseValue(col.Default, col.ColType)
			if err != nil {
				return fmt.Errorf("колонка '%s': неверное DEFAULT значение: %w", col.Name, err)
			}
			row.Values[i] = parsed
			val = parsed
		}

		// CHECK
		if col.Check != "" && val != nil {
			if !evaluateCheck(col.Check, val) {
				return fmt.Errorf("колонка '%s': нарушено условие CHECK (%s)", col.Name, col.Check)
			}
		}
	}

	// PRIMARY KEY и UNIQUE
	for i, col := range schema.Columns {
		if col.PrimaryKey || col.Unique {
			val := row.Values[i]

			rows, err := table.ScanFull()
			if err != nil {
				return err
			}

			for _, existingRow := range rows {
				if fmt.Sprintf("%v", existingRow.Values[i]) == fmt.Sprintf("%v", val) {
					constraintType := "UNIQUE"
					if col.PrimaryKey {
						constraintType = "PRIMARY KEY"
					}
					return fmt.Errorf("нарушение %s: значение '%v' уже существует в колонке '%s'",
						constraintType, val, col.Name)
				}
			}
		}
	}

	return nil
}

// evaluateCheck проверяет CHECK условие
func evaluateCheck(check string, val interface{}) bool {
	// Поддерживаем простые проверки: > 0, < 100, != 0, = 'value'
	parts := strings.Fields(check)
	if len(parts) != 2 {
		return true // не смогли разобрать — пропускаем
	}

	op := parts[0]
	checkVal := parts[1]

	valStr := fmt.Sprintf("%v", val)

	// Пробуем как числа
	numVal, err1 := strconv.ParseFloat(valStr, 64)
	numCheck, err2 := strconv.ParseFloat(checkVal, 64)

	if err1 == nil && err2 == nil {
		switch op {
		case ">": return numVal > numCheck
		case "<": return numVal < numCheck
		case ">=": return numVal >= numCheck
		case "<=": return numVal <= numCheck
		case "=", "==": return numVal == numCheck
		case "!=", "<>": return numVal != numCheck
		}
	}

	// Как строки
	switch op {
	case "=", "==": return valStr == checkVal
	case "!=", "<>": return valStr != checkVal
	}

	return true
}