package main

import (
	"agent-db/pkg/executor"
	"agent-db/pkg/index"
	"agent-db/pkg/storage"
	"fmt"
	"os"
)

func main() {
	os.Remove("test_sort.db")
	os.Remove("test_sort.db.idx")
	os.Remove("test_sort.db.dat")
	os.Remove("test_sort.db.wal")

	disk, _ := storage.NewDiskManager("test_sort.db")
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

	exec := &executor.Executor{
		Tables:  make(map[string]*storage.HeapFile),
		Indexes: make(map[string]*index.BTree),
		BP:      bp,
		Disk:    disk,
	}
	exec.Tables["users"] = usersTable
	exec.Tables["orders"] = ordersTable

	// users
	usersTable.InsertRow(&storage.Row{Values: []interface{}{int32(1), "Alice"}})
	usersTable.InsertRow(&storage.Row{Values: []interface{}{int32(2), "Bob"}})

	// orders
	ordersTable.InsertRow(&storage.Row{Values: []interface{}{int32(1), int32(1), float64(100)}})
	ordersTable.InsertRow(&storage.Row{Values: []interface{}{int32(2), int32(1), float64(50)}})
	ordersTable.InsertRow(&storage.Row{Values: []interface{}{int32(3), int32(2), float64(200)}})

	// ASC
	fmt.Println("=== ORDER BY orders.amount ASC ===")
	res, _ := exec.Execute("SELECT * FROM users JOIN orders ON users.id = orders.user_id ORDER BY orders.amount ASC")
	fmt.Println(res)

	// DESC
	fmt.Println("=== ORDER BY orders.amount DESC ===")
	res, _ = exec.Execute("SELECT * FROM users JOIN orders ON users.id = orders.user_id ORDER BY orders.amount DESC")
	fmt.Println(res)
}
