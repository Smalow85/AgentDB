package fixtures

import "agent-db/pkg/storage"

func UsersSchema() *storage.TableSchema {
	return &storage.TableSchema{
		Name: "users",
		Columns: []storage.ColumnDef{
			{Name: "id", ColType: storage.TypeInt},
			{Name: "name", ColType: storage.TypeText},
			{Name: "age", ColType: storage.TypeInt},
			{Name: "email", ColType: storage.TypeText},
		},
	}
}

func ProductsSchema() *storage.TableSchema {
	return &storage.TableSchema{
		Name: "products",
		Columns: []storage.ColumnDef{
			{Name: "id", ColType: storage.TypeInt},
			{Name: "name", ColType: storage.TypeText},
			{Name: "price", ColType: storage.TypeFloat},
			{Name: "in_stock", ColType: storage.TypeBool},
		},
	}
}