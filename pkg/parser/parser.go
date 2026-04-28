package parser

import "fmt"

type AST interface{}

type SelectStmt struct {
	Table  string
	Where  Expr
	Select []string
}

type InsertStmt struct {
	Table  string
	Values map[string]interface{}
}

type CreateTableStmt struct {
	Name    string
	Columns []ColDef
}

type ColDef struct {
	Name string
	Type string
}

type Expr interface{}

func Parse(sql string) (AST, error) {
	if len(sql) < 6 {
		return nil, fmt.Errorf("empty query")
	}
	switch sql[:6] {
	case "SELECT":
		return &SelectStmt{Table: "unknown"}, nil
	case "INSERT":
		return &InsertStmt{Table: "unknown"}, nil
	case "CREATE":
		return &CreateTableStmt{Name: "unknown"}, nil
	default:
		return nil, fmt.Errorf("unsupported query type")
	}
}
