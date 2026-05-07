package main

import (
	"fmt"
	"agent-db/pkg/graph"
	"agent-db/pkg/storage"
)

func main() {
	disk, _ := storage.NewDiskManager("psi_graph.dat")
	bp := storage.NewBufferPool(100, disk)
	store := graph.NewGraphStore(bp, disk)
	g := graph.NewGraph("psigraph", store)

	err := g.Load()
	if err != nil {
		fmt.Printf("Load error: %v\n", err)
		return
	}

	nodes := g.FindNodes(graph.Query{})
	fmt.Printf("Loaded %d nodes\n", len(nodes))
	
	classes := g.FindNodes(graph.Query{Label: "class"})
	functions := g.FindNodes(graph.Query{Label: "function"})
	files := g.FindNodes(graph.Query{Label: "file"})
	
	fmt.Printf("classes: %d, functions: %d, files: %d\n", len(classes), len(functions), len(files))
}