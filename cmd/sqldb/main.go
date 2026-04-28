package main

import (
	"log"

	"sql-db/pkg/catalog"
	"sql-db/pkg/executor"
	"sql-db/pkg/network"
	"sql-db/pkg/storage"
)

func main() {
	cat := catalog.New()
	store := storage.NewHeap()
	exec := executor.New(cat, store)

	if err := network.Listen(":5432", exec); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
