package catalog

type Table struct {
	Name   string
	Schema *Schema
}

type Schema struct {
	Columns []Column
}

type Column struct {
	Name    string
	Type    string
	NotNull bool
}

type Catalog struct {
	Tables map[string]*Table
}

func New() *Catalog {
	return &Catalog{Tables: make(map[string]*Table)}
}

func (c *Catalog) GetTable(name string) (*Table, bool) {
	t, ok := c.Tables[name]
	return t, ok
}

func (c *Catalog) AddTable(t *Table) {
	c.Tables[t.Name] = t
}
