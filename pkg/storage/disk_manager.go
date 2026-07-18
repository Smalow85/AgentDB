package storage

import (
	"fmt"
	"os"
	"sync"
)

// DiskManager управляет чтением/записью страниц на диск
type DiskManager struct {
	file      *os.File
	pageCount uint64
	mu        sync.Mutex
}

// NewDiskManager открывает БД. dbPath — путь к файлу БД
func NewDiskManager(dbPath string) (*DiskManager, error) {
	// Создаём директорию для файла если нужно
	dir := ""
	for i := len(dbPath) - 1; i >= 0; i-- {
		if dbPath[i] == '/' || dbPath[i] == '\\' {
			dir = dbPath[:i]
			break
		}
	}
	if dir != "" {
		os.MkdirAll(dir, 0755)
	}

	file, err := os.OpenFile(dbPath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, fmt.Errorf("ошибка открытия файла: %w", err)
	}

	stat, _ := file.Stat()
	pageCount := uint64(stat.Size()) / PageSize
	if pageCount == 0 {
		pageCount = 1
	}

	return &DiskManager{
		file:      file,
		pageCount: pageCount,
	}, nil
}

// ReadPage читает страницу с диска
func (dm *DiskManager) ReadPage(pageID uint64) (*Page, error) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	page := &Page{ID: pageID}
	offset := int64(pageID * PageSize)

	n, err := dm.file.ReadAt(page.Data[:], offset)
	if err != nil && err.Error() != "EOF" {
		return nil, fmt.Errorf("ошибка чтения страницы %d: %w", pageID, err)
	}

	if n == 0 || (n > 0 && page.ReadHeader().PageID != pageID) {
		return NewPage(pageID), nil
	}

	return page, nil
}

// WritePage записывает страницу на диск
func (dm *DiskManager) WritePage(page *Page) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	offset := int64(page.ID * PageSize)
	_, err := dm.file.WriteAt(page.Data[:], offset)
	if err != nil {
		return fmt.Errorf("ошибка записи страницы %d: %w", page.ID, err)
	}

	dm.file.Sync()

	if page.ID >= dm.pageCount {
		dm.pageCount = page.ID + 1
	}

	page.Dirty = false
	return nil
}

// AllocatePage выделяет новую страницу
func (dm *DiskManager) AllocatePage() uint64 {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	pageID := dm.pageCount
	dm.pageCount++
	return pageID
}

// Close закрывает файл
func (dm *DiskManager) Close() error {
	return dm.file.Close()
}

// PageCount возвращает количество страниц в файле
func (dm *DiskManager) PageCount() uint64 {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	return dm.pageCount
}
