package parser

import "fmt"

// ASTNode — интерфейс для всех узлов дерева
type ASTNode interface {
	String() string
}

// Statement — любой SQL-запрос
type Statement interface {
	ASTNode
	stmt()
}

type JoinType int

const (
	InnerJoin JoinType = iota
	LeftJoin
	RightJoin
)

// SelectStatement — SELECT
type SelectStatement struct {
	Table     string
	Columns   []string
	Condition *BinaryOp
	Join      *JoinClause // новое поле
}

type JoinClause struct {
	Type      JoinType
	Table     string
	Condition *BinaryOp
}

func (s *SelectStatement) stmt() {}
func (s *SelectStatement) String() string {
	if s.Join != nil {
		return fmt.Sprintf("SELECT %v FROM %s JOIN %s ON %s", s.Columns, s.Table, s.Join.Table, s.Join.Condition)
	}
	return fmt.Sprintf("SELECT %v FROM %s", s.Columns, s.Table)
}

// InsertStatement — INSERT
type InsertStatement struct {
	Table  string
	Values []Expression
}

func (s *InsertStatement) stmt() {}
func (s *InsertStatement) String() string {
	return fmt.Sprintf("INSERT INTO %s VALUES %v", s.Table, s.Values)
}

// CreateTableStatement — CREATE TABLE
type CreateTableStatement struct {
	Table   string
	Columns []ColumnDef
}

func (s *CreateTableStatement) stmt() {}
func (s *CreateTableStatement) String() string {
	return fmt.Sprintf("CREATE TABLE %s", s.Table)
}

// ColumnDef в CREATE TABLE
type ColumnDef struct {
	Name       string
	Type       string
	NotNull    bool
	PrimaryKey bool
	Unique     bool
	Default    string   // значение по умолчанию как строка
	Check      string   // условие CHECK как строка
	References string   // "table(column)" для FOREIGN KEY
}

// Expression — значение в INSERT/SET
type Expression interface {
	ASTNode
	expr()
}

// Literal — строковый или числовой литерал
type Literal struct {
	Value string // храним как строку, преобразуем по типу колонки
}

func (l *Literal) expr() {}
func (l *Literal) String() string {
	return l.Value
}

// Identifier — имя колонки или таблицы
type Identifier struct {
	Name string
}

func (i *Identifier) expr() {}
func (i *Identifier) String() string {
	return i.Name
}

// BinaryOp — бинарная операция (col = val)
type BinaryOp struct {
	Left     Expression
	Operator string
	Right    Expression
}

func (b *BinaryOp) expr() {}
func (b *BinaryOp) String() string {
	return fmt.Sprintf("%s %s %s", b.Left, b.Operator, b.Right)
}

type CreateIndexStatement struct {
	IndexName string
	Table     string
	Column    string
}

func (s *CreateIndexStatement) stmt() {}
func (s *CreateIndexStatement) String() string {
	return fmt.Sprintf("CREATE INDEX %s ON %s (%s)", s.IndexName, s.Table, s.Column)
}

// DeleteStatement — DELETE FROM table WHERE condition
type DeleteStatement struct {
	Table     string
	Condition *BinaryOp
}

func (s *DeleteStatement) stmt() {}
func (s *DeleteStatement) String() string {
	if s.Condition != nil {
		return fmt.Sprintf("DELETE FROM %s WHERE %s", s.Table, s.Condition)
	}
	return fmt.Sprintf("DELETE FROM %s", s.Table)
}

// UpdateStatement — UPDATE table SET col = val WHERE condition
type UpdateStatement struct {
	Table     string
	Updates   []UpdatePair
	Condition *BinaryOp
}

// UpdatePair — пара колонка=значение для SET
type UpdatePair struct {
	Column string
	Value  Expression
}

func (s *UpdateStatement) stmt() {}
func (s *UpdateStatement) String() string {
	return fmt.Sprintf("UPDATE %s SET ...", s.Table)
}