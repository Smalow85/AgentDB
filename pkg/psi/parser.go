package psi

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/odvcencio/gotreesitter"

	"agent-db/pkg/graph"
)

type PSIParser struct {
	Graph  *graph.Graph
	fileID int64
	lang   *gotreesitter.Language
}

func NewPSIParser(g *graph.Graph) *PSIParser {
	return &PSIParser{
		Graph: g,
	}
}

func (p *PSIParser) ParseRepo(repoPath string) error {
	return filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		lang := DetectLanguage(path)
		if lang == nil {
			return nil
		}
		return p.ParseFile(path, lang)
	})
}

func (p *PSIParser) ParseFile(filePath string, lang *LanguageInfo) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("ошибка чтения %s: %w", filePath, err)
	}

	fileNode, _ := p.Graph.AddNode([]string{"file", lang.Name}, map[string]interface{}{
		"path": filePath,
	})
	p.fileID = fileNode.ID

	p.lang = lang.GetLang()
	parser := gotreesitter.NewParser(p.lang)
	tree, err := parser.Parse(content)
	if err != nil {
		return fmt.Errorf("ошибка парсинга %s: %w", filePath, err)
	}

	root := tree.RootNode()
	fmt.Printf("[DEBUG] root type: %v\n", root.Type(p.lang))

	// DEBUG: печатаем все узлы
	printAllNodes(root, p.lang, 0)

	p.walkNode(fileNode.ID, root, string(content), "")
	return nil
}

func (p *PSIParser) walkNode(parentID int64, node *gotreesitter.Node, source string, contextName string) {
	nodeType := node.Type(p.lang)

	// Класс
	if isClassDef(nodeType) {
		name := extractName(node, source, p.lang)
		fmt.Printf("[DEBUG] class: %s\n", name)
		classNode, _ := p.Graph.AddNode([]string{"class"}, map[string]interface{}{
			"name": name,
		})
		p.Graph.AddEdge("contains", parentID, classNode.ID)

		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child != nil {
				p.walkNode(classNode.ID, child, source, name)
			}
		}
		return
	}

	// Функция
	if isFuncDef(nodeType) {
		name := extractName(node, source, p.lang)
		fmt.Printf("[DEBUG] function: %s\n", name)
		funcNode, _ := p.Graph.AddNode([]string{"function"}, map[string]interface{}{
			"name": name,
		})
		p.Graph.AddEdge("contains", parentID, funcNode.ID)

		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child != nil {
				p.walkNode(funcNode.ID, child, source, name)
			}
		}
		return
	}

	// Вызов
	if nodeType == "call_expression" {
		calledName := extractCallName(node, source, p.lang)
		fmt.Printf("[DEBUG] call: %s\n", calledName)
		if calledName != "" {
			callNode, _ := p.Graph.AddNode([]string{"call"}, map[string]interface{}{
				"name": calledName,
			})
			p.Graph.AddEdge("contains", parentID, callNode.ID)
			p.Graph.ResolveCall(callNode.ID, parentID)
		}
	}

	// Рекурсия
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child != nil {
			p.walkNode(parentID, child, source, contextName)
		}
	}
}

func isClassDef(t string) bool {
	switch t {
	case "type_declaration", "type_spec", "struct_type", "interface_type":
		return true
	}
	return false
}

func isFuncDef(t string) bool {
	switch t {
	case "method_declaration", "function_declaration", "function_definition":
		return true
	}
	return false
}

func isCall(t string) bool {
	return t == "call_expression"
}

func extractName(node *gotreesitter.Node, source string, lang *gotreesitter.Language) string {
	// Ищем identifier
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child != nil {
			t := child.Type(lang)
			if t == "identifier" || t == "type_identifier" || t == "field_identifier" {
				return content(child, source)
			}
		}
	}
	return "unknown"
}

func extractCallName(node *gotreesitter.Node, source string, lang *gotreesitter.Language) string {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child != nil {
			t := child.Type(lang)
			if t == "identifier" || t == "property_identifier" || t == "field_identifier" {
				return content(child, source)
			}
		}
	}
	return ""
}

func content(node *gotreesitter.Node, source string) string {
	return source[node.StartByte():node.EndByte()]
}

func printAllNodes(node *gotreesitter.Node, lang *gotreesitter.Language, indent int) {
	for i := 0; i < indent; i++ {
		fmt.Print("  ")
	}
	fmt.Printf("%s\n", node.Type(lang))
	for i := 0; i < int(node.ChildCount()); i++ {
		if child := node.Child(i); child != nil {
			printAllNodes(child, lang, indent+1)
		}
	}
}
