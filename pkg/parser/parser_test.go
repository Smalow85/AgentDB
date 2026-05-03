package parser

import (
	"fmt"
	"testing"
)

func TestLexSimple(t *testing.T) {
	tokens, err := Lex("SELECT * FROM users WHERE id = 5")
	if err != nil {
		t.Fatal(err)
	}

	for _, tok := range tokens {
		fmt.Printf("%v\n", tok)
	}

	if len(tokens) < 5 {
		t.Errorf("Слишком мало токенов: %d", len(tokens))
	}
}

func TestParseSelect(t *testing.T) {
	stmt, err := Parse("SELECT * FROM users")
	if err != nil {
		t.Fatal(err)
	}

	selectStmt, ok := stmt.(*SelectStatement)
	if !ok {
		t.Fatal("Ожидается SelectStatement")
	}

	if selectStmt.Table != "users" {
		t.Errorf("Таблица: ожидается 'users', получено '%s'", selectStmt.Table)
	}
	fmt.Printf("SELECT: %s\n", stmt)
}

func TestParseSelectWhere(t *testing.T) {
	stmt, err := Parse("SELECT name, age FROM users WHERE id = 42")
	if err != nil {
		t.Fatal(err)
	}

	selectStmt := stmt.(*SelectStatement)
	if selectStmt.Condition == nil {
		t.Fatal("Ожидается WHERE условие")
	}

	fmt.Printf("SELECT с WHERE: %s\n", stmt)
	fmt.Printf("  Условие: %s\n", selectStmt.Condition)
}

func TestParseInsert(t *testing.T) {
	stmt, err := Parse("INSERT INTO users VALUES (1, 'Alice', 30)")
	if err != nil {
		t.Fatal(err)
	}

	insertStmt := stmt.(*InsertStatement)
	if insertStmt.Table != "users" {
		t.Errorf("Таблица: 'users' != '%s'", insertStmt.Table)
	}
	if len(insertStmt.Values) != 3 {
		t.Errorf("Значений: 3 != %d", len(insertStmt.Values))
	}

	fmt.Printf("INSERT: %s\n", stmt)
	for i, v := range insertStmt.Values {
		fmt.Printf("  val[%d] = %s\n", i, v)
	}
}

func TestParseCreateTable(t *testing.T) {
	stmt, err := Parse("CREATE TABLE users (id INT, name TEXT, age INT)")
	if err != nil {
		t.Fatal(err)
	}

	createStmt, ok := stmt.(*CreateTableStatement)
	if !ok {
		t.Fatalf("Ожидается CreateTableStatement, получено %T", stmt)
	}
	if createStmt.Table != "users" {
		t.Errorf("Таблица: 'users' != '%s'", createStmt.Table)
	}
	if len(createStmt.Columns) != 3 {
		t.Errorf("Колонок: 3 != %d", len(createStmt.Columns))
	}

	fmt.Printf("CREATE TABLE: %s\n", stmt)
	for _, col := range createStmt.Columns {
		fmt.Printf("  %s %s\n", col.Name, col.Type)
	}
}

func TestParseCreateIndex(t *testing.T) {
	stmt, err := Parse("CREATE INDEX idx_id ON users (id)")
	if err != nil {
		t.Fatal(err)
	}

	idxStmt, ok := stmt.(*CreateIndexStatement)
	if !ok {
		t.Fatalf("Ожидается CreateIndexStatement, получено %T", stmt)
	}
	if idxStmt.IndexName != "idx_id" {
		t.Errorf("IndexName: 'idx_id' != '%s'", idxStmt.IndexName)
	}
	if idxStmt.Table != "users" {
		t.Errorf("Table: 'users' != '%s'", idxStmt.Table)
	}
	if idxStmt.Column != "id" {
		t.Errorf("Column: 'id' != '%s'", idxStmt.Column)
	}

	fmt.Printf("CREATE INDEX: %s\n", stmt)
}

func TestParseInvalid(t *testing.T) {
	_, err := Parse("FOOBAR baz")
	if err == nil {
		t.Error("Ожидалась ошибка для неизвестной команды")
	}
}

func TestParseDelete(t *testing.T) {
	stmt, err := Parse("DELETE FROM users WHERE id = 5")
	if err != nil {
		t.Fatal(err)
	}

	delStmt, ok := stmt.(*DeleteStatement)
	if !ok {
		t.Fatalf("Ожидается DeleteStatement, получено %T", stmt)
	}
	if delStmt.Table != "users" {
		t.Errorf("Table: 'users' != '%s'", delStmt.Table)
	}
	if delStmt.Condition == nil {
		t.Error("Ожидается WHERE условие")
	}

	fmt.Printf("DELETE: %s\n", stmt)
}

func TestParseDeleteAll(t *testing.T) {
	stmt, err := Parse("DELETE FROM users")
	if err != nil {
		t.Fatal(err)
	}

	delStmt := stmt.(*DeleteStatement)
	if delStmt.Condition != nil {
		t.Error("Условия не должно быть")
	}

	fmt.Printf("DELETE ALL: %s\n", stmt)
}

func TestParseUpdate(t *testing.T) {
	stmt, err := Parse("UPDATE users SET name = 'Bob' WHERE id = 1")
	if err != nil {
		t.Fatal(err)
	}

	updStmt, ok := stmt.(*UpdateStatement)
	if !ok {
		t.Fatalf("Ожидается UpdateStatement, получено %T", stmt)
	}
	if updStmt.Table != "users" {
		t.Errorf("Table: 'users' != '%s'", updStmt.Table)
	}
	if len(updStmt.Updates) != 1 {
		t.Errorf("Updates: 1 != %d", len(updStmt.Updates))
	}
	if updStmt.Condition == nil {
		t.Error("Ожидается WHERE условие")
	}

	fmt.Printf("UPDATE: %s\n", stmt)
}

func TestParseUpdateMultiple(t *testing.T) {
	stmt, err := Parse("UPDATE users SET name = 'Alice', age = 31 WHERE id = 1")
	if err != nil {
		t.Fatal(err)
	}

	updStmt := stmt.(*UpdateStatement)
	if len(updStmt.Updates) != 2 {
		t.Errorf("Ожидается 2 обновления, получено %d", len(updStmt.Updates))
	}

	fmt.Printf("UPDATE: %s\n", stmt)
	for _, upd := range updStmt.Updates {
		fmt.Printf("  %s = %s\n", upd.Column, upd.Value)
	}
}

func TestParseJoin(t *testing.T) {
	stmt, err := Parse("SELECT * FROM users JOIN orders ON users.id = orders.user_id")
	if err != nil {
		t.Fatal(err)
	}

	sel, ok := stmt.(*SelectStatement)
	if !ok {
		t.Fatal("Ожидается SelectStatement")
	}
	if sel.Join == nil {
		t.Fatal("Ожидается JOIN")
	}
	if sel.Join.Table != "orders" {
		t.Errorf("JOIN table: 'orders' != '%s'", sel.Join.Table)
	}
	if sel.Join.Type != InnerJoin {
		t.Error("Ожидается INNER JOIN")
	}

	fmt.Printf("JOIN: %s\n", stmt)
}

func TestParseLeftJoin(t *testing.T) {
	stmt, err := Parse("SELECT * FROM users LEFT JOIN orders ON users.id = orders.user_id")
	if err != nil {
		t.Fatal(err)
	}

	sel := stmt.(*SelectStatement)
	if sel.Join.Type != LeftJoin {
		t.Error("Ожидается LEFT JOIN")
	}

	fmt.Printf("LEFT JOIN: %s\n", stmt)
}

func TestParseJoinWithWhere(t *testing.T) {
	stmt, err := Parse("SELECT * FROM users JOIN orders ON users.id = orders.user_id WHERE orders.amount > 100")
	if err != nil {
		t.Fatal(err)
	}

	sel := stmt.(*SelectStatement)
	if sel.Join == nil {
		t.Fatal("Ожидается JOIN")
	}
	if sel.Condition == nil {
		t.Fatal("Ожидается WHERE")
	}

	fmt.Printf("JOIN + WHERE: %s\n", stmt)
}

func TestParseOrderBy(t *testing.T) {
    stmt, err := Parse("SELECT * FROM users ORDER BY age DESC")
    if err != nil {
        t.Fatal(err)
    }

    sel := stmt.(*SelectStatement)
    if sel.OrderBy != "age" {
        t.Errorf("OrderBy: 'age' != '%s'", sel.OrderBy)
    }
    if sel.OrderDir != "DESC" {
        t.Errorf("OrderDir: 'DESC' != '%s'", sel.OrderDir)
    }
    fmt.Printf("ORDER BY: %s\n", stmt)
}

func TestParseLimit(t *testing.T) {
    stmt, err := Parse("SELECT * FROM users LIMIT 10")
    if err != nil {
        t.Fatal(err)
    }

    sel := stmt.(*SelectStatement)
    if sel.Limit != 10 {
        t.Errorf("Limit: 10 != %d", sel.Limit)
    }
    fmt.Printf("LIMIT: %s\n", stmt)
}

func TestParseOrderByLimitOffset(t *testing.T) {
    stmt, err := Parse("SELECT * FROM users ORDER BY id ASC LIMIT 5 OFFSET 10")
    if err != nil {
        t.Fatal(err)
    }

    sel := stmt.(*SelectStatement)
    if sel.OrderBy != "id" || sel.OrderDir != "ASC" {
        t.Error("OrderBy неверно")
    }
    if sel.Limit != 5 {
        t.Errorf("Limit: 5 != %d", sel.Limit)
    }
    if sel.Offset != 10 {
        t.Errorf("Offset: 10 != %d", sel.Offset)
    }
    fmt.Printf("Full: %s\n", stmt)
}