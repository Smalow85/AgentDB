package index

import (
	"encoding/binary"
	"fmt"
)

const (
	MaxKeys = 200 // примерно под страницу 4KB
)

// NodeType — тип узла
type NodeType byte

const (
	InternalNode NodeType = iota
	LeafNode
)

// Node — узел B+Tree (помещается в страницу 4KB)
type Node struct {
	Type     NodeType
	Keys     []int64          // ключи (всегда int64 для простоты)
	Children []uint64         // указатели на дочерние страницы (для internal)
	Values   [][]byte         // значения (для leaf — RID или данные)
	Next     uint64           // указатель на следующий лист (для leaf)
	PageID   uint64           // ID страницы, где сохранён узел
}

// NewNode создаёт новый узел
func NewNode(pageID uint64, nodeType NodeType) *Node {
	return &Node{
		Type:     nodeType,
		Keys:     make([]int64, 0, MaxKeys),
		Children: make([]uint64, 0, MaxKeys+1),
		Values:   make([][]byte, 0, MaxKeys),
		PageID:   pageID,
		Next:     0,
	}
}

// IsFull проверяет, заполнен ли узел
func (n *Node) IsFull() bool {
	return len(n.Keys) >= MaxKeys
}

// IsLeaf проверяет, является ли узел листом
func (n *Node) IsLeaf() bool {
	return n.Type == LeafNode
}

// FindKey находит позицию ключа в узле (бинарный поиск)
func (n *Node) FindKey(key int64) int {
	left, right := 0, len(n.Keys)-1
	for left <= right {
		mid := (left + right) / 2
		if n.Keys[mid] == key {
			return mid
		}
		if n.Keys[mid] < key {
			left = mid + 1
		} else {
			right = mid - 1
		}
	}
	return left // позиция для вставки
}

// InsertKey вставляет ключ в узел (сохраняя порядок)
func (n *Node) InsertKey(key int64, child uint64, value []byte) {
	pos := n.FindKey(key)

	// Вставляем ключ
	n.Keys = append(n.Keys, 0)
	copy(n.Keys[pos+1:], n.Keys[pos:])
	n.Keys[pos] = key

	if n.IsLeaf() {
		// Вставляем значение
		n.Values = append(n.Values, nil)
		copy(n.Values[pos+1:], n.Values[pos:])
		n.Values[pos] = value
	} else {
		// Вставляем указатель на потомка
		n.Children = append(n.Children, 0)
		copy(n.Children[pos+2:], n.Children[pos+1:])
		n.Children[pos+1] = child
	}
}

// SplitNode разделяет узел пополам
// Возвращает: (новый узел, ключ-разделитель для родителя)
func (n *Node) SplitNode(newPageID uint64) (*Node, int64) {
	mid := len(n.Keys) / 2
	splitKey := n.Keys[mid]

	newNode := NewNode(newPageID, n.Type)

	if n.IsLeaf() {
		// Листья: копируем правую половину
		newNode.Keys = append(newNode.Keys, n.Keys[mid:]...)
		newNode.Values = append(newNode.Values, n.Values[mid:]...)
		newNode.Next = n.Next

		// Обновляем связи
		n.Keys = n.Keys[:mid]
		n.Values = n.Values[:mid]
		n.Next = newPageID
	} else {
		// Внутренний узел: ключ-разделитель уходит в родителя
		newNode.Keys = append(newNode.Keys, n.Keys[mid+1:]...)
		newNode.Children = append(newNode.Children, n.Children[mid+1:]...)

		n.Keys = n.Keys[:mid]
		n.Children = n.Children[:mid+1]
	}

	return newNode, splitKey
}

// Serialize сериализует узел в байты (для сохранения в страницу)
func (n *Node) Serialize() []byte {
	buf := make([]byte, PageSize)

	pos := 0
	buf[pos] = byte(n.Type)
	pos++

	// Количество ключей
	binary.LittleEndian.PutUint16(buf[pos:pos+2], uint16(len(n.Keys)))
	pos += 2

	// Next (для листьев)
	binary.LittleEndian.PutUint64(buf[pos:pos+8], n.Next)
	pos += 8

	// Ключи
	for _, k := range n.Keys {
		binary.LittleEndian.PutUint64(buf[pos:pos+8], uint64(k))
		pos += 8
	}

	// Children (для внутренних)
	if !n.IsLeaf() {
		for _, c := range n.Children {
			binary.LittleEndian.PutUint64(buf[pos:pos+8], c)
			pos += 8
		}
	}

	// Values (для листьев)
	if n.IsLeaf() {
		for _, v := range n.Values {
			// Длина значения + значение
			binary.LittleEndian.PutUint16(buf[pos:pos+2], uint16(len(v)))
			pos += 2
			copy(buf[pos:], v)
			pos += len(v)
		}
	}

	return buf
}

// DeserializeNode восстанавливает узел из байтов
func DeserializeNode(data []byte, pageID uint64) (*Node, error) {
	if len(data) < 3 {
		return nil, fmt.Errorf("недостаточно данных для узла")
	}

	nodeType := NodeType(data[0])
	keyCount := binary.LittleEndian.Uint16(data[1:3])
	next := binary.LittleEndian.Uint64(data[3:11])

	node := &Node{
		Type:   nodeType,
		Keys:   make([]int64, keyCount),
		PageID: pageID,
		Next:   next,
	}

	pos := 11

	// Ключи
	for i := 0; i < int(keyCount); i++ {
		node.Keys[i] = int64(binary.LittleEndian.Uint64(data[pos : pos+8]))
		pos += 8
	}

	// Children
	if nodeType == InternalNode {
		node.Children = make([]uint64, keyCount+1)
		for i := 0; i < int(keyCount)+1; i++ {
			node.Children[i] = binary.LittleEndian.Uint64(data[pos : pos+8])
			pos += 8
		}
	}

	// Values
	if nodeType == LeafNode {
		node.Values = make([][]byte, keyCount)
		for i := 0; i < int(keyCount); i++ {
			valLen := binary.LittleEndian.Uint16(data[pos : pos+2])
			pos += 2
			node.Values[i] = make([]byte, valLen)
			copy(node.Values[i], data[pos:pos+int(valLen)])
			pos += int(valLen)
		}
	}

	return node, nil
}

// PageSize для индекса (4KB как и у storage)
const PageSize = 4096