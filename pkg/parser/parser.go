package parser

import (
	"fmt"
	"strconv"
	"strings"
)

// Parser разбирает токены в AST
type Parser struct {
	tokens []Token
	pos    int
}

// Parse разбирает SQL-строку в AST
func Parse(input string) (Statement, error) {
	tokens, err := Lex(input)
	if err != nil {
		return nil, err
	}
	if len(tokens) == 0 {
		return nil, fmt.Errorf("пустой запрос")
	}

	p := &Parser{tokens: tokens, pos: 0}

	if p.current().Type != TokenKeyword {
		return nil, fmt.Errorf("ожидается SQL-команда, получено %v", p.current())
	}

	switch p.current().Value {
	case "SELECT":
		return p.parseSelect()
	case "INSERT":
		return p.parseInsert()
	case "CREATE":
		return p.parseCreate()
	case "DELETE":
		return p.parseDelete()
	case "UPDATE":
		return p.parseUpdate()
	case "DROP":
		return p.parseDrop()
	default:
		return nil, fmt.Errorf("неизвестная команда: %s", p.current().Value)
	}
}

func (p *Parser) current() Token {
	if p.pos >= len(p.tokens) {
		return Token{TokenEOF, ""}
	}
	return p.tokens[p.pos]
}

func (p *Parser) next() Token {
	tok := p.current()
	p.pos++
	return tok
}

func (p *Parser) expect(tokenType TokenType, value string) error {
	tok := p.current()
	if tok.Type != tokenType || tok.Value != value {
		return fmt.Errorf("ожидается %v, получено %v", value, tok.Value)
	}
	p.next()
	return nil
}

func (p *Parser) parseSelect() (*SelectStatement, error) {
	stmt := &SelectStatement{Limit: -1, Offset: 0}

	p.next() // skip SELECT

	// Колонки
	for p.current().Type != TokenEOF && p.current().Value != "FROM" {
		if p.current().Value == "COUNT" || p.current().Value == "SUM" ||
			p.current().Value == "AVG" || p.current().Value == "MIN" ||
			p.current().Value == "MAX" {
			// Парсим агрегатную функцию
			agg, err := p.parseAggregate()
			if err != nil {
				return nil, err
			}
			stmt.Aggregates = append(stmt.Aggregates, agg)
			stmt.Columns = append(stmt.Columns, agg.Func+"("+agg.Column+")")
		} else if p.current().Type == TokenIdentifier || p.current().Value == "*" {
			stmt.Columns = append(stmt.Columns, p.current().Value)
			p.next()
		} else if p.current().Type == TokenPunctuation && p.current().Value == "," {
			p.next()
		} else {
			p.next()
		}
	}
	if p.current().Value != "FROM" {
		return nil, fmt.Errorf("ожидается FROM")
	}
	p.next()

	if p.current().Type != TokenIdentifier {
		return nil, fmt.Errorf("ожидается имя таблицы")
	}
	stmt.Table = p.current().Value
	p.next()

	// JOIN (опционально)
	if p.current().Value == "JOIN" || p.current().Value == "INNER" || p.current().Value == "LEFT" {
		joinType := InnerJoin
		//LEFT/RIGHT/INNER можут быть или сразу JOIN
		if p.current().Value == "LEFT" || p.current().Value == "RIGHT" || p.current().Value == "INNER" {
			if p.current().Value == "LEFT" {
				joinType = LeftJoin
			} else if p.current().Value == "RIGHT" {
				joinType = RightJoin
			}
			p.next()
		}
		// Теперь должно быть JOIN
		if p.current().Value != "JOIN" {
			return nil, fmt.Errorf("ожидается JOIN")
		}
		p.next()

		if p.current().Type != TokenIdentifier {
			return nil, fmt.Errorf("ожидается имя таблицы после JOIN")
		}
		joinTable := p.current().Value
		p.next()

		if p.current().Value != "ON" {
			return nil, fmt.Errorf("ожидается ON после JOIN")
		}
		p.next()

		joinCond, err := p.parseCondition()
		if err != nil {
			return nil, err
		}
		stmt.Join = &JoinClause{
			Type:      joinType,
			Table:     joinTable,
			Condition: joinCond,
		}
	}

	// WHERE (опционально)
	if p.current().Value == "WHERE" {
		p.next()
		cond, err := p.parseCondition()
		if err != nil {
			return nil, err
		}
		stmt.Condition = cond
	}

	// ORDER BY (опционально)
	if p.current().Value == "ORDER" {
		p.next() // skip ORDER
		if p.current().Value != "BY" {
			return nil, fmt.Errorf("ожидается BY после ORDER")
		}
		p.next() // skip BY

		if p.current().Type != TokenIdentifier {
			return nil, fmt.Errorf("ожидается имя колонки после ORDER BY")
		}
		stmt.OrderBy = p.current().Value
		p.next()

		// ASC/DESC (опционально, по умолчанию ASC)
		if p.current().Value == "ASC" || p.current().Value == "DESC" {
			stmt.OrderDir = p.current().Value
			p.next()
		} else {
			stmt.OrderDir = "ASC"
		}
	}

	// LIMIT (опционально)
	if p.current().Value == "LIMIT" {
		p.next() // skip LIMIT
		if p.current().Type != TokenNumber {
			return nil, fmt.Errorf("ожидается число после LIMIT")
		}
		limit, err := strconv.Atoi(p.current().Value)
		if err != nil {
			return nil, fmt.Errorf("неверное значение LIMIT: %s", p.current().Value)
		}
		stmt.Limit = limit
		p.next()
	}

	// OFFSET (опционально)
	if p.current().Value == "OFFSET" {
		p.next() // skip OFFSET
		if p.current().Type != TokenNumber {
			return nil, fmt.Errorf("ожидается число после OFFSET")
		}
		offset, err := strconv.Atoi(p.current().Value)
		if err != nil {
			return nil, fmt.Errorf("неверное значение OFFSET: %s", p.current().Value)
		}
		stmt.Offset = offset
		p.next()
	}

	return stmt, nil
}

func (p *Parser) parseInsert() (*InsertStatement, error) {
	stmt := &InsertStatement{}
	p.next() // skip INSERT

	if p.current().Value != "INTO" {
		return nil, fmt.Errorf("ожидается INTO")
	}
	p.next()

	if p.current().Type != TokenIdentifier {
		return nil, fmt.Errorf("ожидается имя таблицы")
	}
	stmt.Table = p.current().Value
	p.next()

	fmt.Printf("[DEBUG] parseInsert: table=%s current=%v\n", stmt.Table, p.current())

	// Колонки (опционально)
	if p.current().Value == "(" {
		fmt.Println("[DEBUG] parseInsert: found columns")
		p.next() // skip (
		for p.current().Value != ")" {
			if p.current().Type == TokenPunctuation && p.current().Value == "," {
				p.next()
				continue
			}
			if p.current().Type == TokenIdentifier {
				stmt.Columns = append(stmt.Columns, p.current().Value)
			}
			p.next()
		}
		p.next() // skip )
		fmt.Printf("[DEBUG] parseInsert: columns=%v\n", stmt.Columns)
	}

	if p.current().Value != "VALUES" {
		return nil, fmt.Errorf("ожидается VALUES")
	}
	p.next() // skip VALUES

	if p.current().Value != "(" {
		return nil, fmt.Errorf("ожидается (")
	}
	p.next()

	for p.current().Value != ")" {
		if p.current().Type == TokenPunctuation && p.current().Value == "," {
			p.next()
			continue
		}
		expr, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		stmt.Values = append(stmt.Values, expr)
	}
	p.next() // skip )
	return stmt, nil
}

func (p *Parser) parseCreate() (Statement, error) {
	p.next() // skip CREATE

	if p.current().Value == "TABLE" {
		return p.parseCreateTable()
	}
	if p.current().Value == "INDEX" {
		return p.parseCreateIndex()
	}
	return nil, fmt.Errorf("ожидается TABLE или INDEX после CREATE")
}

func (p *Parser) parseCreateTable() (*CreateTableStatement, error) {
	stmt := &CreateTableStatement{}
	p.next() // skip TABLE

	// IF NOT EXISTS
	if p.current().Value == "IF" {
		p.next() // skip IF
		if p.current().Value != "NOT" {
			return nil, fmt.Errorf("ожидается NOT после IF")
		}
		p.next() // skip NOT
		if p.current().Value != "EXISTS" {
			return nil, fmt.Errorf("ожидается EXISTS после NOT")
		}
		p.next() // skip EXISTS
		stmt.IfNotExists = true
	}

	if p.current().Type != TokenIdentifier {
		return nil, fmt.Errorf("ожидается имя таблицы")
	}
	stmt.Table = p.current().Value
	p.next()

	if p.current().Value != "(" {
		return nil, fmt.Errorf("ожидается (")
	}
	p.next()

	for p.current().Value != ")" {
		if p.current().Type == TokenPunctuation && p.current().Value == "," {
			p.next()
			continue
		}

		if p.current().Type != TokenIdentifier {
			return nil, fmt.Errorf("ожидается имя колонки")
		}
		colName := p.current().Value
		p.next()

		if p.current().Type != TokenKeyword && p.current().Type != TokenIdentifier {
			return nil, fmt.Errorf("ожидается тип колонки")
		}
		colType := p.current().Value
		p.next()

		col := ColumnDef{Name: colName, Type: colType}

		// Constraints
		for p.current().Value != "," && p.current().Value != ")" {
			switch p.current().Value {
			case "PRIMARY_KEY":
				p.next()

				col.PrimaryKey = true
				col.NotNull = true
			case "NOT_NULL":
				p.next()
				col.NotNull = true
			case "UNIQUE":
				p.next()
				col.Unique = true
			case "DEFAULT":
				p.next()
				expr, _ := p.parseExpression()
				if lit, ok := expr.(*Literal); ok {
					col.Default = lit.Value
				}
			case "CHECK":
				p.next()
				if p.current().Value != "(" {
					return nil, fmt.Errorf("ожидается (")
				}
				p.next()
				parts := []string{}
				for p.current().Value != ")" {
					parts = append(parts, p.current().Value)
					p.next()
				}
				p.next()
				col.Check = strings.Join(parts, " ")
			case "REFERENCES":
				p.next()
				refTable := p.current().Value
				p.next()
				if p.current().Value != "(" {
					return nil, fmt.Errorf("ожидается (")
				}
				p.next()
				refCol := p.current().Value
				p.next()
				if p.current().Value != ")" {
					return nil, fmt.Errorf("ожидается )")
				}
				p.next()
				col.References = refTable + "(" + refCol + ")"
			case "AUTOINCREMENT":
				col.AutoIncrement = true
				p.next()
			default:
				goto done
			}
		}
	done:
		stmt.Columns = append(stmt.Columns, col)
	}
	p.next() // skip )
	return stmt, nil
}

// parseCreateIndex: CREATE INDEX name ON table (column)
func (p *Parser) parseCreateIndex() (*CreateIndexStatement, error) {
	stmt := &CreateIndexStatement{}

	p.next() // skip INDEX

	if p.current().Type != TokenIdentifier {
		return nil, fmt.Errorf("ожидается имя индекса")
	}
	stmt.IndexName = p.current().Value
	p.next()

	if p.current().Value != "ON" {
		return nil, fmt.Errorf("ожидается ON")
	}
	p.next()

	if p.current().Type != TokenIdentifier {
		return nil, fmt.Errorf("ожидается имя таблицы")
	}
	stmt.Table = p.current().Value
	p.next()

	if p.current().Value != "(" {
		return nil, fmt.Errorf("ожидается (")
	}
	p.next()

	if p.current().Type != TokenIdentifier {
		return nil, fmt.Errorf("ожидается имя колонки")
	}
	stmt.Column = p.current().Value
	p.next()

	if p.current().Value != ")" {
		return nil, fmt.Errorf("ожидается )")
	}
	p.next()

	return stmt, nil
}

func (p *Parser) parseCondition() (*BinaryOp, error) {
	left, err := p.parseExpression()
	if err != nil {
		return nil, err
	}

	if p.current().Type != TokenOperator && p.current().Value != "LIKE" {
		return nil, fmt.Errorf("ожидается оператор, получено %v", p.current())
	}
	op := p.current().Value
	p.next()

	right, err := p.parseExpression()
	if err != nil {
		return nil, err
	}

	return &BinaryOp{Left: left, Operator: op, Right: right}, nil
}

func (p *Parser) parseExpression() (Expression, error) {
	tok := p.current()

	switch tok.Type {
	case TokenNumber:
		p.next()
		return &Literal{Value: tok.Value}, nil
	case TokenString:
		p.next()
		return &Literal{Value: tok.Value}, nil
	case TokenIdentifier:
		p.next()
		return &Identifier{Name: tok.Value}, nil
	}

	return nil, fmt.Errorf("неожиданное выражение: %v", tok)
}

func (p *Parser) parseDelete() (*DeleteStatement, error) {
	stmt := &DeleteStatement{}

	p.next() // skip DELETE

	if p.current().Value != "FROM" {
		return nil, fmt.Errorf("ожидается FROM")
	}
	p.next() // skip FROM

	if p.current().Type != TokenIdentifier {
		return nil, fmt.Errorf("ожидается имя таблицы")
	}
	stmt.Table = p.current().Value
	p.next()

	// WHERE (опционально)
	if p.current().Value == "WHERE" {
		p.next() // skip WHERE
		cond, err := p.parseCondition()
		if err != nil {
			return nil, err
		}
		stmt.Condition = cond
	}

	return stmt, nil
}

// parseUpdate: UPDATE table SET col = val [, ...] [WHERE condition]
func (p *Parser) parseUpdate() (*UpdateStatement, error) {
	stmt := &UpdateStatement{}

	p.next() // skip UPDATE

	if p.current().Type != TokenIdentifier {
		return nil, fmt.Errorf("ожидается имя таблицы")
	}
	stmt.Table = p.current().Value
	p.next()

	if p.current().Value != "SET" {
		return nil, fmt.Errorf("ожидается SET")
	}
	p.next() // skip SET

	// Пары col = val
	for {
		if p.current().Type != TokenIdentifier {
			return nil, fmt.Errorf("ожидается имя колонки")
		}
		colName := p.current().Value
		p.next()

		if p.current().Value != "=" {
			return nil, fmt.Errorf("ожидается =")
		}
		p.next()

		val, err := p.parseExpression()
		if err != nil {
			return nil, err
		}

		stmt.Updates = append(stmt.Updates, UpdatePair{Column: colName, Value: val})

		if p.current().Value == "," {
			p.next() // skip запятая
			continue
		}
		break
	}

	// WHERE (опционально)
	if p.current().Value == "WHERE" {
		p.next()
		cond, err := p.parseCondition()
		if err != nil {
			return nil, err
		}
		stmt.Condition = cond
	}

	return stmt, nil
}

func (p *Parser) parseDrop() (*DropTableStatement, error) {
	stmt := &DropTableStatement{}
	p.next() // skip DROP

	if p.current().Value != "TABLE" {
		return nil, fmt.Errorf("ожидается TABLE после DROP")
	}
	p.next()

	// IF EXISTS
	if p.current().Value == "IF" {
		p.next()
		if p.current().Value != "EXISTS" {
			return nil, fmt.Errorf("ожидается EXISTS после IF")
		}
		p.next()
		stmt.IfExists = true
	}

	if p.current().Type != TokenIdentifier {
		return nil, fmt.Errorf("ожидается имя таблицы")
	}
	stmt.Table = p.current().Value
	p.next()

	return stmt, nil
}

func (p *Parser) parseAggregate() (Aggregate, error) {
	agg := Aggregate{}
	agg.Func = p.current().Value
	p.next() // skip COUNT/SUM/etc

	if p.current().Value != "(" {
		return agg, fmt.Errorf("ожидается ( после %s", agg.Func)
	}
	p.next() // skip (

	if p.current().Value == "*" {
		agg.Column = "*"
	} else if p.current().Type == TokenIdentifier {
		agg.Column = p.current().Value
	} else {
		return agg, fmt.Errorf("ожидается колонка или *")
	}
	p.next()

	if p.current().Value != ")" {
		return agg, fmt.Errorf("ожидается )")
	}
	p.next()

	return agg, nil
}
