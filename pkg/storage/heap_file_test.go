// storage/heap_file_test.go
package storage

import (
	"os"
	"testing"
)

func TestHeapFile_InsertAndScanAfterRestart(t *testing.T) {
	path := "test_heap.db"
	catalogPath := "test_heap.catalog.json"
	defer os.Remove(path)
	defer os.Remove(catalogPath)

	// Сессия 1: создаём таблицу и вставляем данные
	dm1, _ := NewDiskManager(path)
	bp1 := NewBufferPool(10, dm1)

	schema := &TableSchema{
		Name: "test",
		Columns: []ColumnDef{
			{Name: "id", ColType: TypeInt},
			{Name: "name", ColType: TypeText},
		},
	}

	heap1 := NewHeapFile(schema, bp1, dm1)

	row := &Row{Values: []interface{}{int32(1), "hello"}}
	rid, err := heap1.InsertRow(row)
	if err != nil {
		t.Fatalf("InsertRow failed: %v", err)
	}
	t.Logf("Inserted row with RID: %+v", rid)

	// Флушим
	if err := bp1.FlushAll(); err != nil {
		t.Fatalf("FlushAll failed: %v", err)
	}
	dm1.Close()

	// Сессия 2: перезагружаем и сканируем
	dm2, _ := NewDiskManager(path)
	bp2 := NewBufferPool(10, dm2)
	heap2 := NewHeapFile(schema, bp2, dm2)

	rows, err := heap2.ScanFull()
	if err != nil {
		t.Fatalf("ScanFull failed: %v", err)
	}

	if len(rows) != 1 {
		t.Fatalf("Expected 1 row, got %d", len(rows))
	}

	if rows[0].Values[0] != int32(1) || rows[0].Values[1] != "hello" {
		t.Errorf("Data mismatch: got %v", rows[0].Values)
	}
}
