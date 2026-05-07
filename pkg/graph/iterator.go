package graph

import (
    "container/list"
    "fmt"
)

// TraversalConfig — настройки обхода
type TraversalConfig struct {
    MaxDepth   int      // максимальная глубина (-1 = без ограничений)
    EdgeTypes  []string // типы связей для обхода (пусто = все)
    Direction  Direction
}

// Path — путь в графе
type Path struct {
    Nodes []*Node
    Edges []*Edge
}

func (p *Path) String() string {
    if len(p.Nodes) == 0 {
        return "empty path"
    }
    result := ""
    for i, node := range p.Nodes {
        if name, ok := node.GetProp("name"); ok {
            result += fmt.Sprintf("%v", name)
        } else {
            result += fmt.Sprintf("node[%d]", node.ID)
        }
        if i < len(p.Edges) {
            result += fmt.Sprintf(" -[%s]-> ", p.Edges[i].Type)
        }
    }
    return result
}

// FindPath ищет кратчайший путь между узлами (BFS)
func (g *Graph) FindPath(fromID, toID int64, config TraversalConfig) ([]Path, error) {
    g.mu.RLock()
    defer g.mu.RUnlock()

    if g.nodeByID[fromID] == nil {
        return nil, fmt.Errorf("узел %d не найден", fromID)
    }
    if g.nodeByID[toID] == nil {
        return nil, fmt.Errorf("узел %d не найден", toID)
    }

    if fromID == toID {
        return []Path{{Nodes: []*Node{g.nodeByID[fromID]}}}, nil
    }

    // BFS очередь: текущий узел + путь до него
    type queueItem struct {
        nodeID int64
        path   Path
    }

    queue := list.New()
    queue.PushBack(&queueItem{
        nodeID: fromID,
        path:   Path{Nodes: []*Node{g.nodeByID[fromID]}},
    })

    visited := make(map[int64]bool)
    visited[fromID] = true

    var paths []Path

    for queue.Len() > 0 {
        item := queue.Remove(queue.Front()).(*queueItem)

        // Проверяем глубину
        if config.MaxDepth > 0 && len(item.path.Edges) >= config.MaxDepth {
            continue
        }

        // Получаем соседей
        edges := g.GetEdges(item.nodeID, config.Direction)

        for _, edge := range edges {
            // Фильтр по типам связей
            if len(config.EdgeTypes) > 0 {
                found := false
                for _, et := range config.EdgeTypes {
                    if edge.Type == et {
                        found = true
                        break
                    }
                }
                if !found {
                    continue
                }
            }

            // Определяем следующий узел
            nextID := edge.ToID
            if item.nodeID == edge.ToID {
                nextID = edge.FromID
            }

            if visited[nextID] {
                continue
            }

            nextNode := g.nodeByID[nextID]
            if nextNode == nil {
                continue
            }

            // Строим новый путь
            newPath := Path{
                Nodes: make([]*Node, len(item.path.Nodes)+1),
                Edges: make([]*Edge, len(item.path.Edges)+1),
            }
            copy(newPath.Nodes, item.path.Nodes)
            copy(newPath.Edges, item.path.Edges)
            newPath.Nodes[len(item.path.Nodes)] = nextNode
            newPath.Edges[len(item.path.Edges)] = edge

            if nextID == toID {
                paths = append(paths, newPath)
                // Не останавливаемся — ищем все пути до MaxDepth
                continue
            }

            visited[nextID] = true
            queue.PushBack(&queueItem{
                nodeID: nextID,
                path:   newPath,
            })
        }
    }

    return paths, nil
}

// FindAllPaths ищет все пути (включая не кратчайшие)
func (g *Graph) FindAllPaths(fromID, toID int64, config TraversalConfig) ([]Path, error) {
    // BFS который не помечает visited (для поиска всех путей)
    g.mu.RLock()
    defer g.mu.RUnlock()

    var allPaths []Path
    g.dfs(fromID, toID, config, Path{}, make(map[int64]bool), &allPaths)
    return allPaths, nil
}

func (g *Graph) dfs(currentID, targetID int64, config TraversalConfig, currentPath Path, visited map[int64]bool, paths *[]Path) {
    if config.MaxDepth > 0 && len(currentPath.Edges) >= config.MaxDepth {
        return
    }

    currentNode := g.nodeByID[currentID]
    if currentNode == nil {
        return
    }

    currentPath.Nodes = append(currentPath.Nodes, currentNode)

    if currentID == targetID && len(currentPath.Nodes) > 1 {
        pathCopy := Path{
            Nodes: make([]*Node, len(currentPath.Nodes)),
            Edges: make([]*Edge, len(currentPath.Edges)),
        }
        copy(pathCopy.Nodes, currentPath.Nodes)
        copy(pathCopy.Edges, currentPath.Edges)
        *paths = append(*paths, pathCopy)
        return
    }

    visited[currentID] = true
    defer delete(visited, currentID)

    edges := g.GetEdges(currentID, config.Direction)
    for _, edge := range edges {
        nextID := edge.ToID
        if currentID == edge.ToID {
            nextID = edge.FromID
        }

        if visited[nextID] {
            continue
        }

        if len(config.EdgeTypes) > 0 {
            found := false
            for _, et := range config.EdgeTypes {
                if edge.Type == et {
                    found = true
                    break
                }
            }
            if !found {
                continue
            }
        }

        currentPath.Edges = append(currentPath.Edges, edge)
        g.dfs(nextID, targetID, config, currentPath, visited, paths)
        currentPath.Edges = currentPath.Edges[:len(currentPath.Edges)-1]
    }
}

// GetConnectedNodes возвращает все связанные узлы (и edges, и refs)
func (g *Graph) GetConnectedNodes(nodeID int64, edgeTypes []string) []*Node {
    var nodes []*Node
    seen := make(map[int64]bool)

    // Через рёбра
    edges := g.GetEdges(nodeID, DirectionOutgoing)
    for _, edge := range edges {
        if len(edgeTypes) > 0 {
            found := false
            for _, et := range edgeTypes {
                if edge.Type == et {
                    found = true
                    break
                }
            }
            if !found {
                continue
            }
        }
        if node := g.GetNode(edge.ToID); node != nil && !seen[node.ID] {
            nodes = append(nodes, node)
            seen[node.ID] = true
        }
    }

    // Через References (для "call" типа)
    refs := g.GetReferences(nodeID, DirectionOutgoing)
    for _, ref := range refs {
        if ref.IsResolved && !seen[ref.TargetID] {
            if node := g.GetNode(ref.TargetID); node != nil {
                nodes = append(nodes, node)
                seen[ref.TargetID] = true
            }
        }
    }

    return nodes
}