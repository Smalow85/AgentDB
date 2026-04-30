package integration

import (
	"fmt"
	"os"
	"sqldb/pkg/storage"
	"testing"
)

func TestStorageEndToEnd(t *testing.T) {
	dbFile := "/tmp/test_integration.dat"
	defer os.Remove(dbFile)

	disk, err := storage.NewDiskManager(dbFile)
	if err != nil {
		t.Fatal(err)
	}
	defer disk.Close()

	bp := storage.NewBufferPool(100, disk)
	defer bp.FlushAll()

	schema := &storage.TableSchema{
		Name: "users",
		Columns: []storage.ColumnDef{
			{Name: "id", ColType: storage.TypeInt},
			{Name: "name", ColType: storage.TypeText},
			{Name: "age", ColType: storage.TypeInt},
			{Name: "email", ColType: storage.TypeText},
			{Name: "active", ColType: storage.TypeBool},
		},
	}

	heap := storage.NewHeapFile(schema, bp, disk)

	// Тест 1: Вставка 100 строк
	t.Run("Insert100Rows", func(t *testing.T) {
		for i := 1; i <= 100; i++ {
			row := &storage.Row{
				Values: []interface{}{
					i,
					fmt.Sprintf("User%d", i),
					20 + i%30,
					fmt.Sprintf("user%d@example.com", i),
					i%2 == 0,
				},
			}
			rid, err := heap.InsertRow(row)
			if err != nil {
				t.Fatalf("Ошибка вставки строки %d: %v", i, err)
			}
			if rid.PageID == 0 {
				t.Error("RID не должен быть нулевым")
			}
		}
	})

	// Тест 2: Полный скан
	t.Run("FullScan", func(t *testing.T) {
		rows, err := heap.ScanFull()
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 100 {
			t.Errorf("Ожидалось 100 строк, получено %d", len(rows))
		}
	})

	// Тест 3: Перезагрузка с диска
	t.Run("ReloadFromDisk", func(t *testing.T) {
		bp.FlushAll()
		disk.Close()

		// Открываем заново
		disk2, err := storage.NewDiskManager(dbFile)
		if err != nil {
			t.Fatal(err)
		}
		defer disk2.Close()

		bp2 := storage.NewBufferPool(100, disk2)
		defer bp2.FlushAll()

		heap2 := storage.NewHeapFile(schema, bp2, disk2)
		rows, err := heap2.ScanFull()
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) < 100 {
			t.Errorf("После перезагрузки ожидалось минимум 100 строк, получено %d", len(rows))
		}
	})
}

func TestConcurrentAccess(t *testing.T) {
	t.Skip("Будет реализовано после добавления транзакций")
}