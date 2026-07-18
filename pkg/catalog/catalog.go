package catalog

import (
	"agent-db/pkg/storage"
	"encoding/json"
	"fmt"
	"os"
)

// Catalog хранит схемы всех таблиц
type Catalog struct {
	filePath string
	Schemas  map[string]*storage.TableSchema
}

// NewCatalog загружает существующий каталог или создаёт новый
func NewCatalog(filePath string) (*Catalog, error) {
	c := &Catalog{
		filePath: filePath,
		Schemas:  make(map[string]*storage.TableSchema),
	}

	// Пробуем загрузить
	data, err := os.ReadFile(filePath)
	if err == nil {
		json.Unmarshal(data, &c.Schemas)
	}

	return c, nil
}

// AddTable добавляет таблицу и сохраняет каталог
func (c *Catalog) AddTable(schema *storage.TableSchema) error {
	if _, exists := c.Schemas[schema.Name]; exists {
		return fmt.Errorf("таблица '%s' уже существует", schema.Name)
	}
	c.Schemas[schema.Name] = schema
	return c.Save()
}

// GetTable возвращает схему таблицы
func (c *Catalog) GetTable(name string) (*storage.TableSchema, bool) {
	schema, ok := c.Schemas[name]
	return schema, ok
}

// ListTables возвращает список таблиц
func (c *Catalog) ListTables() []string {
	names := make([]string, 0, len(c.Schemas))
	for name := range c.Schemas {
		names = append(names, name)
	}
	return names
}

// Save сохраняет каталог в JSON
func (c *Catalog) Save() error {
	data, err := json.MarshalIndent(c.Schemas, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.filePath, data, 0644)
}

func (c *Catalog) RemoveTable(name string) error {
	if _, ok := c.Schemas[name]; !ok {
		return fmt.Errorf("таблица '%s' не найдена", name)
	}
	delete(c.Schemas, name)
	// Сохраняем каталог на диск
	return c.Save()
}
