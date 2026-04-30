package index

import (
	"fmt"
	"testing"
)

// MockDiskManager — мок для тестов
type MockDiskManager struct {
	pages map[uint64][]byte
	nextID uint64
}

func NewMockDisk() *MockDiskManager {
	return &MockDiskManager{
		pages:  make(map[uint64][]byte),
		nextID: 1,
	}
}

func (m *MockDiskManager) AllocatePage() uint64 {
	id := m.nextID
	m.nextID++
	m.pages[id] = make([]byte, PageSize)
	return id
}

func (m *MockDiskManager) ReadPage(pageID uint64) ([]byte, error) {
	data, ok := m.pages[pageID]
	if !ok {
		return make([]byte, PageSize), nil
	}
	return data, nil
}

func (m *MockDiskManager) WritePage(pageID uint64, data []byte) error {
	m.pages[pageID] = data
	return nil
}

func TestBTreeInsertAndSearch(t *testing.T) {
	mock := NewMockDisk()
	tree := NewBTree(mock)

	// Вставляем значения
	testData := []struct {
		key   int64
		value string
	}{
		{10, "ten"},
		{20, "twenty"},
		{5, "five"},
		{15, "fifteen"},
		{25, "twenty-five"},
		{1, "one"},
		{30, "thirty"},
	}

	for _, d := range testData {
		err := tree.Insert(d.key, []byte(d.value))
		if err != nil {
			t.Fatalf("Insert(%d): %v", d.key, err)
		}
	}

	tree.Print()

	// Проверяем поиск
	for _, d := range testData {
		val, found, err := tree.Search(d.key)
		if err != nil {
			t.Fatalf("Search(%d): %v", d.key, err)
		}
		if !found {
			t.Errorf("Search(%d): не найден", d.key)
		}
		if string(val) != d.value {
			t.Errorf("Search(%d): ожидалось '%s', получено '%s'", d.key, d.value, string(val))
		}
	}

	// Поиск отсутствующего
	_, found, _ := tree.Search(999)
	if found {
		t.Error("Ключ 999 не должен существовать")
	}
}

func TestBTreeInsertMany(t *testing.T) {
	mock := NewMockDisk()
	tree := NewBTree(mock)

	// Вставляем 1000 значений (тест на разделение)
	for i := int64(1); i <= 1000; i++ {
		err := tree.Insert(i, []byte(fmt.Sprintf("val-%d", i)))
		if err != nil {
			t.Fatalf("Insert(%d): %v", i, err)
		}
	}

	// Проверяем поиск
	for i := int64(1); i <= 1000; i++ {
		val, found, _ := tree.Search(i)
		if !found {
			t.Errorf("Search(%d): не найден", i)
		}
		expected := fmt.Sprintf("val-%d", i)
		if string(val) != expected {
			t.Errorf("Search(%d): '%s' != '%s'", i, string(val), expected)
		}
	}

	fmt.Printf("Вставлено 1000 значений. Размер страниц: %d\n", len(mock.pages))
	tree.Print()
}

func TestBTreeScan(t *testing.T) {
	mock := NewMockDisk()
	tree := NewBTree(mock)

	for i := int64(1); i <= 100; i++ {
		tree.Insert(i, []byte(fmt.Sprintf("val-%d", i)))
	}

	// Сканируем диапазон
	results, err := tree.Scan(20, 30)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 11 {
		t.Errorf("Scan(20,30): ожидалось 11 значений, получено %d", len(results))
	}

	for i, val := range results {
		expected := fmt.Sprintf("val-%d", 20+i)
		if string(val) != expected {
			t.Errorf("result[%d]: '%s' != '%s'", i, string(val), expected)
		}
	}
}