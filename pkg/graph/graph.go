package graph

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"agent-db/pkg/storage"
)

// Direction для обхода
type Direction int

const (
	NodePageStart               = 1
	EdgePageStart               = 1000
	DirectionOutgoing Direction = iota
	DirectionIncoming
	DirectionBoth
)

// Query для поиска узлов
type Query struct {
	Label    string
	Property string
	Value    interface{}
}

type Reference struct {
	ID         int64
	SourceID   int64  // кто ссылается (call)
	TargetID   int64  // на что ссылается (function)
	Type       string // "call", "field_access", "type_ref", "inherit"
	IsResolved bool   // true если цель найдена
	Properties map[string]interface{}
}

// Graph — граф в памяти
type Graph struct {
	Name       string
	Store      *GraphStore
	nextNodeID int64
	nextEdgeID int64

	// Индексы в памяти
	nodeByID    map[int64]*Node
	edgeByID    map[int64]*Edge
	nodeByLabel map[string][]int64
	nodeByProp  map[string]map[string][]int64
	edgeByType  map[string][]int64
	edgeByFrom  map[int64][]int64
	edgeByTo    map[int64][]int64
	refByID     map[int64]*Reference
	refBySource map[int64][]int64 // source_id → ref_ids
	refByTarget map[int64][]int64 // target_id → ref_ids
	nextRefID   int64

	mu sync.RWMutex
}

// NewGraph создаёт новый граф
func NewGraph(name string, store *GraphStore) *Graph {
	return &Graph{
		Name:        name,
		Store:       store,
		nextNodeID:  1,
		nextEdgeID:  1,
		nodeByID:    make(map[int64]*Node),
		edgeByID:    make(map[int64]*Edge),
		nodeByLabel: make(map[string][]int64),
		nodeByProp:  make(map[string]map[string][]int64),
		edgeByType:  make(map[string][]int64),
		edgeByFrom:  make(map[int64][]int64),
		edgeByTo:    make(map[int64][]int64),
		refByID:     make(map[int64]*Reference),
		refBySource: make(map[int64][]int64),
		refByTarget: make(map[int64][]int64),
		nextRefID:   1,
	}
}

// AddNode добавляет узел в граф
func (g *Graph) AddNode(labels []string, props map[string]interface{}) (*Node, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	node := NewNode(g.nextNodeID, labels, props)
	g.nextNodeID++

	// Сохраняем на диск
	if err := g.Store.PutNode(node); err != nil {
		return nil, fmt.Errorf("ошибка сохранения узла: %w", err)
	}

	// Обновляем индексы
	g.nodeByID[node.ID] = node
	for _, label := range node.Labels {
		g.nodeByLabel[label] = append(g.nodeByLabel[label], node.ID)
	}
	for key, val := range node.Properties {
		valStr := fmt.Sprintf("%v", val)
		if g.nodeByProp[key] == nil {
			g.nodeByProp[key] = make(map[string][]int64)
		}
		g.nodeByProp[key][valStr] = append(g.nodeByProp[key][valStr], node.ID)
	}

	return node, nil
}

// AddEdge добавляет связь между узлами
func (g *Graph) AddEdge(edgeType string, fromID, toID int64) (*Edge, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Проверяем существование узлов
	if _, ok := g.nodeByID[fromID]; !ok {
		return nil, fmt.Errorf("узел %d не найден", fromID)
	}
	if _, ok := g.nodeByID[toID]; !ok {
		return nil, fmt.Errorf("узел %d не найден", toID)
	}

	edge := NewEdge(g.nextEdgeID, edgeType, fromID, toID)
	g.nextEdgeID++

	// ТОЛЬКО обновляем индексы в памяти (не сохраняем сразу на диск!)
	g.edgeByID[edge.ID] = edge
	g.edgeByType[edge.Type] = append(g.edgeByType[edge.Type], edge.ID)
	g.edgeByFrom[fromID] = append(g.edgeByFrom[fromID], edge.ID)
	g.edgeByTo[toID] = append(g.edgeByTo[toID], edge.ID)

	return edge, nil
}

// FindNodes ищет узлы по запросу
func (g *Graph) FindNodes(q Query) []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var result []*Node

	// Если запрос пустой - возвращаем все узлы
	if q.Label == "" && q.Property == "" {
		for _, node := range g.nodeByID {
			result = append(result, node)
		}
		return result
	}

	if q.Label != "" && q.Property != "" {
		// Поиск по метке И свойству
		valStr := fmt.Sprintf("%v", q.Value)
		ids := g.nodeByProp[q.Property][valStr]
		for _, id := range ids {
			if node := g.nodeByID[id]; node != nil && node.HasLabel(q.Label) {
				result = append(result, node)
			}
		}
	} else if q.Label != "" {
		// Поиск только по метке
		for _, id := range g.nodeByLabel[q.Label] {
			if node := g.nodeByID[id]; node != nil {
				result = append(result, node)
			}
		}
	} else if q.Property != "" {
		// Поиск только по свойству
		valStr := fmt.Sprintf("%v", q.Value)
		for _, id := range g.nodeByProp[q.Property][valStr] {
			if node := g.nodeByID[id]; node != nil {
				result = append(result, node)
			}
		}
	}

	return result
}

// GetNode возвращает узел по ID
func (g *Graph) GetNode(id int64) *Node {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.nodeByID[id]
}

// GetEdges возвращает связи узла
func (g *Graph) GetEdges(nodeID int64, dir Direction) []*Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var result []*Edge

	if dir == DirectionOutgoing || dir == DirectionBoth {
		for _, edgeID := range g.edgeByFrom[nodeID] {
			if edge := g.edgeByID[edgeID]; edge != nil {
				result = append(result, edge)
			}
		}
	}

	if dir == DirectionIncoming || dir == DirectionBoth {
		for _, edgeID := range g.edgeByTo[nodeID] {
			if edge := g.edgeByID[edgeID]; edge != nil {
				result = append(result, edge)
			}
		}
	}

	return result
}

// GetNeighbors возвращает соседние узлы
func (g *Graph) GetNeighbors(nodeID int64, dir Direction) []*Node {
	edges := g.GetEdges(nodeID, dir)
	var nodes []*Node
	seen := make(map[int64]bool)

	for _, edge := range edges {
		neighborID := edge.ToID
		if edge.FromID != nodeID {
			neighborID = edge.FromID
		}
		if !seen[neighborID] {
			if node := g.GetNode(neighborID); node != nil {
				nodes = append(nodes, node)
				seen[neighborID] = true
			}
		}
	}
	return nodes
}

func (g *Graph) Load() error {
	// Читаем страницу 0 (метаданные)
	page, err := g.Store.Disk.ReadPage(0)
	if err != nil {
		return err
	}

	offset := storage.HeaderSize + 24
	length := binary.LittleEndian.Uint32(page.Data[offset : offset+4])

	var meta struct {
		Name         string
		NextNodeID   int64
		NextEdgeID   int64
		NextRefID    int64
		NextNodePage uint64
		NextEdgePage uint64
	}
	json.Unmarshal(page.Data[offset+4:offset+4+int(length)], &meta)

	g.Name = meta.Name
	if meta.NextNodeID > 0 {
		g.nextNodeID = meta.NextNodeID
	}
	if meta.NextEdgeID > 0 {
		g.nextEdgeID = meta.NextEdgeID
	}
	if meta.NextRefID > 0 {
		g.nextRefID = meta.NextRefID
	}
	if meta.NextNodePage > 0 {
		g.Store.nextNodePage = meta.NextNodePage
	}
	if meta.NextEdgePage > 0 {
		g.Store.nextEdgePage = meta.NextEdgePage
	}

	// Узлы
	nodes, _ := g.Store.GetAllNodes()
	for _, node := range nodes {
		g.nodeByID[node.ID] = node
		for _, label := range node.Labels {
			g.nodeByLabel[label] = append(g.nodeByLabel[label], node.ID)
		}
		for key, val := range node.Properties {
			valStr := fmt.Sprintf("%v", val)
			if g.nodeByProp[key] == nil {
				g.nodeByProp[key] = make(map[string][]int64)
			}
			g.nodeByProp[key][valStr] = append(g.nodeByProp[key][valStr], node.ID)
		}
	}

	// Рёбра
	edges, _ := g.Store.GetAllEdges()
	for _, edge := range edges {
		if strings.HasPrefix(edge.Type, "_ref_") {
			refType := strings.TrimPrefix(edge.Type, "_ref_")
			ref := &Reference{
				ID:         edge.ID - 1000000,
				SourceID:   edge.FromID,
				TargetID:   edge.ToID,
				Type:       refType,
				IsResolved: true,
			}
			if props, ok := edge.Properties["is_resolved"].(bool); ok {
				ref.IsResolved = props
			}
			g.refByID[ref.ID] = ref
			g.refBySource[ref.SourceID] = append(g.refBySource[ref.SourceID], ref.ID)
			g.refByTarget[ref.TargetID] = append(g.refByTarget[ref.TargetID], ref.ID)
		} else {
			g.edgeByID[edge.ID] = edge
			g.edgeByType[edge.Type] = append(g.edgeByType[edge.Type], edge.ID)
			g.edgeByFrom[edge.FromID] = append(g.edgeByFrom[edge.FromID], edge.ID)
			g.edgeByTo[edge.ToID] = append(g.edgeByTo[edge.ToID], edge.ID)
		}
	}

	fmt.Printf("✓ Загружен граф: %d узлов, %d рёбер\n", len(g.nodeByID), len(g.edgeByID))
	return nil
}

// DeleteNode удаляет узел и все связанные с ним рёбра
func (g *Graph) DeleteNode(id int64) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.nodeByID[id] == nil {
		return fmt.Errorf("узел %d не найден", id)
	}

	// Удаляем все связанные рёбра
	edgeIDs := append(g.edgeByFrom[id], g.edgeByTo[id]...)
	for _, edgeID := range edgeIDs {
		g.deleteEdgeInternal(edgeID)
	}

	// Удаляем из индексов
	node := g.nodeByID[id]
	for _, label := range node.Labels {
		g.nodeByLabel[label] = removeID(g.nodeByLabel[label], id)
	}
	delete(g.nodeByID, id)

	// Удаляем с диска (помечаем страницу пустой)
	pageID := uint64(NodePageStart + id)
	page := storage.NewPage(pageID)
	page.Dirty = true
	return g.Store.Disk.WritePage(page)
}

// DeleteEdge удаляет связь
func (g *Graph) DeleteEdge(id int64) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.deleteEdgeInternal(id)
}

func (g *Graph) deleteEdgeInternal(id int64) error {
	edge := g.edgeByID[id]
	if edge == nil {
		return fmt.Errorf("связь %d не найдена", id)
	}

	// Удаляем из индексов
	g.edgeByType[edge.Type] = removeID(g.edgeByType[edge.Type], id)
	g.edgeByFrom[edge.FromID] = removeID(g.edgeByFrom[edge.FromID], id)
	g.edgeByTo[edge.ToID] = removeID(g.edgeByTo[edge.ToID], id)
	delete(g.edgeByID, id)

	// Удаляем с диска
	pageID := uint64(EdgePageStart + id)
	page := storage.NewPage(pageID)
	page.Dirty = true
	return g.Store.Disk.WritePage(page)
}

func removeID(ids []int64, target int64) []int64 {
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id != target {
			result = append(result, id)
		}
	}
	return result
}

// AddReference добавляет ссылку между узлами
func (g *Graph) AddReference(sourceID, targetID int64, refType string) (*Reference, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.nodeByID[sourceID] == nil {
		return nil, fmt.Errorf("узел-источник %d не найден", sourceID)
	}
	if g.nodeByID[targetID] == nil {
		return nil, fmt.Errorf("узел-цель %d не найден", targetID)
	}

	ref := &Reference{
		ID:         g.nextRefID,
		SourceID:   sourceID,
		TargetID:   targetID,
		Type:       refType,
		IsResolved: true,
	}
	g.nextRefID++

	g.refByID[ref.ID] = ref
	g.refBySource[sourceID] = append(g.refBySource[sourceID], ref.ID)
	g.refByTarget[targetID] = append(g.refByTarget[targetID], ref.ID)

	return ref, nil
}

// GetReferences возвращает ссылки узла
// dir: Incoming — кто ссылается на этот узел
// dir: Outgoing — на что ссылается этот узел
func (g *Graph) GetReferences(nodeID int64, dir Direction) []*Reference {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var result []*Reference
	var refIDs []int64

	if dir == DirectionIncoming {
		refIDs = g.refByTarget[nodeID]
	} else if dir == DirectionOutgoing {
		refIDs = g.refBySource[nodeID]
	} else {
		refIDs = append(refIDs, g.refBySource[nodeID]...)
		refIDs = append(refIDs, g.refByTarget[nodeID]...)
	}

	for _, id := range refIDs {
		if ref := g.refByID[id]; ref != nil {
			result = append(result, ref)
		}
	}
	return result
}

// GetCallers возвращает узлы, которые вызывают данный
func (g *Graph) GetCallers(nodeID int64) []*Node {
	refs := g.GetReferences(nodeID, DirectionIncoming)
	var callers []*Node
	seen := make(map[int64]bool)
	for _, ref := range refs {
		if ref.Type == "call" && !seen[ref.SourceID] {
			if node := g.GetNode(ref.SourceID); node != nil {
				callers = append(callers, node)
				seen[ref.SourceID] = true
			}
		}
	}
	return callers
}

// GetCallees возвращает узлы, которые вызываются из данного
func (g *Graph) GetCallees(nodeID int64) []*Node {
	refs := g.GetReferences(nodeID, DirectionOutgoing)
	var callees []*Node
	seen := make(map[int64]bool)
	for _, ref := range refs {
		if ref.Type == "call" && !seen[ref.TargetID] {
			if node := g.GetNode(ref.TargetID); node != nil {
				callees = append(callees, node)
				seen[ref.TargetID] = true
			}
		}
	}
	return callees
}

// ResolveCall находит function-узел по имени вызова и создаёт Reference
func (g *Graph) ResolveCall(callNodeID int64, contextNodeID int64) (*Reference, error) {
	callNode := g.GetNode(callNodeID)
	if callNode == nil {
		return nil, fmt.Errorf("узел вызова %d не найден", callNodeID)
	}

	callName, ok := callNode.GetProp("name")
	if !ok {
		return nil, fmt.Errorf("у вызова %d нет имени", callNodeID)
	}

	// Ищем function с таким именем
	// Сначала ищем в том же scope (файл → класс)
	var candidates []*Node

	// Если есть контекст (класс), ищем методы этого класса
	if contextNodeID != 0 {
		// Ищем в соседях класса
		neighbors := g.GetNeighbors(contextNodeID, DirectionOutgoing)
		for _, n := range neighbors {
			if n.HasLabel("function") {
				if name, _ := n.GetProp("name"); name == callName {
					candidates = append(candidates, n)
				}
			}
		}
	}

	// Если не нашли в контексте — ищем по всему графу
	if len(candidates) == 0 {
		candidates = g.FindNodes(Query{
			Label:    "function",
			Property: "name",
			Value:    callName,
		})
	}

	if len(candidates) > 0 {
		// Берём первого кандидата
		return g.AddReference(callNodeID, candidates[0].ID, "call")
	}

	// Не удалось разрешить — создаём неразрешённую ссылку
	ref := &Reference{
		ID:         g.nextRefID,
		SourceID:   callNodeID,
		Type:       "call",
		IsResolved: false,
	}
	g.nextRefID++
	g.refByID[ref.ID] = ref
	g.refBySource[callNodeID] = append(g.refBySource[callNodeID], ref.ID)

	return ref, nil
}

func (g *Graph) SaveToDisk() error {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// Сохраняем все узлы
	for _, node := range g.nodeByID {
		if err := g.Store.PutNode(node); err != nil {
			return err
		}
	}

	// Сохраняем все рёбра
	for _, edge := range g.edgeByID {
		if err := g.Store.PutEdge(edge); err != nil {
			return err
		}
	}

	// Сохраняем References как специальные рёбра
	for _, ref := range g.refByID {
		refEdge := &Edge{
			ID:     ref.ID + 1000000, // оффсет чтобы не пересекаться с обычными рёбрами
			Type:   "_ref_" + ref.Type,
			FromID: ref.SourceID,
			ToID:   ref.TargetID,
			Properties: map[string]interface{}{
				"is_resolved": ref.IsResolved,
			},
		}
		if err := g.Store.PutEdge(refEdge); err != nil {
			return err
		}
	}

	// Flush записывает всё на диск
	if err := g.Store.Flush(); err != nil {
		return err
	}

	// Сохраняем метаданные графа
	meta := map[string]interface{}{
		"name":           g.Name,
		"next_node_id":   g.nextNodeID,
		"next_edge_id":   g.nextEdgeID,
		"next_ref_id":    g.nextRefID,
		"next_node_page": g.Store.nextNodePage,
		"next_edge_page": g.Store.nextEdgePage,
	}
	metaJSON, _ := json.Marshal(meta)
	pageID := uint64(0)
	page := storage.NewPage(pageID)
	offset := storage.HeaderSize + 24 // skip header and slot area
	binary.LittleEndian.PutUint32(page.Data[offset:offset+4], uint32(len(metaJSON)))
	copy(page.Data[offset+4:], metaJSON)
	page.Dirty = true
	return g.Store.Disk.WritePage(page)
}

func (g *Graph) ResolveCallWithContext(callNodeID, scopeNodeID int64, contextName string) (*Reference, error) {
	callNode := g.GetNode(callNodeID)
	if callNode == nil {
		return nil, fmt.Errorf("узел вызова %d не найден", callNodeID)
	}

	callName, ok := callNode.GetProp("name")
	if !ok {
		return nil, fmt.Errorf("у вызова %d нет имени", callNodeID)
	}

	// 1. Если есть контекст (класс), ищем метод в этом классе
	if contextName != "" {
		classes := g.FindNodes(Query{Label: "class", Property: "name", Value: contextName})
		for _, class := range classes {
			methods := g.GetNeighbors(class.ID, DirectionOutgoing)
			for _, m := range methods {
				if m.HasLabel("function") {
					if name, _ := m.GetProp("name"); name == callName {
						return g.AddReference(callNodeID, m.ID, "call")
					}
				}
			}
		}
	}

	// 2. Глобальный поиск
	funcs := g.FindNodes(Query{Label: "function", Property: "name", Value: callName})
	if len(funcs) > 0 {
		return g.AddReference(callNodeID, funcs[0].ID, "call")
	}

	// 3. Неразрешённая
	ref := &Reference{
		ID:         g.nextRefID,
		SourceID:   callNodeID,
		Type:       "call",
		IsResolved: false,
	}
	g.nextRefID++
	g.refByID[ref.ID] = ref
	g.refBySource[callNodeID] = append(g.refBySource[callNodeID], ref.ID)
	return ref, nil
}

// GetCallersByName возвращает имена функций, которые вызывают функцию с заданным именем
func (g *Graph) GetCallersByName(funcName string) []string {
	calls := g.FindNodes(Query{Label: "call", Property: "name", Value: funcName})
	var callers []string
	seen := make(map[string]bool)

	for _, call := range calls {
		edges := g.GetEdges(call.ID, DirectionIncoming)
		for _, edge := range edges {
			if edge.Type == "contains" {
				parent := g.GetNode(edge.FromID)
				if parent != nil && parent.HasLabel("function") {
					name, _ := parent.GetProp("name")
					n := fmt.Sprintf("%v", name)
					if !seen[n] {
						callers = append(callers, n)
						seen[n] = true
					}
				}
			}
		}
	}
	return callers
}

// GetCalleesByName возвращает имена функций, которые вызываются из метода
func (g *Graph) GetCalleesByName(funcName string) []string {
	funcs := g.FindNodes(Query{Label: "function", Property: "name", Value: funcName})
	var callees []string
	seen := make(map[string]bool)

	for _, fn := range funcs {
		// Найти call-узлы внутри этой функции
		children := g.GetNeighbors(fn.ID, DirectionOutgoing)
		for _, child := range children {
			if child.HasLabel("call") {
				name, _ := child.GetProp("name")
				n := fmt.Sprintf("%v", name)
				if !seen[n] {
					callees = append(callees, n)
					seen[n] = true
				}
			}
		}
	}
	return callees
}

// FindCallPath ищет путь между двумя функциями по вызовам
func (g *Graph) FindCallPath(fromName, toName string) ([][]string, error) {
	fromNodes := g.FindNodes(Query{Label: "function", Property: "name", Value: fromName})
	toNodes := g.FindNodes(Query{Label: "function", Property: "name", Value: toName})

	if len(fromNodes) == 0 {
		return nil, fmt.Errorf("функция '%s' не найдена", fromName)
	}
	if len(toNodes) == 0 {
		return nil, fmt.Errorf("функция '%s' не найдена", toName)
	}

	var allPaths [][]string
	visited := make(map[int64]bool)

	for _, from := range fromNodes {
		for _, to := range toNodes {
			paths := g.findCallPaths(from.ID, to.ID, []string{fromName}, visited, 10)
			allPaths = append(allPaths, paths...)
		}
	}

	return allPaths, nil
}

func (g *Graph) findCallPaths(currentID, targetID int64, currentPath []string, visited map[int64]bool, maxDepth int) [][]string {
	if len(currentPath) > maxDepth {
		return nil
	}

	if currentID == targetID {
		pathCopy := make([]string, len(currentPath))
		copy(pathCopy, currentPath)
		return [][]string{pathCopy}
	}

	visited[currentID] = true
	defer delete(visited, currentID)

	var allPaths [][]string

	// Ищем вызовы из текущей функции
	children := g.GetNeighbors(currentID, DirectionOutgoing)
	for _, child := range children {
		if child.HasLabel("call") && !visited[child.ID] {
			callName, _ := child.GetProp("name")
			callNameStr := fmt.Sprintf("%v", callName)

			// Найти функцию с таким именем
			targetFuncs := g.FindNodes(Query{Label: "function", Property: "name", Value: callNameStr})
			for _, tf := range targetFuncs {
				paths := g.findCallPaths(tf.ID, targetID, append(currentPath, callNameStr), visited, maxDepth)
				allPaths = append(allPaths, paths...)
			}
		}
	}

	return allPaths
}

func (g *Graph) GetAllEdges() []*Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()

	edges := make([]*Edge, 0, len(g.edgeByID))
	for _, edge := range g.edgeByID {
		edges = append(edges, edge)
	}
	return edges
}

// GetAllReferences возвращает все ссылки
func (g *Graph) GetAllReferences() []*Reference {
	g.mu.RLock()
	defer g.mu.RUnlock()

	refs := make([]*Reference, 0, len(g.refByID))
	for _, ref := range g.refByID {
		refs = append(refs, ref)
	}
	return refs
}
