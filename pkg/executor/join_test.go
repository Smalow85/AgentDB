package executor

import (
	"testing"

	"agent-db/pkg/index"
	"agent-db/pkg/parser"
	"agent-db/pkg/storage"
)

func TestExecuteJoin(t *testing.T) {
	disk, _ := storage.NewDiskManager("test_join.db")
	bp := storage.NewBufferPool(100, disk)

	// Создаём таблицы напрямую вручную
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
	userRow1 := &storage.Row{Values: []interface{}{int32(1), "Alice"}}
	usersTable.InsertRow(userRow1)

	userRow2 := &storage.Row{Values: []interface{}{int32(2), "Bob"}}
	usersTable.InsertRow(userRow2)

	orderRow1 := &storage.Row{Values: []interface{}{int32(1), int32(1), float64(100.50)}}
	ordersTable.InsertRow(orderRow1)

	orderRow2 := &storage.Row{Values: []interface{}{int32(2), int32(1), float64(200.00)}}
	ordersTable.InsertRow(orderRow2)

	orderRow3 := &storage.Row{Values: []interface{}{int32(3), int32(2), float64(50.00)}}
	ordersTable.InsertRow(orderRow3)

	// Проверяем что данные есть
	users, _ := usersTable.ScanFull()
	t.Logf("Users rows: %d", len(users))
	for i, r := range users {
		t.Logf("  user[%d]: %v", i, r.Values)
	}

	orders, _ := ordersTable.ScanFull()
	t.Logf("Orders rows: %d", len(orders))
	for i, r := range orders {
		t.Logf("  order[%d]: %v", i, r.Values)
	}

	// JOIN
	result, err := exec.Execute("SELECT * FROM users JOIN orders ON users.id = orders.user_id")
	t.Logf("JOIN result:\n%s", result)
	if err != nil {
		t.Fatalf("JOIN error: %v", err)
	}

	if result == "" {
		t.Error("JOIN вернул пустой результат")
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
