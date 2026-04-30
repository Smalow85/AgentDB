package storage

import (
	"testing"
)

func TestNewPage(t *testing.T) {
	page := NewPage(1)
	header := page.ReadHeader()

	if header.PageID != 1 {
		t.Errorf("PageID: ожидалось 1, получено %d", header.PageID)
	}
	if page.FreeSpace() != PageSize-HeaderSize {
		t.Errorf("Свободное место: ожидалось %d, получено %d", PageSize-HeaderSize, page.FreeSpace())
	}
	if header.RowCount != 0 {
		t.Errorf("RowCount: ожидалось 0, получено %d", header.RowCount)
	}
}

func TestPageInsertAndRead(t *testing.T) {
	page := NewPage(1)

	data := []byte("Hello, World!")
	slotID, err := page.InsertRow(data)
	if err != nil {
		t.Fatalf("InsertRow: %v", err)
	}

	if slotID != 0 {
		t.Errorf("SlotID: ожидалось 0, получено %d", slotID)
	}

	readData, err := page.GetRow(slotID)
	if err != nil {
		t.Fatalf("GetRow: %v", err)
	}

	if string(readData) != string(data) {
		t.Errorf("Данные не совпадают: '%s' vs '%s'", data, readData)
	}
}

func TestPageMultipleInserts(t *testing.T) {
	page := NewPage(1)

	rows := [][]byte{
		[]byte("First"),
		[]byte("Second"),
		[]byte("Third"),
	}

	for i, data := range rows {
		slotID, err := page.InsertRow(data)
		if err != nil {
			t.Fatalf("InsertRow %d: %v", i, err)
		}
		if slotID != uint16(i) {
			t.Errorf("Ожидался slotID %d, получен %d", i, slotID)
		}
	}

	header := page.ReadHeader()
	if header.RowCount != 3 {
		t.Errorf("RowCount: ожидалось 3, получено %d", header.RowCount)
	}

	// Проверяем, что данные читаются в обратном порядке (LIFO для данных)
	for i, expected := range rows {
		readData, err := page.GetRow(uint16(i))
		if err != nil {
			t.Fatalf("GetRow %d: %v", i, err)
		}
		if string(readData) != string(expected) {
			t.Errorf("Строка %d: '%s' vs '%s'", i, expected, readData)
		}
	}
}

func TestPageDelete(t *testing.T) {
	page := NewPage(1)

	data := []byte("To be deleted")
	slotID, _ := page.InsertRow(data)

	err := page.DeleteRow(slotID)
	if err != nil {
		t.Fatalf("DeleteRow: %v", err)
	}

	_, err = page.GetRow(slotID)
	if err == nil {
		t.Error("GetRow должен вернуть ошибку для удалённой строки")
	}
}

func TestPageFull(t *testing.T) {
	page := NewPage(1)

	// Максимальный размер строки
	bigData := make([]byte, PageSize-HeaderSize-SlotSize-1)
	for i := range bigData {
		bigData[i] = 'A'
	}

	_, err := page.InsertRow(bigData)
	if err != nil {
		t.Fatalf("Должна помещаться строка размером %d: %v", len(bigData), err)
	}

	// Вторая не должна поместиться
	_, err = page.InsertRow([]byte("X"))
	if err == nil {
		t.Error("Ожидалась ошибка при заполненной странице")
	}
}