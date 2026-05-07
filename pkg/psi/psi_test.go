package psi

import (
    "fmt"
    "os"
    "testing"

    "agent-db/pkg/graph"
    "agent-db/pkg/storage"
)

func TestParseGoWithTreeSitter(t *testing.T) {
    disk, _ := storage.NewDiskManager("test_psi_go.dat")
    defer disk.Close()

    bp := storage.NewBufferPool(100, disk)
    store := graph.NewGraphStore(bp, disk)
    g := graph.NewGraph("test", store)

    parser := NewPSIParser(g)

    testCode := `package main

type Agent struct {
    Name string
}

func (a *Agent) Think() {
    a.Analyze()
    result := a.Process()
    fmt.Println(result)
}

func (a *Agent) Analyze() string {
    return "thinking..."
}

func (a *Agent) Process() string {
    return "done"
}
`
    os.WriteFile("test_tree.go", []byte(testCode), 0644)
    defer os.Remove("test_tree.go")

    lang := GetLanguage("go")
    err := parser.ParseFile("test_tree.go", lang)
    if err != nil {
        t.Fatal(err)
    }

    classes := g.FindNodes(graph.Query{Label: "class", Property: "name", Value: "Agent"})
    if len(classes) == 0 {
        t.Error("Класс Agent не найден")
    } else {
        fmt.Printf("Класс: %v\n", classes[0].Properties)
    }

    funcs := g.FindNodes(graph.Query{Label: "function"})
    fmt.Printf("Функций: %d\n", len(funcs))
    for _, f := range funcs {
        name, _ := f.GetProp("name")
        fmt.Printf("  - %v\n", name)
        callers := g.GetCallers(f.ID)
        if len(callers) > 0 {
            for _, c := range callers {
                cn, _ := c.GetProp("name")
                fmt.Printf("    ← вызывается из %v\n", cn)
            }
        }
    }

    calls := g.FindNodes(graph.Query{Label: "call"})
    fmt.Printf("Вызовов: %d\n", len(calls))
}