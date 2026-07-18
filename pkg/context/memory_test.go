// context/memory_test.go
package context

import (
	"os"
	"testing"

	"agent-db/pkg/executor"
	"agent-db/pkg/storage"
)

func TestMemoryManager_PushInstructionAndReload(t *testing.T) {
	path := "test_mm.db"
	catalogPath := "test_mm.catalog.json"
	defer os.Remove(path)
	defer os.Remove(catalogPath)

	// Сессия 1
	dm1, _ := storage.NewDiskManager(path)
	bp1 := storage.NewBufferPool(10, dm1)
	exec1, _ := executor.NewExecutor(bp1, dm1, catalogPath)
	mm1 := NewMemoryManager(exec1)

	// Push instruction (использует транзакцию)
	id, err := mm1.PushInstruction(1, 1, "test instruction", 0)
	if err != nil {
		t.Fatalf("PushInstruction failed: %v", err)
	}
	t.Logf("Pushed instruction id=%d", id)

	// Флуш и закрытие
	bp1.FlushAll()
	dm1.Close()

	// Сессия 2
	dm2, _ := storage.NewDiskManager(path)
	bp2 := storage.NewBufferPool(10, dm2)
	exec2, _ := executor.NewExecutor(bp2, dm2, catalogPath)
	mm2 := NewMemoryManager(exec2)

	// Проверяем стек
	stack := mm2.GetCurrentStack(1, 1)
	t.Logf("Stack after restart: %d instructions", len(stack))

	if len(stack) != 1 {
		t.Fatalf("Expected 1 instruction after restart, got %d", len(stack))
	}

	if stack[0].Content != "test instruction" {
		t.Errorf("Content mismatch: expected 'test instruction', got '%s'", stack[0].Content)
	}
}
