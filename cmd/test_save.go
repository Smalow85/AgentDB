package main

import (
	"fmt"
	"agent-db/pkg/graph"
	"agent-db/pkg/storage"
)

func main() {
	disk, err := storage.NewDiskManager("psi_graph.dat")
	if err != nil {
		fmt.Printf("Disk error: %v\n", err)
		return
	}
	defer disk.Close()

	bp := storage.NewBufferPool(100, disk)
	store := graph.NewGraphStore(bp, disk)
	g := graph.NewGraph("psigraph", store)

	// Create some test nodes
	node1, err := g.AddNode([]string{"file"}, map[string]interface{}{"name": "test.go"})
	if err != nil {
		fmt.Printf("AddNode error: %v\n", err)
		return
	}
	fmt.Printf("Created node 1: ID=%d\n", node1.ID)
	
	node2, err := g.AddNode([]string{"function"}, map[string]interface{}{"name": "main"})
	if err != nil {
		fmt.Printf("AddNode error: %v\n", err)
		return
	}
	fmt.Printf("Created node 2: ID=%d\n", node2.ID)
	
	node3, err := g.AddNode([]string{"class"}, map[string]interface{}{"name": "MyClass"})
	if err != nil {
		fmt.Printf("AddNode error: %v\n", err)
		return
	}
	fmt.Printf("Created node 3: ID=%d\n", node3.ID)

	fmt.Printf("Total nodes in memory: %d\n", len(g.FindNodes(graph.Query{})))

	// Save to disk
	err = g.SaveToDisk()
	if err != nil {
		fmt.Printf("SaveToDisk error: %v\n", err)
		return
	}
	fmt.Println("Saved to disk")

	// Reload
	g2 := graph.NewGraph("psigraph", store)
	err = g2.Load()
	if err != nil {
		fmt.Printf("Load error: %v\n", err)
		return
	}

	nodes := g2.FindNodes(graph.Query{})
	fmt.Printf("Loaded %d nodes\n", len(nodes))
}