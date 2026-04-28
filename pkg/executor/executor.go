package executor

import (
	"sql-db/pkg/catalog"
	"sql-db/pkg/parser"
	"sql-db/pkg/storage"
)

type Executor struct {
	cat   *catalog.Catalog
	store storage.StorageEngine
}

func New(cat *catalog.Catalog, store storage.StorageEngine) *Executor {
	return &Executor{cat: cat, store: store}
}

func (e *Executor) Execute(sql string) (interface{}, error) {
	ast, err := parser.Parse(sql)
	if err != nil {
		return nil, err
	}

	switch stmt := ast.(type) {
	case *parser.SelectStmt:
		return e.store.Scan(stmt.Table)
	case *parser.InsertStmt:
		return nil, e.store.Insert(stmt.Table, stmt.Values)
	case *parser.CreateTableStmt:
		e.cat.AddTable(&catalog.Table{
			Name:   stmt.Name,
			Schema: &catalog.Schema{},
		})
		return "created", nil
	default:
		return nil, nil
	}
}
