package storage

import (
	"os"
	"testing"
)

func setupBufferPool(t *testing.T) (*BufferPool, *DiskManager) {
	disk, err := NewDiskManager(os.TempDir() + "/test_bp_" + t.Name() + ".dat")
	if err != nil {
		t.Fatal(err)
	}
	bp := NewBufferPool(3, disk)
	return bp, disk
}

func teardownBufferPool(bp *BufferPool, disk *DiskManager) {
	bp.FlushAll()
	disk.Close()
	os.Remove("/tmp/test_bp_" + "test" + ".dat")
}

func TestBufferPoolFetchAndCache(t *testing.T) {
	bp, disk := setupBufferPool(t)
	defer teardownBufferPool(bp, disk)

	// Первая загрузка — с диска
	page1, err := bp.FetchPage(1)
	if err != nil {
		t.Fatal(err)
	}
	bp.UnpinPage(1, false)

	// Вторая — должна быть из кэша
	page1Again, err := bp.FetchPage(1)
	if err != nil {
		t.Fatal(err)
	}
	bp.UnpinPage(1, false)

	if page1 != page1Again {
		t.Error("Указатели должны совпадать (тот же объект в кэше)")
	}
}

func TestBufferPoolLRU(t *testing.T) {
	bp, disk := setupBufferPool(t)
	defer teardownBufferPool(bp, disk)

	// Загружаем 3 страницы (capacity=3)
	bp.FetchPage(1)
	bp.UnpinPage(1, false)

	bp.FetchPage(2)
	bp.UnpinPage(2, false)

	bp.FetchPage(3)
	bp.UnpinPage(3, false)

	// Доступ к page1 обновляет её позицию в LRU
	bp.FetchPage(1)
	bp.UnpinPage(1, false)

	// Загружаем 4-ю — должна вытеснить самую старую (page2)
	page4, err := bp.FetchPage(4)
	if err != nil {
		t.Fatal(err)
	}
	bp.UnpinPage(4, false)

	_ = page4
	// Проверить что page2 вытеснена — при следующем FetchPage(2) будет новый объект
}

func TestBufferPoolDirtyFlush(t *testing.T) {
	bp, disk := setupBufferPool(t)
	defer teardownBufferPool(bp, disk)

	page, _ := bp.FetchPage(1)

	// Меняем страницу
	data := []byte("test")
	page.InsertRow(data)
	bp.UnpinPage(1, true) // dirty

	// Вытесняем — должно записаться на диск
	bp.FetchPage(2)
	bp.UnpinPage(2, false)
	bp.FetchPage(3)
	bp.UnpinPage(3, false)
	bp.FetchPage(4) // вытеснит page1
	bp.UnpinPage(4, false)

	// Загружаем page1 снова — должна быть с диска
	pageAgain, _ := bp.FetchPage(1)
	defer bp.UnpinPage(1, false)

	row, _ := pageAgain.GetRow(0)
	if string(row) != "test" {
		t.Error("Данные должны сохраниться после вытеснения")
	}
}

func TestBufferPool_FlushAll(t *testing.T) {
	path := "test_bp.db"
	defer os.Remove(path)

	dm, _ := NewDiskManager(path)
	defer dm.Close()

	bp := NewBufferPool(10, dm)

	// Создаём страницу через BufferPool
	page, err := bp.FetchPage(1)
	if err != nil {
		t.Fatalf("FetchPage failed: %v", err)
	}

	// Пишем данные
	page.Data[100] = 0x42
	bp.UnpinPage(1, true)

	// Флушим
	if err := bp.FlushAll(); err != nil {
		t.Fatalf("FlushAll failed: %v", err)
	}

	// Создаём НОВЫЙ BufferPool (симулируем перезапуск)
	bp2 := NewBufferPool(10, dm)

	// Читаем страницу
	page2, err := bp2.FetchPage(1)
	if err != nil {
		t.Fatalf("FetchPage 2 failed: %v", err)
	}

	if page2.Data[100] != 0x42 {
		t.Errorf("Data lost after flush: expected 0x42, got 0x%X", page2.Data[100])
	}
}
