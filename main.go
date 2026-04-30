package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"agent-db/pkg/executor"
	"agent-db/pkg/storage"
)

func main() {
	fmt.Println("╔══════════════════════════════════╗")
	fmt.Println("║        AgentDB v0.2              ║")
	fmt.Println("║  SQL-подобная база данных       ║")
	fmt.Println("╠══════════════════════════════════╣")
	fmt.Println("║  CREATE TABLE | INSERT | SELECT ║")
	fmt.Println("║  quit/exit — выход              ║")
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

	// Показываем загруженные таблицы
	tables := exec.ListTables()
	if len(tables) > 0 {
		fmt.Println("Загруженные таблицы:")
		for _, t := range tables {
			fmt.Printf("  • %s\n", t)
		}
		fmt.Println()
	}

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("agent-db> ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if input == "" {
			continue
		}
		if strings.ToLower(input) == "quit" || strings.ToLower(input) == "exit" {
			fmt.Println("До свидания!")
			break
		}

		result, err := exec.Execute(input)
		if err != nil {
			fmt.Printf("× Ошибка: %v\n", err)
		} else {
			fmt.Println(result)
		}
	}
}