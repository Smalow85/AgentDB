// storage/disk_manager_test.go
package storage

import (
	"os"
	"testing"
)

func TestDiskManager_WriteReadPage(t *testing.T) {
	path := "test_disk.db"
	defer os.Remove(path)

	dm, err := NewDiskManager(path)
	if err != nil {
		t.Fatalf("NewDiskManager failed: %v", err)
	}
	defer dm.Close()

	// Создаём страницу с данными
	page := NewPage(1)
	page.Data[100] = 0xAB
	page.Data[101] = 0xCD
	page.Dirty = true

	// Пишем
	if err := dm.WritePage(page); err != nil {
		t.Fatalf("WritePage failed: %v", err)
	}

	// Читаем обратно
	readPage, err := dm.ReadPage(1)
	if err != nil {
		t.Fatalf("ReadPage failed: %v", err)
	}

	// Проверяем
	if readPage.Data[100] != 0xAB || readPage.Data[101] != 0xCD {
		t.Errorf("Data mismatch: expected AB CD, got %X %X", readPage.Data[100], readPage.Data[101])
	}

	// Проверяем заголовок
	header := readPage.ReadHeader()
	if header.PageID != 1 {
		t.Errorf("PageID mismatch: expected 1, got %d", header.PageID)
	}

	// Проверяем размер файла
	stat, _ := os.Stat(path)
	expectedSize := int64(2 * PageSize) // page 0 + page 1
	if stat.Size() != expectedSize {
		t.Errorf("File size mismatch: expected %d, got %d", expectedSize, stat.Size())
	}
}
