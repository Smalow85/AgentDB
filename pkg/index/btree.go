package index

import (
	"fmt"
	"sync"
)

// BTree — B+Tree индекс
type BTree struct {
	Root     uint64          // PageID корневого узла
	Disk     DiskManager     // интерфейс для чтения/записи страниц
	mu       sync.RWMutex
	nodePool map[uint64]*Node // кэш узлов в памяти
}

// DiskManager — интерфейс, который должен предоставить storage
type DiskManager interface {
	AllocatePage() uint64
	ReadPage(pageID uint64) ([]byte, error)
	WritePage(pageID uint64, data []byte) error
}

// NewBTree создаёт новое B+Tree
func NewBTree(disk DiskManager) *BTree {
	bt := &BTree{
		Root:     0,
		Disk:     disk,
		nodePool: make(map[uint64]*Node),
	}

	// Создаём корневой лист
	rootPage := disk.AllocatePage()
	root := NewNode(rootPage, LeafNode)
	bt.saveNode(root)
	bt.Root = rootPage

	return bt
}

// getNode загружает узел (из кэша или с диска)
func (bt *BTree) getNode(pageID uint64) (*Node, error) {
	// Проверяем кэш
	if node, ok := bt.nodePool[pageID]; ok {
		return node, nil
	}

	// Загружаем с диска
	data, err := bt.Disk.ReadPage(pageID)
	if err != nil {
		return nil, err
	}

	node, err := DeserializeNode(data, pageID)
	if err != nil {
		return nil, err
	}

	bt.nodePool[pageID] = node
	return node, nil
}

// saveNode сохраняет узел на диск
func (bt *BTree) saveNode(node *Node) error {
	data := node.Serialize()
	err := bt.Disk.WritePage(node.PageID, data)
	if err == nil {
		bt.nodePool[node.PageID] = node
	}
	return err
}

// Insert вставляет ключ и значение в дерево
func (bt *BTree) Insert(key int64, value []byte) error {
	bt.mu.Lock()
	defer bt.mu.Unlock()

	// Если дерево пустое
	if bt.Root == 0 {
		rootPage := bt.Disk.AllocatePage()
		root := NewNode(rootPage, LeafNode)
		root.InsertKey(key, 0, value)
		bt.Root = rootPage
		return bt.saveNode(root)
	}

	// Рекурсивная вставка
	newChild, splitKey, err := bt.insertRecursive(bt.Root, key, value)
	if err != nil {
		return err
	}

	// Если корень разделился — создаём новый корень
	if newChild != 0 {
		newRootPage := bt.Disk.AllocatePage()
		newRoot := NewNode(newRootPage, InternalNode)
		newRoot.Keys = append(newRoot.Keys, splitKey)
		newRoot.Children = append(newRoot.Children, bt.Root, newChild)
		bt.Root = newRootPage
		bt.saveNode(newRoot)
	}

	return nil
}

// insertRecursive рекурсивно вставляет ключ
// Возвращает (новая дочерняя страница, ключ-разделитель, ошибка)
func (bt *BTree) insertRecursive(pageID uint64, key int64, value []byte) (uint64, int64, error) {
	node, err := bt.getNode(pageID)
	if err != nil {
		return 0, 0, err
	}

	// Если лист — вставляем напрямую
	if node.IsLeaf() {
		// Проверяем, нет ли уже такого ключа (упрощённо — заменяем)
		pos := node.FindKey(key)
		if pos < len(node.Keys) && node.Keys[pos] == key {
			node.Values[pos] = value
			return 0, 0, bt.saveNode(node)
		}

		node.InsertKey(key, 0, value)

		// Если не переполнен — сохраняем и выходим
		if !node.IsFull() {
			return 0, 0, bt.saveNode(node)
		}

		// Разделяем лист
		newPageID := bt.Disk.AllocatePage()
		newNode, splitKey := node.SplitNode(newPageID)

		bt.saveNode(node)
		bt.saveNode(newNode)
		return newPageID, splitKey, nil
	}

	// Внутренний узел — идём в нужного потомка
	childIdx := node.FindKey(key)
	if childIdx < len(node.Children) {
		// Безопасно проверяем границы
	} else {
		childIdx = len(node.Children) - 1
	}

	childPageID := node.Children[childIdx]
	newChildPage, splitKey, err := bt.insertRecursive(childPageID, key, value)
	if err != nil {
		return 0, 0, err
	}

	// Если потомок разделился
	if newChildPage != 0 {
		node.InsertKey(splitKey, newChildPage, nil)

		if !node.IsFull() {
			return 0, 0, bt.saveNode(node)
		}

		// Разделяем внутренний узел
		newPageID := bt.Disk.AllocatePage()
		newNode, splitKey := node.SplitNode(newPageID)

		bt.saveNode(node)
		bt.saveNode(newNode)
		return newPageID, splitKey, nil
	}

	return 0, 0, bt.saveNode(node)
}

// Search ищет ключ в дереве
func (bt *BTree) Search(key int64) ([]byte, bool, error) {
	bt.mu.RLock()
	defer bt.mu.RUnlock()

	return bt.searchRecursive(bt.Root, key)
}

func (bt *BTree) searchRecursive(pageID uint64, key int64) ([]byte, bool, error) {
	if pageID == 0 {
		return nil, false, nil
	}

	node, err := bt.getNode(pageID)
	if err != nil {
		return nil, false, err
	}

	pos := node.FindKey(key)

	if node.IsLeaf() {
		if pos < len(node.Keys) && node.Keys[pos] == key {
			return node.Values[pos], true, nil
		}
		return nil, false, nil
	}

	// Внутренний узел — идём в потомка
	if pos >= len(node.Children) {
		pos = len(node.Children) - 1
	}
	if pos < len(node.Keys) && node.Keys[pos] <= key {
		// Идём в правого потомка если ключ >= текущего
		if pos+1 < len(node.Children) {
			return bt.searchRecursive(node.Children[pos+1], key)
		}
	}
	return bt.searchRecursive(node.Children[pos], key)
}

// Scan возвращает все значения в диапазоне [startKey, endKey]
func (bt *BTree) Scan(startKey, endKey int64) ([][]byte, error) {
	bt.mu.RLock()
	defer bt.mu.RUnlock()

	var result [][]byte

	// Находим первый лист
	leaf, err := bt.findLeaf(bt.Root, startKey)
	if err != nil {
		return nil, err
	}
	if leaf == nil {
		return result, nil
	}

	// Идём по листьям через next указатели
	for leaf != nil {
		for i, key := range leaf.Keys {
			if key > endKey {
				return result, nil
			}
			if key >= startKey {
				result = append(result, leaf.Values[i])
			}
		}

		if leaf.Next == 0 {
			break
		}
		leaf, err = bt.getNode(leaf.Next)
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

// findLeaf находит лист, содержащий ключ
func (bt *BTree) findLeaf(pageID uint64, key int64) (*Node, error) {
	if pageID == 0 {
		return nil, nil
	}

	node, err := bt.getNode(pageID)
	if err != nil {
		return nil, err
	}

	if node.IsLeaf() {
		return node, nil
	}

	pos := node.FindKey(key)
	if pos >= len(node.Children) {
		pos = len(node.Children) - 1
	}

	return bt.findLeaf(node.Children[pos], key)
}

// Print отладочная печать дерева
func (bt *BTree) Print() {
	fmt.Println("=== B+Tree ===")
	bt.printNode(bt.Root, "")
	fmt.Println("==============")
}

func (bt *BTree) printNode(pageID uint64, indent string) {
	if pageID == 0 {
		return
	}

	node, _ := bt.getNode(pageID)
	typeStr := "LEAF"
	if !node.IsLeaf() {
		typeStr = "INT"
	}

	fmt.Printf("%s[%s] Page=%d Keys=%v Next=%d\n", indent, typeStr, pageID, node.Keys, node.Next)

	if !node.IsLeaf() {
		for _, child := range node.Children {
			bt.printNode(child, indent+"  ")
		}
	}
}