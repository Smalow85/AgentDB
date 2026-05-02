package executor

import (
	"testing"

	"agent-db/pkg/parser"
	"agent-db/pkg/storage"
)

func TestEvaluateJoinCondition(t *testing.T) {
	usersSchema := &storage.TableSchema{
		Name: "users",
		Columns: []storage.ColumnDef{
			{Name: "id", ColType: storage.TypeInt},
			{Name: "name", ColType: storage.TypeText},
		},
	}
	ordersSchema := &storage.TableSchema{
		Name: "orders",
		Columns: []storage.ColumnDef{
			{Name: "id", ColType: storage.TypeInt},
			{Name: "user_id", ColType: storage.TypeInt},
			{Name: "amount", ColType: storage.TypeFloat},
		},
	}

	// Парсим условие JOIN
	stmt, err := parser.Parse("SELECT * FROM users JOIN orders ON users.id = orders.user_id")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	sel := stmt.(*parser.SelectStatement)
	cond := sel.Join.Condition
	t.Logf("Condition type: %T", cond)
	t.Logf("Condition: %+v", cond)

	if cond == nil {
		t.Fatal("condition is nil")
	}

	// Проверяем структуру BinaryOp
	t.Logf("Left: %+v (type %T)", cond.Left, cond.Left)
	t.Logf("Right: %+v (type %T)", cond.Right, cond.Right)
	t.Logf("Operator: %s", cond.Operator)

	// Тестируем evaluateJoinCondition напрямую
	userRow := &storage.Row{Values: []interface{}{int32(1), "Alice"}}
	orderRow := &storage.Row{Values: []interface{}{int32(1), int32(1), float64(100.50)}}

	result := evaluateJoinCondition(userRow, orderRow, usersSchema, ordersSchema, cond)
	t.Logf("evaluateJoinCondition result: %v", result)

	if !result {
		t.Error("Expected join condition to match, got false")
	}
}
