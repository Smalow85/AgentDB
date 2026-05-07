package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"time"
    "os"

	"agent-db/pkg/executor"
	"agent-db/pkg/graph"
	"agent-db/pkg/psi"
	"agent-db/pkg/storage"
)

//go:embed static
var staticFiles embed.FS

type Server struct {
	Exec     *executor.Executor
	PSIGraph *graph.Graph
	PSIDisk  *storage.DiskManager
	parser   *psi.PSIParser
}

func NewServer(exec *executor.Executor) *Server {
	psiDisk, _ := storage.NewDiskManager("psi_graph.dat")
	psiBP := storage.NewBufferPool(100, psiDisk)
	psiStore := graph.NewGraphStore(psiBP, psiDisk)
	g := graph.NewGraph("psigraph", psiStore)

	// Пробуем загрузить сохранённый граф
	if err := g.Load(); err != nil {
		fmt.Printf("Ошибка загрузки графа: %v\n", err)
	} else {
		nodes := g.FindNodes(graph.Query{})
		fmt.Printf("✓ Загружен сохранённый граф (%d узлов)\n", len(nodes))
	}

	return &Server{
		Exec:     exec,
		PSIGraph: g,
		PSIDisk:  psiDisk,
		parser:   psi.NewPSIParser(g),
	}
}

func (s *Server) Start(addr string) error {
	http.HandleFunc("/api/query", s.handleQuery)
	http.HandleFunc("/api/tables", s.handleTables)
	http.HandleFunc("/api/schema", s.handleSchema)
	http.HandleFunc("/api/parse", s.handleParse)
	http.HandleFunc("/api/graph", s.handleGraph)
	http.HandleFunc("/api/graphs", s.handleGraphList)
	http.HandleFunc("/api/psi/query", s.handlePSIQuery)

	staticFS, _ := fs.Sub(staticFiles, "static")
	http.Handle("/", http.FileServer(http.FS(staticFS)))

	fmt.Printf("🌐 AgentDB Web UI: http://localhost%s\n", addr)
	return http.ListenAndServe(addr, nil)
}

// ========== SQL API ==========

type QueryRequest struct {
	SQL string `json:"sql"`
}

type QueryResponse struct {
	Result string `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}

	var req QueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, QueryResponse{Error: "неверный формат запроса"})
		return
	}

	result, err := s.Exec.Execute(req.SQL)
	if err != nil {
		writeJSON(w, QueryResponse{Error: err.Error()})
		return
	}

	writeJSON(w, QueryResponse{Result: result})
}

func (s *Server) handleTables(w http.ResponseWriter, r *http.Request) {
	tables := s.Exec.ListTables()
	writeJSON(w, map[string]interface{}{"tables": tables})
}

func (s *Server) handleSchema(w http.ResponseWriter, r *http.Request) {
	tableName := r.URL.Query().Get("table")
	if tableName == "" {
		writeJSON(w, map[string]string{"error": "укажите имя таблицы"})
		return
	}
	result, err := s.Exec.Execute(fmt.Sprintf("SELECT * FROM %s LIMIT 0", tableName))
	if err != nil {
		writeJSON(w, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, map[string]string{"schema": result})
}

// ========== Graph API ==========

type ParseRequest struct {
	Path string `json:"path"`
}

func (s *Server) handleParse(w http.ResponseWriter, r *http.Request) {
    var req ParseRequest
    json.NewDecoder(r.Body).Decode(&req)

    if req.Path == "" {
        writeJSON(w, map[string]string{"error": "укажите путь к репозиторию"})
        return
    }

    // Создаём новый граф
    psiBP := storage.NewBufferPool(100, s.PSIDisk)
    psiStore := graph.NewGraphStore(psiBP, s.PSIDisk)
    s.PSIGraph = graph.NewGraph("psigraph", psiStore)
    s.parser = psi.NewPSIParser(s.PSIGraph)

    startTime := time.Now()
    err := s.parser.ParseRepo(req.Path)
    elapsed := time.Since(startTime)

    if err != nil {
        writeJSON(w, map[string]string{"error": err.Error()})
        return
    }

    // Статистика ДО сохранения
    files := s.PSIGraph.FindNodes(graph.Query{Label: "file"})
    classes := s.PSIGraph.FindNodes(graph.Query{Label: "class"})
    functions := s.PSIGraph.FindNodes(graph.Query{Label: "function"})
    calls := s.PSIGraph.FindNodes(graph.Query{Label: "call"})

    fmt.Printf("[DEBUG] Перед сохранением: files=%d classes=%d functions=%d calls=%d\n",
        len(files), len(classes), len(functions), len(calls))

    // Сохраняем граф
    if err := s.PSIGraph.SaveToDisk(); err != nil {
        fmt.Printf("[ERROR] Ошибка сохранения графа: %v\n", err)
    } else {
        fmt.Println("[DEBUG] Граф сохранён успешно")
    }

    // Проверяем файл
    stat, _ := os.Stat("psi_graph.dat")
    if stat != nil {
        fmt.Printf("[DEBUG] Размер psi_graph.dat: %d байт\n", stat.Size())
    }

    writeJSON(w, map[string]interface{}{
        "status":    "ok",
        "time_ms":   elapsed.Milliseconds(),
        "files":     len(files),
        "classes":   len(classes),
        "functions": len(functions),
        "calls":     len(calls),
    })
}

func (s *Server) handlePSIQuery(w http.ResponseWriter, r *http.Request) {
    var req struct {
        Type  string `json:"type"`  // "class", "method", "callers", "callees"
        Name  string `json:"name"`
        Class string `json:"class"`
    }
    json.NewDecoder(r.Body).Decode(&req)

    switch req.Type {
    case "class":
        nodes := s.PSIGraph.FindNodes(graph.Query{
            Label: "class", Property: "name", Value: req.Name,
        })
        if len(nodes) > 0 {
            methods := s.PSIGraph.GetNeighbors(nodes[0].ID, graph.DirectionOutgoing)
            var result []map[string]interface{}
            for _, m := range methods {
                name, _ := m.GetProp("name")
                result = append(result, map[string]interface{}{
                    "name": name,
                    "type": m.Labels[0],
                })
            }
            writeJSON(w, map[string]interface{}{
                "class":  req.Name,
                "methods": result,
            })
            return
        }

    case "callers":
		callers := s.PSIGraph.GetCallersByName(req.Name)
		writeJSON(w, map[string]interface{}{
			"function": req.Name,
			"callers":  callers,
		})
		return

	case "callees":
		callees := s.PSIGraph.GetCalleesByName(req.Name)
		writeJSON(w, map[string]interface{}{
			"function": req.Name,
			"callees":  callees,
		})
		return
	}

    writeJSON(w, map[string]string{"error": "не найдено"})
}

func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	type GraphNode struct {
		ID    int64                  `json:"id"`
		Label string                 `json:"label"`
		Type  string                 `json:"type"`
		Props map[string]interface{} `json:"props"`
	}

	type GraphEdge struct {
		From int64  `json:"from"`
		To   int64  `json:"to"`
		Type string `json:"type"`
	}

	var result struct {
		Nodes []GraphNode `json:"nodes"`
		Edges []GraphEdge `json:"edges"`
	}

	seen := make(map[int64]bool)

	classes := s.PSIGraph.FindNodes(graph.Query{Label: "class"})
	functions := s.PSIGraph.FindNodes(graph.Query{Label: "function"})
	calls := s.PSIGraph.FindNodes(graph.Query{Label: "call"})
	allNodes := append(classes, functions...)
	allNodes = append(allNodes, calls...)

	for _, node := range allNodes {
		if seen[node.ID] {
			continue
		}
		seen[node.ID] = true

		nodeType := "function"
		if node.HasLabel("class") {
			nodeType = "class"
		}
		if node.HasLabel("call") {
			nodeType = "call"
		}
		name, _ := node.GetProp("name")
		result.Nodes = append(result.Nodes, GraphNode{
			ID:    node.ID,
			Label: fmt.Sprintf("%v", name),
			Type:  nodeType,
			Props: node.Properties,
		})

		// Рёбра
		edges := s.PSIGraph.GetEdges(node.ID, graph.DirectionOutgoing)
		for _, edge := range edges {
			result.Edges = append(result.Edges, GraphEdge{
				From: edge.FromID,
				To:   edge.ToID,
				Type: edge.Type,
			})
		}
	}

	writeJSON(w, result)
}

func (s *Server) handleGraphList(w http.ResponseWriter, r *http.Request) {
	// Возвращаем информацию о текущем графе
	nodes := s.PSIGraph.FindNodes(graph.Query{})
	classes := s.PSIGraph.FindNodes(graph.Query{Label: "class"})
	functions := s.PSIGraph.FindNodes(graph.Query{Label: "function"})

	writeJSON(w, map[string]interface{}{
		"graphs": []map[string]interface{}{
			{
				"name":      s.PSIGraph.Name,
				"nodes":     len(nodes),
				"classes":   len(classes),
				"functions": len(functions),
			},
		},
	})
}

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(data)
}