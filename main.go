package main

import (
	"fmt"
	"os"

	"agent-db/pkg/executor"
	"agent-db/pkg/server"
	"agent-db/pkg/storage"
)

func main() {
	fmt.Println("╔══════════════════════════════════╗")
	fmt.Println("║        AgentDB v0.6              ║")
	fmt.Println("╚══════════════════════════════════╝")
	fmt.Println()

	disk, err := storage.NewDiskManager("agentdb.dat")
	if err != nil {
		fmt.Printf("Ошибка диска: %v\n", err)
		os.Exit(1)
	}
	defer disk.Close()

	bp := storage.NewBufferPool(100, disk)
	defer bp.FlushAll()

	exec, err := executor.NewExecutor(bp, disk, "agentdb.catalog.json")
	if err != nil {
		fmt.Printf("Ошибка executor: %v\n", err)
		os.Exit(1)
	}

	tables := exec.ListTables()
	if len(tables) > 0 {
		fmt.Printf("✓ Загружено %d таблиц: %v\n", len(tables), tables)
	}

	srv := server.NewServer(exec)
	if err := srv.Start(":8080"); err != nil {
		fmt.Printf("Ошибка сервера: %v\n", err)
		os.Exit(1)
	}
}
