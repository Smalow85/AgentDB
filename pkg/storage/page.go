package storage

import (
	"encoding/binary"
	"fmt"
)

const (
	PageSize     = 4096
	HeaderSize   = 24
	SlotSize     = 4
	MaxSlots     = (PageSize - HeaderSize) / SlotSize
)

// Page — 4KB
type Page struct {
	Data [PageSize]byte
	ID   uint64
	Dirty bool
}

// PageHeader — метаданные страницы (первые 24 байта)
type PageHeader struct {
	PageID       uint64
	TableID      uint32
	FreeSpaceOff uint16
	FreeSpaceEnd uint16
	RowCount     uint16
	Flags        uint16
	Checksum     uint32
}

// Slot — 4 байта: смещение и длина строки
type Slot struct {
	Offset uint16
	Length uint16
}

// NewPage создаёт новую пустую страницу
func NewPage(pageID uint64) *Page {
	p := &Page{ID: pageID}
	header := PageHeader{
		PageID:       pageID,
		FreeSpaceOff: HeaderSize,           // начинается сразу после заголовка
		FreeSpaceEnd: PageSize,             // заканчивается в конце страницы
	}
	p.writeHeader(&header)
	return p
}

// ReadHeader читает заголовок страницы
func (p *Page) ReadHeader() *PageHeader {
	return &PageHeader{
		PageID:       binary.LittleEndian.Uint64(p.Data[0:8]),
		TableID:      binary.LittleEndian.Uint32(p.Data[8:12]),
		FreeSpaceOff: binary.LittleEndian.Uint16(p.Data[12:14]),
		FreeSpaceEnd: binary.LittleEndian.Uint16(p.Data[14:16]),
		RowCount:     binary.LittleEndian.Uint16(p.Data[16:18]),
		Flags:        binary.LittleEndian.Uint16(p.Data[18:20]),
		Checksum:     binary.LittleEndian.Uint32(p.Data[20:24]),
	}
}

// writeHeader записывает заголовок в страницу
func (p *Page) writeHeader(h *PageHeader) {
	binary.LittleEndian.PutUint64(p.Data[0:8], h.PageID)
	binary.LittleEndian.PutUint32(p.Data[8:12], h.TableID)
	binary.LittleEndian.PutUint16(p.Data[12:14], h.FreeSpaceOff)
	binary.LittleEndian.PutUint16(p.Data[14:16], h.FreeSpaceEnd)
	binary.LittleEndian.PutUint16(p.Data[16:18], h.RowCount)
	binary.LittleEndian.PutUint16(p.Data[18:20], h.Flags)
	binary.LittleEndian.PutUint32(p.Data[20:24], h.Checksum)
}

// GetSlot читает слот по индексу
func (p *Page) GetSlot(slotID uint16) *Slot {
	offset := HeaderSize + slotID*SlotSize
	return &Slot{
		Offset: binary.LittleEndian.Uint16(p.Data[offset : offset+2]),
		Length: binary.LittleEndian.Uint16(p.Data[offset+2 : offset+4]),
	}
}

// setSlot записывает слот по индексу
func (p *Page) setSlot(slotID uint16, s *Slot) {
	offset := HeaderSize + slotID*SlotSize
	binary.LittleEndian.PutUint16(p.Data[offset:offset+2], s.Offset)
	binary.LittleEndian.PutUint16(p.Data[offset+2:offset+4], s.Length)
}

// FreeSpace возвращает количество свободного места на странице
func (p *Page) FreeSpace() uint16 {
	h := p.ReadHeader()
	free := int(h.FreeSpaceEnd) - int(h.FreeSpaceOff)
	if free < 0 {
		return 0
	}
	return uint16(free)
}

// CanFit проверяет, поместится ли строка
func (p *Page) CanFit(rowSize uint16) bool {
	// Нужно место для данных + слота
	needed := rowSize + SlotSize
	return p.FreeSpace() >= needed
}

// InsertRow вставляет строку на страницу, возвращает slotID
func (p *Page) InsertRow(data []byte) (uint16, error) {
	if len(data) > 65535 {
		return 0, fmt.Errorf("строка слишком длинная: %d байт", len(data))
	}

	rowSize := uint16(len(data))

	if !p.CanFit(rowSize) {
		return 0, fmt.Errorf("недостаточно места на странице")
	}

	h := p.ReadHeader()
	slotID := h.RowCount

	// Вычисляем, где разместить данные
	newEnd := h.FreeSpaceEnd - rowSize
	copy(p.Data[newEnd:newEnd+rowSize], data)

	// Записываем слот
	slot := &Slot{
		Offset: newEnd,
		Length: rowSize,
	}
	p.setSlot(slotID, slot)

	// Обновляем заголовок
	h.RowCount++
	h.FreeSpaceEnd = newEnd
	h.FreeSpaceOff = HeaderSize + h.RowCount*SlotSize
	p.writeHeader(h)
	p.Dirty = true

	return slotID, nil
}

// GetRow читает строку по slotID
func (p *Page) GetRow(slotID uint16) ([]byte, error) {
	h := p.ReadHeader()
	if slotID >= h.RowCount {
		return nil, fmt.Errorf("слот %d не существует (всего %d строк)", slotID, h.RowCount)
	}

	slot := p.GetSlot(slotID)
	if slot.Length == 0 {
		return nil, fmt.Errorf("строка %d удалена", slotID)
	}

	row := make([]byte, slot.Length)
	copy(row, p.Data[slot.Offset:slot.Offset+slot.Length])
	return row, nil
}

// DeleteRow помечает строку как удалённую (длина = 0)
func (p *Page) DeleteRow(slotID uint16) error {
	h := p.ReadHeader()
	if slotID >= h.RowCount {
		return fmt.Errorf("слот не существует")
	}

	p.setSlot(slotID, &Slot{Offset: 0, Length: 0})
	p.Dirty = true
	return nil
}

// String возвращает дамп страницы для отладки
func (p *Page) String() string {
	h := p.ReadHeader()
	return fmt.Sprintf("Page[%d] rows=%d free=%d (off=%d end=%d)",
		h.PageID, h.RowCount, p.FreeSpace(), h.FreeSpaceOff, h.FreeSpaceEnd)
}