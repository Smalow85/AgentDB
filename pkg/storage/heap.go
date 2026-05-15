package storage

import (
	"fmt"
	"sync"
)

// HeapFile представляет таблицу как кучу страниц
type HeapFile struct {
	Schema     *TableSchema
	BufferPool *BufferPool
	Disk       *DiskManager
	mu         sync.RWMutex
}

// NewHeapFile создаёт новую кучу
func NewHeapFile(schema *TableSchema, bp *BufferPool, disk *DiskManager) *HeapFile {
	return &HeapFile{
		Schema:     schema,
		BufferPool: bp,
		Disk:       disk,
	}
}

// InsertRow вставляет строку в таблицу, возвращает RID
func (h *HeapFile) InsertRow(row *Row) (RID, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	data, err := SerializeRow(row, h.Schema)
	if err != nil {
		return InvalidRID, fmt.Errorf("ошибка сериализации: %w", err)
	}

	var page *Page
	var pageID uint64

	// Ищем страницу с местом СРЕДИ СТРАНИЦ ЭТОЙ ТАБЛИЦЫ
	for pid := uint64(1); pid < h.Disk.pageCount; pid++ {
		p, err := h.BufferPool.FetchPage(pid)
		if err != nil {
			continue
		}
		header := p.ReadHeader()
		// Проверяем, что страница принадлежит этой таблице или пустая (TableID=0 AND RowCount=0)
		if header.TableID == h.TableID() || (header.TableID == 0 && header.RowCount == 0) {
			if p.CanFit(uint16(len(data))) {
				page = p
				pageID = pid
				break
			}
		}
		h.BufferPool.UnpinPage(pid, false)
	}

	// Если нет подходящей — создаём новую
	if page == nil {
		pageID = h.Disk.AllocatePage()
		// Добавляем страницу в буферный пул, чтобы FlushAll мог её записать
		var err error
		page, err = h.BufferPool.FetchPage(pageID)
		if err != nil {
			return InvalidRID, fmt.Errorf("ошибка выделения страницы: %w", err)
		}
		// Устанавливаем TableID сразу, чтобы страница не была видна другим таблицам как "пустая"
		header := page.ReadHeader()
		header.TableID = h.TableID()
		page.writeHeader(header)
	}

	// Устанавливаем TableID
	header := page.ReadHeader()
	header.TableID = h.TableID()
	page.writeHeader(header)

	// Вставляем
	slotID, err := page.InsertRow(data)
	if err != nil {
		h.BufferPool.UnpinPage(pageID, false)
		return InvalidRID, fmt.Errorf("ошибка вставки на страницу: %w", err)
	}

	h.BufferPool.UnpinPage(pageID, true)
	h.BufferPool.FlushAll()

	return RID{PageID: pageID, SlotID: slotID}, nil
}

// ScanFull — полный скан таблицы (только свои страницы)
func (h *HeapFile) ScanFull() ([]*Row, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var rows []*Row
	tableID := h.TableID()

	for pid := uint64(1); pid < h.Disk.pageCount; pid++ {
		page, err := h.BufferPool.FetchPage(pid)
		if err != nil {
			continue
		}

		header := page.ReadHeader()
		// Пропускаем чужие страницы
		if header.TableID != tableID {
			h.BufferPool.UnpinPage(pid, false)
			continue
		}

		for slotID := uint16(0); slotID < header.RowCount; slotID++ {
			rowData, err := page.GetRow(slotID)
			if err != nil {
				continue
			}
			row, err := DeserializeRow(rowData, h.Schema)
			if err != nil {
				continue
			}
			row.RID = RID{PageID: pid, SlotID: slotID}
			rows = append(rows, row)
		}

		h.BufferPool.UnpinPage(pid, false)
	}

	return rows, nil
}

// PageCount returns total number of pages
func (h *HeapFile) PageCount() uint64 {
	return h.Disk.pageCount
}

// TableID возвращает числовой ID таблицы (хеш от имени)
func (h *HeapFile) TableID() uint32 {
	return hashString(h.Schema.Name)
}

// hashString — простой хеш строки в uint32
func hashString(s string) uint32 {
	var h uint32
	for _, c := range s {
		h = h*31 + uint32(c)
	}
	return h
}

// GetRow читает строку по RID
func (h *HeapFile) GetRow(rid RID) (*Row, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	page, err := h.BufferPool.FetchPage(rid.PageID)
	if err != nil {
		return nil, err
	}
	defer h.BufferPool.UnpinPage(rid.PageID, false)

	rowData, err := page.GetRow(rid.SlotID)
	if err != nil {
		return nil, err
	}

	return DeserializeRow(rowData, h.Schema)
}

// DeleteRow помечает строку удалённой
func (h *HeapFile) DeleteRow(rid RID) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	page, err := h.BufferPool.FetchPage(rid.PageID)
	if err != nil {
		return err
	}
	defer h.BufferPool.UnpinPage(rid.PageID, true)

	return page.DeleteRow(rid.SlotID)
}

// UpdateRow обновляет строку по RID
func (h *HeapFile) UpdateRow(rid RID, newRow *Row) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	page, err := h.BufferPool.FetchPage(rid.PageID)
	if err != nil {
		return fmt.Errorf("ошибка загрузки страницы %d: %w", rid.PageID, err)
	}
	defer h.BufferPool.UnpinPage(rid.PageID, true)

	// Удаляем старую строку
	err = page.DeleteRow(rid.SlotID)
	if err != nil {
		return fmt.Errorf("ошибка удаления старой строки: %w", err)
	}

	// Сериализуем новую
	data, err := SerializeRow(newRow, h.Schema)
	if err != nil {
		return fmt.Errorf("ошибка сериализации: %w", err)
	}

	// Пробуем вставить на ту же страницу
	_, err = page.InsertRow(data)
	if err != nil {
		// Не помещается — вставляем как новую строку
		_, err = h.InsertRow(newRow)
		if err != nil {
			return err
		}
	}

	return nil
}

// String — дамп таблицы для отладки
func (h *HeapFile) String() string {
	rows, _ := h.ScanFull()
	result := fmt.Sprintf("Таблица: %s (%d строк)\n", h.Schema.Name, len(rows))
	for i, row := range rows {
		result += fmt.Sprintf("  [%d] %v\n", i, row.Values)
	}
	return result
}