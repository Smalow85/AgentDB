package graph

import (
    "fmt"
    "testing"

    "agent-db/pkg/storage"
)

func TestGraphAddNode(t *testing.T) {
    disk, _ := storage.NewDiskManager("test_graph.dat")
    defer disk.Close()

    bp := storage.NewBufferPool(100, disk)
    store := NewGraphStore(bp, disk)
    g := NewGraph("test", store)

    node, err := g.AddNode([]string{"class"}, map[string]interface{}{
        "name": "Agent",
        "file": "agent.go",
    })
    if err != nil {
        t.Fatal(err)
    }

    fmt.Printf("Added: %s\n", node)
}

func TestGraphAddEdge(t *testing.T) {
    disk, _ := storage.NewDiskManager("test_graph2.dat")
    defer disk.Close()

    bp := storage.NewBufferPool(100, disk)
    store := NewGraphStore(bp, disk)
    g := NewGraph("test", store)

    node1, _ := g.AddNode([]string{"class"}, map[string]interface{}{"name": "Agent"})
    node2, _ := g.AddNode([]string{"method"}, map[string]interface{}{"name": "Think"})

    edge, err := g.AddEdge("contains", node1.ID, node2.ID)
    if err != nil {
        t.Fatal(err)
    }

    fmt.Printf("Added edge: %s\n", edge)

    // Найти методы класса
    methods := g.GetNeighbors(node1.ID, DirectionOutgoing)
    for _, m := range methods {
        if name, ok := m.GetProp("name"); ok {
            fmt.Printf("  Method: %v\n", name)
        }
    }
}

func TestGraphFindNodes(t *testing.T) {
    disk, _ := storage.NewDiskManager("test_graph3.dat")
    defer disk.Close()

    bp := storage.NewBufferPool(100, disk)
    store := NewGraphStore(bp, disk)
    g := NewGraph("test", store)

    g.AddNode([]string{"class"}, map[string]interface{}{"name": "Agent"})
    g.AddNode([]string{"class"}, map[string]interface{}{"name": "Tool"})
    g.AddNode([]string{"method"}, map[string]interface{}{"name": "Think"})

    classes := g.FindNodes(Query{Label: "class"})
    fmt.Printf("Found %d classes\n", len(classes))

    agent := g.FindNodes(Query{Label: "class", Property: "name", Value: "Agent"})
    if len(agent) > 0 {
        fmt.Printf("Found: %s\n", agent[0])
    }
}

func TestReferences(t *testing.T) {
	disk, _ := storage.NewDiskManager("test_refs.dat")
    defer disk.Close()

    bp := storage.NewBufferPool(100, disk)
    store := NewGraphStore(bp, disk)        // ← убрать graph.
    g := NewGraph("test", store)   

    // Создаём класс с методом
    classNode, _ := g.AddNode([]string{"class"}, map[string]interface{}{"name": "Agent"})
    methodNode, _ := g.AddNode([]string{"function"}, map[string]interface{}{"name": "Think"})
    g.AddEdge("contains", classNode.ID, methodNode.ID)

    // Создаём вызов
    callNode, _ := g.AddNode([]string{"call"}, map[string]interface{}{"name": "Think"})
    
    // Резолвим
    ref, err := g.ResolveCall(callNode.ID, classNode.ID)
    if err != nil {
        t.Fatal(err)
    }

    fmt.Printf("Reference: %d -[%s]-> %d (resolved=%v)\n", ref.SourceID, ref.Type, ref.TargetID, ref.IsResolved)

    // Проверяем GetCallers
    callers := g.GetCallers(methodNode.ID)
    if len(callers) == 0 {
        t.Error("GetCallers должен найти вызов")
    }
    fmt.Printf("Callers of Think: %d\n", len(callers))

    // Проверяем GetCallees
    callees := g.GetCallees(callNode.ID)
    if len(callees) == 0 {
        t.Error("GetCallees должен найти метод")
    }
    fmt.Printf("Callees from call: %d\n", len(callees))
}

func TestGraphFindPath(t *testing.T) {
    disk, _ := storage.NewDiskManager("test_graph_path.dat")
    defer disk.Close()

    bp := storage.NewBufferPool(100, disk)
    store := NewGraphStore(bp, disk)
    g := NewGraph("test", store)

    controller, _ := g.AddNode([]string{"class"}, map[string]interface{}{"name": "Controller"})
    service, _ := g.AddNode([]string{"class"}, map[string]interface{}{"name": "Service"})
    repo, _ := g.AddNode([]string{"class"}, map[string]interface{}{"name": "Repository"})
    database, _ := g.AddNode([]string{"class"}, map[string]interface{}{"name": "Database"})

    g.AddEdge("calls", controller.ID, service.ID)
    g.AddEdge("calls", service.ID, repo.ID)
    g.AddEdge("calls", repo.ID, database.ID)

    paths, err := g.FindPath(controller.ID, database.ID, TraversalConfig{
        MaxDepth:  5,
        EdgeTypes: []string{"calls"},
        Direction: DirectionOutgoing,
    })
    if err != nil {
        t.Fatal(err)
    }

    if len(paths) == 0 {
        t.Error("Путь не найден")
    }

    for _, p := range paths {
        fmt.Printf("Path: %s\n", p)
    }
}

func TestGraphDeleteNode(t *testing.T) {
    disk, _ := storage.NewDiskManager("test_graph_delete.dat")
    defer disk.Close()

    bp := storage.NewBufferPool(100, disk)
    store := NewGraphStore(bp, disk)
    g := NewGraph("test", store)

    node, _ := g.AddNode([]string{"class"}, map[string]interface{}{"name": "Temp"})

    err := g.DeleteNode(node.ID)
    if err != nil {
        t.Fatal(err)
    }

    found := g.GetNode(node.ID)
    if found != nil {
        t.Error("Узел должен быть удалён")
    }
}