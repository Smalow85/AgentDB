package storage

type Row map[string]interface{}

type StorageEngine interface {
	Insert(table string, row Row) error
	Scan(table string) ([]Row, error)
	Get(table string, key interface{}) (Row, error)
}

type HeapStorage struct {
	Tables map[string][]Row
}

func NewHeap() *HeapStorage {
	return &HeapStorage{Tables: make(map[string][]Row)}
}

func (h *HeapStorage) Insert(table string, row Row) error {
	h.Tables[table] = append(h.Tables[table], row)
	return nil
}

func (h *HeapStorage) Scan(table string) ([]Row, error) {
	return h.Tables[table], nil
}

func (h *HeapStorage) Get(table string, key interface{}) (Row, error) {
	return nil, nil
}
