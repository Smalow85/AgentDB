package executor

import (
	"strings"
	"testing"
	"os"

	"agent-db/pkg/index"
	"agent-db/pkg/storage"
)

func TestExecuteLeftJoin(t *testing.T) {
	os.Remove("test_leftjoin.db")
	disk, _ := storage.NewDiskManager("test_leftjoin.db")
	bp := storage.NewBufferPool(100, disk)

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

	// users: Alice (id=1), Bob (id=2), Charlie (id=3)
	usersTable.InsertRow(&storage.Row{Values: []interface{}{int32(1), "Alice"}})
	usersTable.InsertRow(&storage.Row{Values: []interface{}{int32(2), "Bob"}})
	usersTable.InsertRow(&storage.Row{Values: []interface{}{int32(3), "Charlie"}})

	// orders: только для user_id=1 и user_id=2
	ordersTable.InsertRow(&storage.Row{Values: []interface{}{int32(1), int32(1), float64(100.50)}})
	ordersTable.InsertRow(&storage.Row{Values: []interface{}{int32(2), int32(1), float64(200.00)}})
	ordersTable.InsertRow(&storage.Row{Values: []interface{}{int32(3), int32(2), float64(50.00)}})

	// LEFT JOIN - Charlie не имеет заказов, должен быть NULL справа
	result, err := exec.Execute("SELECT * FROM users LEFT JOIN orders ON users.id = orders.user_id")
	if err != nil {
		t.Fatalf("LEFT JOIN error: %v", err)
	}

	t.Logf("LEFT JOIN result:\n%s", result)

	// Проверяем что есть NULL для Charlie
	if result == "" {
		t.Fatal("LEFT JOIN вернул пустой результат")
	}

	// Проверяем что есть NULL в результате
	if !containsNull(result) {
		t.Error("LEFT JOIN должен содержать NULL для Charlie")
	}
}

func TestExecuteJoinWithWhere(t *testing.T) {
	os.Remove("test_join_where.db")
	disk, _ := storage.NewDiskManager("test_join_where.db")
	bp := storage.NewBufferPool(100, disk)

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

	usersTable.InsertRow(&storage.Row{Values: []interface{}{int32(1), "Alice"}})
	usersTable.InsertRow(&storage.Row{Values: []interface{}{int32(2), "Bob"}})

	ordersTable.InsertRow(&storage.Row{Values: []interface{}{int32(1), int32(1), float64(100.50)}})
	ordersTable.InsertRow(&storage.Row{Values: []interface{}{int32(2), int32(1), float64(200.00)}})
	ordersTable.InsertRow(&storage.Row{Values: []interface{}{int32(3), int32(2), float64(50.00)}})

	// JOIN + WHERE - только для Alice (id=1)
	result, err := exec.Execute("SELECT * FROM users JOIN orders ON users.id = orders.user_id WHERE users.id = 1")
	if err != nil {
		t.Fatalf("JOIN + WHERE error: %v", err)
	}

	t.Logf("JOIN + WHERE result:\n%s", result)

	if result == "" {
		t.Fatal("JOIN + WHERE вернул пустой результат")
	}

	// Должна быть только Alice с заказами (Bob отфильтрован)
	if containsString(result, "Bob") {
		t.Error("JOIN + WHERE должен был отфильтровать Bob")
	}
}

func containsNull(s string) bool {
	return containsString(s, "NULL") || containsString(s, "<nil>")
}

func containsString(s, substr string) bool {
	return len(s) > 0 && strings.Contains(s, substr)
}
