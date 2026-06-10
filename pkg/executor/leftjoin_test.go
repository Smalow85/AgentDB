package executor

import (
	"os"
	"strings"
	"testing"

	"agent-db/pkg/index"
	"agent-db/pkg/storage"
)

func TestExecuteLeftJoin(t *testing.T) {
	os.Remove("test_leftjoin.db")
	disk, _ := storage.NewDiskManager("test_leftjoin.db")
	defer disk.Close()

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

	if result.Type == "ERROR" {
		t.Fatalf("LEFT JOIN returned error: %s", result.Error)
	}

	t.Logf("LEFT JOIN result: %+v", result)
	t.Logf("Rows count: %d", len(result.Rows))

	if len(result.Rows) == 0 {
		t.Fatal("LEFT JOIN вернул пустой результат")
	}

	// Проверяем что есть Charlie
	foundCharlie := false
	for _, row := range result.Rows {
		rowStr := strings.Join(toStringSlice(row), " ")
		if strings.Contains(rowStr, "Charlie") {
			foundCharlie = true
			// Проверяем что есть NULL для Charlie
			if !strings.Contains(rowStr, "NULL") && !strings.Contains(rowStr, "<nil>") {
				t.Errorf("Charlie должен иметь NULL в правой части, но получил: %s", rowStr)
			}
		}
	}

	if !foundCharlie {
		t.Error("LEFT JOIN должен содержать Charlie с NULL")
	}
}

func TestExecuteJoinWithWhere(t *testing.T) {
	os.Remove("test_join_where.db")
	disk, _ := storage.NewDiskManager("test_join_where.db")
	defer disk.Close()

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

	if result.Type == "ERROR" {
		t.Fatalf("JOIN + WHERE returned error: %s", result.Error)
	}

	t.Logf("JOIN + WHERE result: %+v", result)

	if len(result.Rows) == 0 {
		t.Fatal("JOIN + WHERE вернул пустой результат")
	}

	// Должна быть только Alice (Bob отфильтрован)
	foundBob := false
	for _, row := range result.Rows {
		rowStr := strings.Join(toStringSlice(row), " ")
		if strings.Contains(rowStr, "Bob") {
			foundBob = true
		}
	}

	if foundBob {
		t.Error("JOIN + WHERE должен был отфильтровать Bob")
	}
}

func containsNull(s string) bool {
	return strings.Contains(s, "NULL") || strings.Contains(s, "<nil>")
}

func containsString(s, substr string) bool {
	return len(s) > 0 && strings.Contains(s, substr)
}
