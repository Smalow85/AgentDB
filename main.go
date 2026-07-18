package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

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

	// ✅ Graceful shutdown по сигналам
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := srv.Start(":8080"); err != nil {
			fmt.Printf("Ошибка сервера: %v\n", err)
			stop()
		}
	}()

	<-ctx.Done()
	fmt.Println("\n⏳ Завершение работа...")

	// ✅ Финальный сброс
	if err := bp.FlushAll(); err != nil {
		fmt.Printf("Ошибка сброса буфера: %v\n", err)
	}

	fmt.Println("✓ Данные сохранены")
}
