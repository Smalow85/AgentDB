package storage

import (
	"container/list"
	"fmt"
	"sync"
)

// Frame — фрейм в буферном пуле
type Frame struct {
	Page     *Page
	PinCount int
	LRUNode  *list.Element
}

// BufferPool кэширует страницы в памяти
type BufferPool struct {
	frames   []*Frame
	pageMap  map[uint64]int // PageID -> индекс фрейма
	lruList  *list.List
	disk     *DiskManager
	capacity int
	mu       sync.Mutex
}

// NewBufferPool создаёт пул заданной ёмкости
func NewBufferPool(capacity int, disk *DiskManager) *BufferPool {
	bp := &BufferPool{
		frames:   make([]*Frame, capacity),
		pageMap:  make(map[uint64]int),
		lruList:  list.New(),
		disk:     disk,
		capacity: capacity,
	}

	// Инициализируем пустые фреймы
	for i := 0; i < capacity; i++ {
		bp.frames[i] = &Frame{}
	}

	return bp
}

// FetchPage получает страницу: из кэша или с диска
func (bp *BufferPool) FetchPage(pageID uint64) (*Page, error) {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	// 1. Проверяем, есть ли уже в кэше
	if idx, ok := bp.pageMap[pageID]; ok {
		frame := bp.frames[idx]
		frame.PinCount++
		bp.lruList.MoveToFront(frame.LRUNode)
		return frame.Page, nil
	}

	// 2. Ищем свободный фрейм или жертву
	frameIdx := bp.findFreeFrame()
	if frameIdx == -1 {
		if err := bp.evictPage(); err != nil {
			return nil, err
		}
		frameIdx = bp.findFreeFrame()
	}

	// 3. Загружаем страницу с диска
	page, err := bp.disk.ReadPage(pageID)
	if err != nil {
		return nil, err
	}

	// 4. Обновляем фрейм
	frame := bp.frames[frameIdx]
	if frame.Page != nil {
		delete(bp.pageMap, frame.Page.ID)
	}
	frame.Page = page
	frame.PinCount = 1
	frame.LRUNode = bp.lruList.PushFront(frameIdx)
	bp.pageMap[pageID] = frameIdx

	return page, nil
}

// UnpinPage освобождает страницу
func (bp *BufferPool) UnpinPage(pageID uint64, dirty bool) {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	idx, ok := bp.pageMap[pageID]
	if !ok {
		return
	}

	frame := bp.frames[idx]
	if frame.PinCount > 0 {
		frame.PinCount--
	}
	if dirty {
		frame.Page.Dirty = true
	}
}

// FlushAll сбрасывает все грязные страницы на диск
func (bp *BufferPool) FlushAll() error {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	for i, frame := range bp.frames {
		if frame.Page != nil && frame.Page.Dirty {
			if err := bp.disk.WritePage(frame.Page); err != nil {
				return fmt.Errorf("ошибка сброса фрейма %d: %w", i, err)
			}
		}
	}
	return nil
}

// findFreeFrame ищет незанятый фрейм
func (bp *BufferPool) findFreeFrame() int {
	for i, frame := range bp.frames {
		if frame.Page == nil {
			return i
		}
	}
	return -1
}

// evictPage вытесняет страницу по LRU
func (bp *BufferPool) evictPage() error {
	// Ищем unpinned страницу с конца списка
	for e := bp.lruList.Back(); e != nil; e = e.Prev() {
		idx := e.Value.(int)
		frame := bp.frames[idx]

		if frame.PinCount == 0 {
			// Сбрасываем на диск, если грязная
			if frame.Page.Dirty {
				if err := bp.disk.WritePage(frame.Page); err != nil {
					return err
				}
			}

			// Удаляем из map и списка
			delete(bp.pageMap, frame.Page.ID)
			bp.lruList.Remove(e)
			frame.Page = nil
			frame.LRUNode = nil
			return nil
		}
	}

	return fmt.Errorf("все страницы заняты (pinned)")
}