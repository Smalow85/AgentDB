package integration

import (
	"agent-db/pkg/storage"
	"fmt"
	"os"
	"testing"
)

func TestStorageEndToEnd(t *testing.T) {
    dbFile := "/tmp/test_integration.dat"
    defer os.Remove(dbFile)

    disk, err := storage.NewDiskManager(dbFile)
    if err != nil {
        t.Fatal(err)
    }
    // НЕТ defer disk.Close() — закрываем вручную в ReloadFromDisk

    bp := storage.NewBufferPool(100, disk)
    // НЕТ defer bp.FlushAll() — флушим вручную

    schema := &storage.TableSchema{...}
    heap := storage.NewHeapFile(schema, bp, disk)

    // Тест 1: Вставка
    t.Run("Insert100Rows", func(t *testing.T) {
        for i := 1; i <= 100; i++ {
            row := &storage.Row{...}
            if _, err := heap.InsertRow(row); err != nil {
                t.Fatalf("Ошибка вставки: %v", err)
            }
        }
    })

    // Тест 2: Полный скан (из буфера)
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
        // ✅ Флушим и закрываем ПЕРЕД созданием новых
        if err := bp.FlushAll(); err != nil {
            t.Fatalf("FlushAll failed: %v", err)
        }
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
            t.Errorf("После перезагрузки ожидалось 100 строк, получено %d", len(rows))
        }
    })
}
