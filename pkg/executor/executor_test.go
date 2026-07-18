// executor/executor_test.go
package executor

import (
	"os"
	"testing"

	"agent-db/pkg/storage"
)

func TestExecutor_InsertAndSelectAfterRestart(t *testing.T) {
	path := "test_exec.db"
	catalogPath := "test_exec.catalog.json"
	defer os.Remove(path)
	defer os.Remove(catalogPath)

	// Сессия 1
	dm1, _ := storage.NewDiskManager(path)
	bp1 := storage.NewBufferPool(10, dm1)
	exec1, _ := NewExecutor(bp1, dm1, catalogPath)

	// CREATE TABLE
	res, _ := exec1.Execute("CREATE TABLE users (id INT PRIMARY_KEY, name TEXT)")
	t.Logf("CREATE result: %s", res.Type)

	// INSERT
	res, _ = exec1.Execute("INSERT INTO users VALUES (1, 'Alice')")
	t.Logf("INSERT result: %s", res.Type)

	// SELECT (должно работать)
	res, _ = exec1.Execute("SELECT * FROM users")
	t.Logf("SELECT before restart: %d rows", len(res.Rows))

	// Флуш и закрытие
	bp1.FlushAll()
	dm1.Close()

	// Сессия 2: перезагрузка
	dm2, _ := storage.NewDiskManager(path)
	bp2 := storage.NewBufferPool(10, dm2)
	exec2, _ := NewExecutor(bp2, dm2, catalogPath)

	// Проверяем таблицы
	tables := exec2.ListTables()
	t.Logf("Tables after restart: %v", tables)

	// SELECT
	res, _ = exec2.Execute("SELECT * FROM users")
	t.Logf("SELECT after restart: %d rows", len(res.Rows))

	if len(res.Rows) != 1 {
		t.Fatalf("Expected 1 row after restart, got %d", len(res.Rows))
	}

	name := res.Rows[0][1]
	if name != "Alice" {
		t.Errorf("Name mismatch: expected Alice, got %v", name)
	}
}
