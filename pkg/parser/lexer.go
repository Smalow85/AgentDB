package parser

import (
	"fmt"
	"strings"
)

// TokenType — тип токена
type TokenType int

const (
	TokenEOF TokenType = iota
	TokenKeyword
	TokenIdentifier
	TokenNumber
	TokenString
	TokenOperator
	TokenPunctuation
)

// Token — лексема
type Token struct {
	Type  TokenType
	Value string
}

func (t Token) String() string {
	return fmt.Sprintf("[%d]%s", t.Type, t.Value)
}

// Lexer — токенизатор
type Lexer struct {
	input  string
	pos    int
	tokens []Token
}

// Lex разбирает строку на токены
func Lex(input string) ([]Token, error) {
	l := &Lexer{input: input, pos: 0}
	return l.tokenize()
}

var keywords = map[string]bool{
    "SELECT": true, "FROM": true, "WHERE": true,
    "INSERT": true, "INTO": true, "VALUES": true,
    "CREATE": true, "TABLE": true, "DROP": true,
    "INDEX": true, "ON": true,
    "INT": true, "TEXT": true, "FLOAT": true, "BOOL": true,
    "AND": true, "OR": true, "NOT": true, "NULL": true,
    "PRIMARY": true, "KEY": true, "UNIQUE": true,
    "DEFAULT": true, "CHECK": true, "REFERENCES": true,
    "UPDATE": true, "SET": true, "DELETE": true,
    "JOIN": true, "INNER": true, "LEFT": true, "RIGHT": true,
	"ORDER": true, "BY": true, "ASC": true, "DESC": true,
    "LIMIT": true, "OFFSET": true,
}

func (l *Lexer) tokenize() ([]Token, error) {
	var tokens []Token

	for l.pos < len(l.input) {
		ch := l.input[l.pos]

		// Пропускаем пробелы
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			l.pos++
			continue
		}

		// Однострочный комментарий --
		if ch == '-' && l.pos+1 < len(l.input) && l.input[l.pos+1] == '-' {
			for l.pos < len(l.input) && l.input[l.pos] != '\n' {
				l.pos++
			}
			continue
		}

		// Строки в кавычках
		if ch == '\'' || ch == '"' {
			start := l.pos
			l.pos++ // пропускаем открывающую кавычку
			for l.pos < len(l.input) && l.input[l.pos] != ch {
				l.pos++
			}
			if l.pos >= len(l.input) {
				return nil, fmt.Errorf("незакрытая строка")
			}
			tokens = append(tokens, Token{TokenString, l.input[start+1 : l.pos]})
			l.pos++ // закрывающая кавычка
			continue
		}

		// Числа
		if (ch >= '0' && ch <= '9') || (ch == '.' && l.pos+1 < len(l.input) && l.input[l.pos+1] >= '0' && l.input[l.pos+1] <= '9') {
			start := l.pos
			for l.pos < len(l.input) && ((l.input[l.pos] >= '0' && l.input[l.pos] <= '9') || l.input[l.pos] == '.') {
				l.pos++
			}
			tokens = append(tokens, Token{TokenNumber, l.input[start:l.pos]})
			continue
		}

		// Ключевые слова и идентификаторы
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_' || ch == '*' {
			start := l.pos
			for l.pos < len(l.input) &&
				((l.input[l.pos] >= 'a' && l.input[l.pos] <= 'z') ||
					(l.input[l.pos] >= 'A' && l.input[l.pos] <= 'Z') ||
					(l.input[l.pos] >= '0' && l.input[l.pos] <= '9') ||
					l.input[l.pos] == '_' || l.input[l.pos] == '.' || l.input[l.pos] == '*') {  // ← добавить '.'
				l.pos++
			}
			word := l.input[start:l.pos]
			upper := strings.ToUpper(word)
			
			// TRUE, FALSE, NULL — как литералы
			if upper == "TRUE" || upper == "FALSE" || upper == "NULL" {
				tokens = append(tokens, Token{TokenString, word})
				continue
			}
			
			// Ключевые слова (без точек)
			if !strings.Contains(word, ".") && keywords[upper] {
				tokens = append(tokens, Token{TokenKeyword, upper})
			} else if word == "*" {
				tokens = append(tokens, Token{TokenOperator, "*"})
			} else {
				tokens = append(tokens, Token{TokenIdentifier, word})
			}
			continue
		}

		// Операторы
		if ch == '=' || ch == '>' || ch == '<' || ch == '!' {
			op := string(ch)
			if l.pos+1 < len(l.input) && l.input[l.pos+1] == '=' {
				op += "="
				l.pos++
			}
			tokens = append(tokens, Token{TokenOperator, op})
			l.pos++
			continue
		}

		// Пунктуация
		if ch == '(' || ch == ')' || ch == ',' || ch == ';' {
			tokens = append(tokens, Token{TokenPunctuation, string(ch)})
			l.pos++
			continue
		}

		return nil, fmt.Errorf("неожиданный символ: %c (позиция %d)", ch, l.pos)
	}

	return tokens, nil
}