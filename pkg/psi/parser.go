package psi

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/odvcencio/gotreesitter"

	"agent-db/pkg/graph"
)

type PSIParser struct {
	Graph           *graph.Graph
	fileID          int64
	currentFilePath string // ← ДОБАВИТЬ: текущий файл
	lang            *gotreesitter.Language
}

func NewPSIParser(g *graph.Graph) *PSIParser {
	return &PSIParser{Graph: g}
}

func (p *PSIParser) ParseRepo(repoPath string) error {
	err := filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		lang := DetectLanguage(path)
		if lang == nil {
			return nil
		}
		return p.ParseFile(path, lang)
	})
	if err != nil {
		return err
	}
	p.resolveAllCalls()
	return nil
}

func (p *PSIParser) ParseFile(filePath string, lang *LanguageInfo) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("ошибка чтения %s: %w", filePath, err)
	}

	// Нормализуем путь
	normalizedPath := filepath.ToSlash(filePath)
	p.currentFilePath = normalizedPath

	// Создаём узел файла с path
	fileNode, _ := p.Graph.AddNode([]string{"file", lang.Name}, map[string]interface{}{
		"path": normalizedPath,
		"name": filepath.Base(normalizedPath),
	})
	p.fileID = fileNode.ID

	p.lang = lang.GetLang()
	parser := gotreesitter.NewParser(p.lang)
	tree, err := parser.Parse(content)
	if err != nil {
		return fmt.Errorf("ошибка парсинга %s: %w", filePath, err)
	}

	root := tree.RootNode()
	p.walkNode(fileNode.ID, root, string(content), "")

	p.linkMethodsToClasses()
	return nil
}

func nodeType(node *gotreesitter.Node, lang *gotreesitter.Language) string {
	if node == nil {
		return ""
	}
	return node.Type(lang)
}

func (p *PSIParser) walkNode(parentID int64, node *gotreesitter.Node, source string, contextName string) {
	t := nodeType(node, p.lang)

	switch t {
	case "type_declaration":
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child != nil {
				p.walkNode(parentID, child, source, contextName)
			}
		}

	case "type_spec":
		name := extractName(node, source, p.lang)
		if name != "" {
			// ← ДОБАВЛЯЕМ path и file
			classNode, _ := p.Graph.AddNode([]string{"class"}, map[string]interface{}{
				"name": name,
				"path": p.currentFilePath, // ← СВЯЗЬ С ФАЙЛОМ
				"file": p.currentFilePath, // ← ДЛЯ НАДЁЖНОСТИ
			})
			p.Graph.AddEdge("contains", parentID, classNode.ID)
			for i := 0; i < int(node.ChildCount()); i++ {
				child := node.Child(i)
				if child != nil {
					p.walkNode(classNode.ID, child, source, name)
				}
			}
		}

	case "method_declaration":
		name := extractName(node, source, p.lang)
		className := extractReceiverClass(node, source, p.lang)
		if className != "" {
			contextName = className
		}

		if name != "" {
			// ← ДОБАВЛЯЕМ path и file
			funcNode, _ := p.Graph.AddNode([]string{"function"}, map[string]interface{}{
				"name":      name,
				"class":     contextName,
				"path":      p.currentFilePath, // ← СВЯЗЬ С ФАЙЛОМ
				"file":      p.currentFilePath, // ← ДЛЯ НАДЁЖНОСТИ
				"is_method": true,
			})
			p.Graph.AddEdge("contains", parentID, funcNode.ID)
			for i := 0; i < int(node.ChildCount()); i++ {
				child := node.Child(i)
				if child != nil {
					p.walkNode(funcNode.ID, child, source, contextName)
				}
			}
		}

	case "function_declaration":
		name := extractName(node, source, p.lang)
		if name != "" {
			// ← ДОБАВЛЯЕМ path и file
			funcNode, _ := p.Graph.AddNode([]string{"function"}, map[string]interface{}{
				"name":      name,
				"path":      p.currentFilePath, // ← СВЯЗЬ С ФАЙЛОМ
				"file":      p.currentFilePath, // ← ДЛЯ НАДЁЖНОСТИ
				"is_method": false,
			})
			p.Graph.AddEdge("contains", parentID, funcNode.ID)
			for i := 0; i < int(node.ChildCount()); i++ {
				child := node.Child(i)
				if child != nil {
					p.walkNode(funcNode.ID, child, source, contextName)
				}
			}
		}

	case "call_expression":
		calledName := extractCallName(node, source, p.lang)
		if calledName != "" {
			// ← ДОБАВЛЯЕМ path
			callNode, _ := p.Graph.AddNode([]string{"call"}, map[string]interface{}{
				"name":    calledName,
				"context": contextName,
				"path":    p.currentFilePath, // ← СВЯЗЬ С ФАЙЛОМ
			})
			p.Graph.AddEdge("contains", parentID, callNode.ID)
		}

	default:
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child != nil {
				p.walkNode(parentID, child, source, contextName)
			}
		}
	}
}

// linkMethodsToClasses связывает методы с классами
func (p *PSIParser) linkMethodsToClasses() {
	functions := p.Graph.FindNodes(graph.Query{Label: "function"})
	classes := p.Graph.FindNodes(graph.Query{Label: "class"})

	for _, fn := range functions {
		className, _ := fn.GetProp("class")
		if className == nil || className == "" {
			continue
		}
		classStr := strings.TrimLeft(fmt.Sprintf("%v", className), "*&")
		for _, class := range classes {
			if name, _ := class.GetProp("name"); fmt.Sprintf("%v", name) == classStr {
				neighbors := p.Graph.GetNeighbors(class.ID, graph.DirectionOutgoing)
				found := false
				for _, n := range neighbors {
					if n.ID == fn.ID {
						found = true
						break
					}
				}
				if !found {
					p.Graph.AddEdge("contains", class.ID, fn.ID)
				}
				break
			}
		}
	}
}

// extractName извлекает имя
func extractName(node *gotreesitter.Node, source string, lang *gotreesitter.Language) string {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		ct := nodeType(child, lang)
		if ct == "field_identifier" || ct == "identifier" || ct == "type_identifier" {
			name := content(child, source)
			if name != "" && name != "type" && name != "struct" {
				return name
			}
		}
		if ct == "type_spec" || ct == "type_identifier" {
			for j := 0; j < int(child.ChildCount()); j++ {
				grandchild := child.Child(j)
				gct := nodeType(grandchild, lang)
				if gct == "field_identifier" || gct == "identifier" || gct == "type_identifier" {
					name := content(grandchild, source)
					if name != "" && name != "type" && name != "struct" {
						return name
					}
				}
			}
		}
	}
	return ""
}

// extractReceiverClass извлекает класс из ресивера метода
func extractReceiverClass(node *gotreesitter.Node, source string, lang *gotreesitter.Language) string {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		ct := nodeType(child, lang)
		if ct == "parameter_list" {
			for j := 0; j < int(child.ChildCount()); j++ {
				param := child.Child(j)
				pt := nodeType(param, lang)
				if pt == "parameter_declaration" {
					for k := 0; k < int(param.ChildCount()); k++ {
						typeNode := param.Child(k)
						tt := nodeType(typeNode, lang)
						if tt == "type_identifier" || tt == "pointer_type" {
							name := content(typeNode, source)
							name = strings.TrimLeft(name, "*&")
							return name
						}
					}
				}
			}
		}
	}
	return ""
}

// extractCallName извлекает имя вызываемой функции
func extractCallName(node *gotreesitter.Node, source string, lang *gotreesitter.Language) string {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		ct := nodeType(child, lang)
		if ct == "selector_expression" {
			for j := 0; j < int(child.ChildCount()); j++ {
				sub := child.Child(j)
				st := nodeType(sub, lang)
				if st == "field_identifier" {
					return content(sub, source)
				}
			}
		}
		if ct == "identifier" || ct == "field_identifier" {
			return content(child, source)
		}
	}
	return ""
}

// resolveAllCalls резолвит вызовы
func (p *PSIParser) resolveAllCalls() {
	calls := p.Graph.FindNodes(graph.Query{Label: "call"})
	for _, call := range calls {
		refs := p.Graph.GetReferences(call.ID, graph.DirectionOutgoing)
		if len(refs) > 0 {
			continue
		}

		contextName, _ := call.GetProp("context")
		if contextName != nil && contextName != "" {
			p.Graph.ResolveCallWithContext(call.ID, 0, fmt.Sprintf("%v", contextName))
		} else {
			p.Graph.ResolveCall(call.ID, 0)
		}
	}
}

// content возвращает текст узла
func content(node *gotreesitter.Node, source string) string {
	return source[node.StartByte():node.EndByte()]
}
