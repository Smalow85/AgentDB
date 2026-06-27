package psi

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agent-db/pkg/graph"

	"github.com/odvcencio/gotreesitter"
)

// ============================================================
// ОСНОВНОЙ ПАРСЕР
// ============================================================

type PSIParser struct {
	Graph           *graph.Graph
	fileID          int64
	currentFilePath string
	currentPackage  string
	lang            *gotreesitter.Language
}

func NewPSIParser(g *graph.Graph) *PSIParser {
	return &PSIParser{Graph: g}
}

// ParseRepo обходит все файлы и парсит их
func (p *PSIParser) ParseRepo(repoPath string) error {
	fmt.Printf("Парсинг репозитория: %s\n", repoPath)

	err := filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		langInfo := DetectLanguage(path)
		if langInfo == nil {
			return nil
		}
		return p.ParseFile(path, langInfo)
	})
	if err != nil {
		return err
	}

	fmt.Printf("[DEBUG] ParseRepo: all files parsed, resolving cross-file calls\n")
	p.resolveCrossFileCalls()
	fmt.Printf("[DEBUG] ParseRepo: building file edges\n")
	p.buildFileEdges()
	fmt.Printf("[DEBUG] ParseRepo: finished\n")
	return nil
}

// ParseFile парсит один файл и заполняет граф
func (p *PSIParser) ParseFile(filePath string, langInfo *LanguageInfo) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("ошибка чтения %s: %w", filePath, err)
	}

	normalizedPath := filepath.ToSlash(filePath)
	p.currentFilePath = normalizedPath

	// --- 1. Узел файла ---
	fileNode, _ := p.Graph.AddNode([]string{"file", langInfo.Name}, map[string]interface{}{
		"path": normalizedPath,
		"name": filepath.Base(normalizedPath),
	})
	p.fileID = fileNode.ID
	p.currentPackage = ""

	p.lang = langInfo.GetLang()
	parser := gotreesitter.NewParser(p.lang)
	tree, err := parser.Parse(content)
	if err != nil {
		return fmt.Errorf("ошибка парсинга %s: %w", filePath, err)
	}

	root := tree.RootNode()
	p.walkNode(p.fileID, root, string(content), "")

	return nil
}

// ============================================================
// ОБХОД AST (упрощённый, без реестров и мьютексов)
// ============================================================

func (p *PSIParser) walkNode(parentID int64, node *gotreesitter.Node, source string, contextClass string) {
	t := nodeType(node, p.lang)

	switch t {
	case "package_clause":
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if nodeType(child, p.lang) == "package_identifier" {
				pkgName := content(child, source)
				p.currentPackage = pkgName
				break
			}
		}
		p.walkChildren(node, parentID, source, contextClass)

	case "type_declaration":
		p.walkChildren(node, parentID, source, contextClass)

	case "type_spec":
		name := extractName(node, source, p.lang)
		if name != "" {
			classNode, _ := p.Graph.AddNode([]string{"class"}, map[string]interface{}{
				"name":       name,
				"package":    p.currentPackage,
				"path":       p.currentFilePath,
				"file":       p.currentFilePath,
				"start_byte": node.StartByte(),
				"end_byte":   node.EndByte(),
			})
			p.Graph.AddEdge("contains", parentID, classNode.ID)
			p.walkChildren(node, classNode.ID, source, name)
		}

	case "method_declaration":
		name := extractName(node, source, p.lang)
		className := extractReceiverClass(node, source, p.lang)
		if className == "" {
			className = contextClass
		}
		if name != "" {
			funcNode, _ := p.Graph.AddNode([]string{"function"}, map[string]interface{}{
				"name":       name,
				"class":      className,
				"package":    p.currentPackage,
				"path":       p.currentFilePath,
				"file":       p.currentFilePath,
				"is_method":  true,
				"start_byte": node.StartByte(),
				"end_byte":   node.EndByte(),
			})
			if className != "" {
				classID := p.findClassNode(className)
				if classID != 0 {
					p.Graph.AddEdge("contains", classID, funcNode.ID)
				} else {
					p.Graph.AddEdge("contains", parentID, funcNode.ID)
				}
			} else {
				p.Graph.AddEdge("contains", parentID, funcNode.ID)
			}
			p.walkChildren(node, funcNode.ID, source, className)
		}

	case "function_declaration":
		name := extractName(node, source, p.lang)
		if name != "" {
			funcNode, _ := p.Graph.AddNode([]string{"function"}, map[string]interface{}{
				"name":      name,
				"package":   p.currentPackage,
				"path":      p.currentFilePath,
				"file":      p.currentFilePath,
				"is_method": false,
			})
			p.Graph.AddEdge("contains", parentID, funcNode.ID)
			p.walkChildren(node, funcNode.ID, source, contextClass)
		}

	case "call_expression":
		callName := extractCallName(node, source, p.lang)
		if callName != "" {
			callNode, _ := p.Graph.AddNode([]string{"call"}, map[string]interface{}{
				"name":    callName,
				"context": contextClass,
				"package": p.currentPackage,
				"path":    p.currentFilePath,
				"file":    p.currentFilePath,
			})
			p.Graph.AddEdge("contains", parentID, callNode.ID)
		}

	default:
		p.walkChildren(node, parentID, source, contextClass)
	}
}

// walkChildren обходит детей
func (p *PSIParser) walkChildren(node *gotreesitter.Node, parentID int64, source string, contextClass string) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child != nil {
			p.walkNode(parentID, child, source, contextClass)
		}
	}
}

// ============================================================
// ВСПОМОГАТЕЛЬНЫЕ ФУНКЦИИ
// ============================================================

// findClassNode ищет узел класса по имени в текущем файле или пакете
func (p *PSIParser) findClassNode(className string) int64 {
	// Ищем по всем узлам класса
	nodes := p.Graph.FindNodes(graph.Query{Label: "class", Property: "name", Value: className})
	for _, n := range nodes {
		if file, ok := n.Properties["file"].(string); ok && file == p.currentFilePath {
			return n.ID
		}
	}
	if len(nodes) > 0 {
		return nodes[0].ID
	}
	return 0
}

// ============================================================
// МЕЖФАЙЛОВЫЙ РЕЗОЛВИНГ
// ============================================================

func (p *PSIParser) resolveCrossFileCalls() {
	// Получаем все call-узлы
	callNodes := p.Graph.FindNodes(graph.Query{Label: "call"})
	fmt.Printf("[DEBUG] Resolving %d calls\n", len(callNodes))

	for _, call := range callNodes {
		// Находим родительскую функцию
		sourceFuncID := p.findEnclosingFunction(call.ID)
		if sourceFuncID == 0 {
			continue
		}

		// Получаем имя вызываемой функции и пакет
		callName, _ := call.GetProp("name")
		callPkg, _ := call.GetProp("package")
		contextClass, _ := call.GetProp("context")

		// Ищем целевую функцию
		targetID := p.findTargetFunction(
			fmt.Sprintf("%v", callName),
			fmt.Sprintf("%v", callPkg),
			fmt.Sprintf("%v", contextClass),
		)

		if targetID != 0 {
			// Создаём ребро call между функциями
			p.Graph.AddEdge("call", sourceFuncID, targetID)
		} else {
			// Можно создать unresolved узел (опционально)
			unresolvedNode, _ := p.Graph.AddNode([]string{"unresolved_call"}, map[string]interface{}{
				"name":     callName,
				"package":  callPkg,
				"context":  contextClass,
				"file":     p.currentFilePath,
				"resolved": false,
			})
			p.Graph.AddEdge("contains", sourceFuncID, unresolvedNode.ID)
		}
	}
}

// findEnclosingFunction поднимается по цепочке contains и находит родительскую функцию
func (p *PSIParser) findEnclosingFunction(nodeID int64) int64 {
	visited := make(map[int64]bool)
	return p.findEnclosingFunctionRec(nodeID, visited)
}

func (p *PSIParser) findEnclosingFunctionRec(nodeID int64, visited map[int64]bool) int64 {
	if visited[nodeID] {
		return 0
	}
	visited[nodeID] = true

	edges := p.Graph.GetEdges(nodeID, graph.DirectionIncoming)
	for _, edge := range edges {
		if edge.Type == "contains" {
			parent := p.Graph.GetNode(edge.FromID)
			if parent != nil && parent.HasLabel("function") {
				return parent.ID
			}
			return p.findEnclosingFunctionRec(edge.FromID, visited)
		}
	}
	return 0
}

// findTargetFunction ищет функцию по имени, пакету и контексту
func (p *PSIParser) findTargetFunction(name, pkg, contextClass string) int64 {
	// 1. Если есть контекст (класс), ищем метод этого класса
	if contextClass != "" && contextClass != "<nil>" {
		// Ищем класс
		classNodes := p.Graph.FindNodes(graph.Query{Label: "class", Property: "name", Value: contextClass})
		for _, class := range classNodes {
			// Ищем методы этого класса (по ребру contains)
			methods := p.Graph.GetNeighbors(class.ID, graph.DirectionOutgoing)
			for _, m := range methods {
				if m.HasLabel("function") {
					if mName, ok := m.Properties["name"].(string); ok && mName == name {
						return m.ID
					}
				}
			}
		}
	}

	// 2. Ищем функцию в том же пакете
	if pkg != "" && pkg != "<nil>" {
		funcNodes := p.Graph.FindNodes(graph.Query{Label: "function", Property: "name", Value: name})
		for _, fn := range funcNodes {
			if fnPkg, ok := fn.Properties["package"].(string); ok && fnPkg == pkg {
				return fn.ID
			}
		}
	}

	// 3. Ищем глобально по имени (без учёта пакета)
	funcNodes := p.Graph.FindNodes(graph.Query{Label: "function", Property: "name", Value: name})
	if len(funcNodes) > 0 {
		// Если несколько, берём первую (можно улучшить)
		return funcNodes[0].ID
	}

	return 0
}

// ============================================================
// АГРЕГАЦИЯ СВЯЗЕЙ МЕЖДУ ФАЙЛАМИ
// ============================================================

func (p *PSIParser) buildFileEdges() {
	// Получаем все рёбра типа "call"
	allEdges := p.Graph.GetAllEdges()
	callEdges := []*graph.Edge{}
	for _, e := range allEdges {
		if e.Type == "call" {
			callEdges = append(callEdges, e)
		}
	}

	if len(callEdges) == 0 {
		fmt.Println("[DEBUG] No call edges found, skipping file edges")
		return
	}

	// Маппинг: ID узла → путь к файлу
	allNodes := p.Graph.GetAllNodes()
	nodeToFile := make(map[int64]string)
	for _, node := range allNodes {
		if node.HasLabel("file") {
			if path, ok := node.Properties["path"].(string); ok {
				nodeToFile[node.ID] = path
			}
		} else {
			if file, ok := node.Properties["file"].(string); ok {
				nodeToFile[node.ID] = file
			} else if path, ok := node.Properties["path"].(string); ok {
				nodeToFile[node.ID] = path
			}
		}
	}

	// Маппинг путь → ID файла
	filePathToID := make(map[string]int64)
	for _, node := range allNodes {
		if node.HasLabel("file") {
			if path, ok := node.Properties["path"].(string); ok {
				filePathToID[path] = node.ID
			}
		}
	}

	// Агрегируем
	fileEdgesMap := make(map[string]bool)
	for _, edge := range callEdges {
		fromFile := nodeToFile[edge.FromID]
		toFile := nodeToFile[edge.ToID]
		if fromFile == "" || toFile == "" || fromFile == toFile {
			continue
		}
		fromID, ok1 := filePathToID[fromFile]
		toID, ok2 := filePathToID[toFile]
		if !ok1 || !ok2 {
			continue
		}
		key := fmt.Sprintf("%d|%d", fromID, toID)
		if fromID > toID {
			key = fmt.Sprintf("%d|%d", toID, fromID)
		}
		if !fileEdgesMap[key] {
			fileEdgesMap[key] = true
			p.Graph.AddEdge("file-call", fromID, toID)
		}
	}

	fmt.Printf("[DEBUG] Added %d file-call edges\n", len(fileEdgesMap))
}

// ============================================================
// ВСПОМОГАТЕЛЬНЫЕ ФУНКЦИИ ДЛЯ РАБОТЫ С AST
// ============================================================

func nodeType(node *gotreesitter.Node, lang *gotreesitter.Language) string {
	if node == nil {
		return ""
	}
	return node.Type(lang)
}

func content(node *gotreesitter.Node, source string) string {
	return source[node.StartByte():node.EndByte()]
}

func extractName(node *gotreesitter.Node, source string, lang *gotreesitter.Language) string {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		ct := nodeType(child, lang)
		if ct == "field_identifier" || ct == "identifier" || ct == "type_identifier" {
			return content(child, source)
		}
	}
	return ""
}

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

func extractCallName(node *gotreesitter.Node, source string, lang *gotreesitter.Language) string {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		ct := nodeType(child, lang)
		if ct == "selector_expression" {
			for j := 0; j < int(child.ChildCount()); j++ {
				sub := child.Child(j)
				if nodeType(sub, lang) == "field_identifier" {
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
