package executor

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"agent-db/pkg/index"
	"agent-db/pkg/parser"
	"agent-db/pkg/storage"
)

func TestExecuteJoin(t *testing.T) {
	os.Remove("test_join.db")
	disk, _ := storage.NewDiskManager("test_join.db")
	defer disk.Close()

	bp := storage.NewBufferPool(100, disk)

	// Создаём таблицы
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

	usersTable := storage.NewHeapFile(usersSchema, bp, disk)
	ordersTable := storage.NewHeapFile(ordersSchema, bp, disk)

	exec := &Executor{
		Tables:  make(map[string]*storage.HeapFile),
		Indexes: make(map[string]*index.BTree),
		BP:      bp,
		Disk:    disk,
	}
	exec.Tables["users"] = usersTable
	exec.Tables["orders"] = ordersTable

	// Вставка данных
	usersTable.InsertRow(&storage.Row{Values: []interface{}{int32(1), "Alice"}})
	usersTable.InsertRow(&storage.Row{Values: []interface{}{int32(2), "Bob"}})

	ordersTable.InsertRow(&storage.Row{Values: []interface{}{int32(1), int32(1), float64(100.50)}})
	ordersTable.InsertRow(&storage.Row{Values: []interface{}{int32(2), int32(1), float64(200.00)}})
	ordersTable.InsertRow(&storage.Row{Values: []interface{}{int32(3), int32(2), float64(50.00)}})

	// JOIN
	result, err := exec.Execute("SELECT * FROM users JOIN orders ON users.id = orders.user_id")
	if err != nil {
		t.Fatalf("JOIN error: %v", err)
	}

	if result.Type == "ERROR" {
		t.Fatalf("JOIN returned error: %s", result.Error)
	}

	t.Logf("JOIN result: %+v", result)
	t.Logf("Columns: %v", result.Columns)
	t.Logf("Rows count: %d", len(result.Rows))

	if len(result.Rows) == 0 {
		t.Error("JOIN вернул пустой результат")
	}

	// Проверяем что есть данные из обеих таблиц
	foundAlice := false
	foundBob := false
	for _, row := range result.Rows {
		rowStr := strings.Join(toStringSlice(row), " ")
		if strings.Contains(rowStr, "Alice") {
			foundAlice = true
		}
		if strings.Contains(rowStr, "Bob") {
			foundBob = true
		}
	}

	if !foundAlice {
		t.Error("JOIN результат не содержит Alice")
	}
	if !foundBob {
		t.Error("JOIN результат не содержит Bob")
	}
}

func TestParseJoinStmt(t *testing.T) {
	stmt, err := parser.Parse("SELECT * FROM users JOIN orders ON users.id = orders.user_id")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	sel, ok := stmt.(*parser.SelectStatement)
	if !ok {
		t.Fatalf("expected SelectStatement, got %T", stmt)
	}

	if sel.Join == nil {
		t.Error("JOIN not parsed")
	}

	if sel.Join.Table != "orders" {
		t.Errorf("expected JOIN table 'orders', got '%s'", sel.Join.Table)
	}

	if sel.Join.Condition == nil {
		t.Error("JOIN condition not parsed")
	}
}

func toStringSlice(row []interface{}) []string {
	result := make([]string, len(row))
	for i, v := range row {
		if v == nil {
			result[i] = "NULL"
		} else {
			result[i] = fmt.Sprintf("%v", v)
		}
	}
	return result
}
