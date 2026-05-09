package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"time"
    "os"
	"strings"

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
	s := &Server{
        Exec:     exec,
        PSIGraph: g,
        PSIDisk:  psiDisk,
        parser:   psi.NewPSIParser(g),
    }
    s.initContextManager()
    return s
}

func (s *Server) Start(addr string) error {
	http.HandleFunc("/api/query", s.handleQuery)
	http.HandleFunc("/api/tables", s.handleTables)
	http.HandleFunc("/api/schema", s.handleSchema)
	http.HandleFunc("/api/parse", s.handleParse)
	http.HandleFunc("/api/graph", s.handleGraph)
	http.HandleFunc("/api/graphs", s.handleGraphList)
	http.HandleFunc("/api/psi/query", s.handlePSIQuery)
	http.HandleFunc("/api/psi/path", s.handlePSIPath)
	http.HandleFunc("/api/psi/context", s.handlePSIContext)
	http.HandleFunc("/api/context/metaspace", s.handleMetaspaceLoad)
	http.HandleFunc("/api/context/metaspace/add", s.handleMetaspaceAdd)
	http.HandleFunc("/api/context/instruction", s.handleInstructionAdd)
	http.HandleFunc("/api/context/reason", s.handleReasonAdd)
	http.HandleFunc("/api/context/buffer", s.handleBufferAdd)
	http.HandleFunc("/api/context/inference", s.handleInferenceAdd)
	http.HandleFunc("/api/context/current", s.handleContextCurrent)
	http.HandleFunc("/api/context/rollback", s.handleContextRollback)
	http.HandleFunc("/api/context/gc", s.handleContextGC)

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

	files := s.PSIGraph.FindNodes(graph.Query{Label: "file"})
	classes := s.PSIGraph.FindNodes(graph.Query{Label: "class"})
	functions := s.PSIGraph.FindNodes(graph.Query{Label: "function"})
	calls := s.PSIGraph.FindNodes(graph.Query{Label: "call"})
	allNodes := append(files, classes...)
	allNodes = append(allNodes, functions...)
	allNodes = append(allNodes, calls...)

	for _, node := range allNodes {
		if seen[node.ID] {
			continue
		}
		seen[node.ID] = true

		nodeType := "function"
		if node.HasLabel("file") {
			nodeType = "file"
		} else if node.HasLabel("class") {
			nodeType = "class"
		} else if node.HasLabel("call") {
			nodeType = "call"
		}

		var label string
		if name, ok := node.GetProp("name"); ok {
			label = fmt.Sprintf("%v", name)
		} else if path, ok := node.GetProp("path"); ok {
			label = fmt.Sprintf("%v", path)
		}

		result.Nodes = append(result.Nodes, GraphNode{
			ID:    node.ID,
			Label: label,
			Type:  nodeType,
			Props: node.Properties,
		})

		// Outgoing edges
		edges := s.PSIGraph.GetEdges(node.ID, graph.DirectionOutgoing)
		for _, edge := range edges {
			result.Edges = append(result.Edges, GraphEdge{
				From: edge.FromID,
				To:   edge.ToID,
				Type: edge.Type,
			})
		}

		// References as edges
		refs := s.PSIGraph.GetReferences(node.ID, graph.DirectionOutgoing)
		for _, ref := range refs {
			if ref.IsResolved {
				result.Edges = append(result.Edges, GraphEdge{
					From: ref.SourceID,
					To:   ref.TargetID,
					Type: ref.Type,
				})
			}
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

func (s *Server) handlePSIPath(w http.ResponseWriter, r *http.Request) {
    var req struct {
        From string `json:"from"`
        To   string `json:"to"`
    }
    json.NewDecoder(r.Body).Decode(&req)

    paths, err := s.PSIGraph.FindCallPath(req.From, req.To)
    if err != nil {
        writeJSON(w, map[string]string{"error": err.Error()})
        return
    }

    writeJSON(w, map[string]interface{}{
        "from":  req.From,
        "to":    req.To,
        "paths": paths,
    })
}

func (s *Server) handlePSIContext(w http.ResponseWriter, r *http.Request) {
    classes := s.PSIGraph.FindNodes(graph.Query{Label: "class"})
    functions := s.PSIGraph.FindNodes(graph.Query{Label: "function"})
    calls := s.PSIGraph.FindNodes(graph.Query{Label: "call"})

    var sb strings.Builder

    sb.WriteString("=== Структура проекта ===\n\n")

    for _, class := range classes {
        name, _ := class.GetProp("name")
        sb.WriteString(fmt.Sprintf("📦 Класс: %s\n", name))

        // Методы класса
        children := s.PSIGraph.GetNeighbors(class.ID, graph.DirectionOutgoing)
        for _, child := range children {
            if child.HasLabel("function") {
                methodName, _ := child.GetProp("name")

                // Найти вызовы внутри метода
                grandchildren := s.PSIGraph.GetNeighbors(child.ID, graph.DirectionOutgoing)
                var callNames []string
                for _, gc := range grandchildren {
                    if gc.HasLabel("call") {
                        cn, _ := gc.GetProp("name")
                        callNames = append(callNames, fmt.Sprintf("%v", cn))
                    }
                }

                if len(callNames) > 0 {
                    sb.WriteString(fmt.Sprintf("  ├─ %s() → %s\n", methodName, strings.Join(callNames, ", ")))
                } else {
                    sb.WriteString(fmt.Sprintf("  ├─ %s()\n", methodName))
                }
            }
        }
        sb.WriteString("\n")
    }

    // Функции без класса
    var orphanFuncs []string
    for _, fn := range functions {
        class, _ := fn.GetProp("class")
        if class == nil || class == "" {
            name, _ := fn.GetProp("name")
            orphanFuncs = append(orphanFuncs, fmt.Sprintf("%v", name))
        }
    }
    if len(orphanFuncs) > 0 {
        sb.WriteString("📌 Функции вне классов: " + strings.Join(orphanFuncs, ", ") + "\n")
    }

    // Статистика
    sb.WriteString(fmt.Sprintf("\n📊 Статистика: %d классов, %d функций, %d вызовов\n",
        len(classes), len(functions), len(calls)))

    writeJSON(w, map[string]string{"context": sb.String()})
}

func (s *Server) handleAgentLoop(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID string `json:"session_id"`
		Message   string `json:"message"`
		LLMKey    string `json:"llm_key"`
		Model     string `json:"model"`
		BaseURL   string `json:"base_url"`
	}

	if req.BaseURL == "" {
    	writeJSON(w, map[string]string{"error": "не указан base_url"})
    	return
	}

	if req.Model == "" {
    	writeJSON(w, map[string]string{"error": "не указан model"})
    	return
	}

	json.NewDecoder(r.Body).Decode(&req)


	a := &agent.AgentLoop{
		SessionID: req.SessionID,
		PSIGraph:  s.PSIGraph,
		LLMKey:    req.LLMKey,
		Model:     req.Model,
		BaseURL:   req.BaseURL,
	}

	result, err := a.Run(req.Message)
	if err != nil {
		writeJSON(w, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, map[string]string{"result": result})
}

func (s *Server) handleMetaspaceLoad(w http.ResponseWriter, r *http.Request) {
    agentID := r.URL.Query().Get("agent_id")
    result, err := s.Exec.Execute(fmt.Sprintf(
        "SELECT content_type, content FROM metaspace WHERE agent_id = '%s' AND is_active = 1 ORDER BY priority DESC",
        agentID,
    ))
    if err != nil {
        writeJSON(w, map[string]string{"error": err.Error()})
        return
    }
    writeJSON(w, map[string]string{"metaspace": result})
}

// POST /api/context/metaspace/add — добавить в Metaspace
func (s *Server) handleMetaspaceAdd(w http.ResponseWriter, r *http.Request) {
    var req struct {
        AgentID     string `json:"agent_id"`
        ContentType string `json:"content_type"`
        Content     string `json:"content"`
        Priority    int    `json:"priority"`
    }
    json.NewDecoder(r.Body).Decode(&req)

    result, err := s.Exec.Execute(fmt.Sprintf(
        "INSERT INTO metaspace (agent_id, version, content_type, content, priority) VALUES ('%s', 1, '%s', '%s', %d)",
        req.AgentID, req.ContentType, escapeSQL(req.Content), req.Priority,
    ))
    if err != nil {
        writeJSON(w, map[string]string{"error": err.Error()})
        return
    }
    writeJSON(w, map[string]string{"status": "ok", "result": result})
}

// POST /api/context/instruction — добавить инструкцию
func (s *Server) handleInstructionAdd(w http.ResponseWriter, r *http.Request) {
    var req struct {
        SessionID string `json:"session_id"`
        Content   string `json:"content"`
        ParentID  int    `json:"parent_id"`
    }
    json.NewDecoder(r.Body).Decode(&req)

    // Получаем текущую эпоху
    s.Exec.Execute(fmt.Sprintf(
        "UPDATE sessions SET current_epoch = current_epoch + 1 WHERE id = '%s'",
        req.SessionID,
    ))

    result, err := s.Exec.Execute(fmt.Sprintf(
        "INSERT INTO instruction_stack (session_id, epoch, parent_id, content) VALUES ('%s', (SELECT current_epoch FROM sessions WHERE id = '%s'), %d, '%s')",
        req.SessionID, req.SessionID, req.ParentID, escapeSQL(req.Content),
    ))
    if err != nil {
        writeJSON(w, map[string]string{"error": err.Error()})
        return
    }
    writeJSON(w, map[string]string{"status": "ok", "result": result})
}

// POST /api/context/reason — добавить мысль
func (s *Server) handleReasonAdd(w http.ResponseWriter, r *http.Request) {
    var req struct {
        SessionID      string `json:"session_id"`
        Content        string `json:"content"`
        ThoughtType    string `json:"thought_type"`
        ParentThoughtID int   `json:"parent_thought_id"`
    }
    json.NewDecoder(r.Body).Decode(&req)

    s.Exec.Execute(fmt.Sprintf(
        "UPDATE sessions SET current_epoch = current_epoch + 1 WHERE id = '%s'",
        req.SessionID,
    ))

    result, err := s.Exec.Execute(fmt.Sprintf(
        "INSERT INTO reasoning_space (session_id, epoch, parent_thought_id, thought_type, content) VALUES ('%s', (SELECT current_epoch FROM sessions WHERE id = '%s'), %d, '%s', '%s')",
        req.SessionID, req.SessionID, req.ParentThoughtID, req.ThoughtType, escapeSQL(req.Content),
    ))
    if err != nil {
        writeJSON(w, map[string]string{"error": err.Error()})
        return
    }
    writeJSON(w, map[string]string{"status": "ok", "result": result})
}

// POST /api/context/buffer — добавить в буфер
func (s *Server) handleBufferAdd(w http.ResponseWriter, r *http.Request) {
    var req struct {
        SessionID string `json:"session_id"`
        Key       string `json:"key"`
        Data      string `json:"data"`
        TTL       int    `json:"ttl"`
    }
    json.NewDecoder(r.Body).Decode(&req)
    if req.TTL == 0 {
        req.TTL = 300
    }

    now := time.Now().Unix()
    s.Exec.Execute(fmt.Sprintf(
        "UPDATE sessions SET current_epoch = current_epoch + 1 WHERE id = '%s'",
        req.SessionID,
    ))

    result, err := s.Exec.Execute(fmt.Sprintf(
        "INSERT INTO buffer_space (session_id, epoch, key, data, ttl, created_at) VALUES ('%s', (SELECT current_epoch FROM sessions WHERE id = '%s'), '%s', '%s', %d, %d)",
        req.SessionID, req.SessionID, req.Key, escapeSQL(req.Data), req.TTL, now,
    ))
    if err != nil {
        writeJSON(w, map[string]string{"error": err.Error()})
        return
    }
    writeJSON(w, map[string]string{"status": "ok", "result": result})
}

// POST /api/context/inference — добавить вывод
func (s *Server) handleInferenceAdd(w http.ResponseWriter, r *http.Request) {
    var req struct {
        SessionID     string  `json:"session_id"`
        Conclusion    string  `json:"conclusion"`
        Confidence    float64 `json:"confidence"`
        InferenceType string  `json:"inference_type"`
    }
    json.NewDecoder(r.Body).Decode(&req)
    if req.InferenceType == "" {
        req.InferenceType = "assumption"
    }

    s.Exec.Execute(fmt.Sprintf(
        "UPDATE sessions SET current_epoch = current_epoch + 1 WHERE id = '%s'",
        req.SessionID,
    ))

    result, err := s.Exec.Execute(fmt.Sprintf(
        "INSERT INTO inference_space (session_id, epoch, conclusion, confidence, inference_type) VALUES ('%s', (SELECT current_epoch FROM sessions WHERE id = '%s'), '%s', %f, '%s')",
        req.SessionID, req.SessionID, escapeSQL(req.Conclusion), req.Confidence, req.InferenceType,
    ))
    if err != nil {
        writeJSON(w, map[string]string{"error": err.Error()})
        return
    }
    writeJSON(w, map[string]string{"status": "ok", "result": result})
}

// GET /api/context/current?session_id=abc — получить текущий контекст для LLM
func (s *Server) handleContextCurrent(w http.ResponseWriter, r *http.Request) {
    sessionID := r.URL.Query().Get("session_id")

    // Metaspace
    metaspace, _ := s.Exec.Execute(
        "SELECT content FROM metaspace WHERE is_active = 1 ORDER BY priority DESC")

    // Инструкции
    instructions, _ := s.Exec.Execute(fmt.Sprintf(
        "SELECT content FROM instruction_stack WHERE session_id = '%s' AND rolled_back = 0 ORDER BY depth",
        sessionID))

    // Мысли
    thoughts, _ := s.Exec.Execute(fmt.Sprintf(
        "SELECT thought_type || ': ' || content FROM reasoning_space WHERE session_id = '%s' AND rolled_back = 0 ORDER BY epoch LIMIT 10",
        sessionID))

    // Буфер
    buffer, _ := s.Exec.Execute(fmt.Sprintf(
        "SELECT key || ': ' || data FROM buffer_space WHERE session_id = '%s' AND rolled_back = 0",
        sessionID))

    // Выводы
    inferences, _ := s.Exec.Execute(fmt.Sprintf(
        "SELECT conclusion || ' (confidence: ' || confidence || ')' FROM inference_space WHERE session_id = '%s' AND rolled_back = 0",
        sessionID))

    writeJSON(w, map[string]string{
        "metaspace":    metaspace,
        "instructions": instructions,
        "thoughts":     thoughts,
        "buffer":       buffer,
        "inferences":   inferences,
    })
}

// POST /api/context/rollback — откат
func (s *Server) handleContextRollback(w http.ResponseWriter, r *http.Request) {
    var req struct {
        SessionID string `json:"session_id"`
        Steps     int    `json:"steps"`
    }
    json.NewDecoder(r.Body).Decode(&req)

    targetEpoch := fmt.Sprintf("(SELECT MAX(epoch) - %d FROM reasoning_space WHERE session_id = '%s')", req.Steps, req.SessionID)

    s.Exec.Execute(fmt.Sprintf(
        "UPDATE reasoning_space SET rolled_back = 1 WHERE session_id = '%s' AND epoch > %s",
        req.SessionID, targetEpoch))
    s.Exec.Execute(fmt.Sprintf(
        "UPDATE buffer_space SET rolled_back = 1 WHERE session_id = '%s' AND epoch > %s",
        req.SessionID, targetEpoch))
    s.Exec.Execute(fmt.Sprintf(
        "UPDATE tool_calls SET rolled_back = 1 WHERE session_id = '%s' AND epoch > %s",
        req.SessionID, targetEpoch))
    s.Exec.Execute(fmt.Sprintf(
        "UPDATE inference_space SET rolled_back = 1 WHERE session_id = '%s' AND epoch > %s",
        req.SessionID, targetEpoch))

    writeJSON(w, map[string]string{"status": "ok"})
}

// POST /api/context/gc — запуск GC
func (s *Server) handleContextGC(w http.ResponseWriter, r *http.Request) {
    var req struct {
        SessionID string `json:"session_id"`
        GCType    string `json:"gc_type"` // "minor", "major", "full"
    }
    json.NewDecoder(r.Body).Decode(&req)

    switch req.GCType {
    case "minor":
        // Очистка буфера по TTL
        s.Exec.Execute(fmt.Sprintf(
            "DELETE FROM buffer_space WHERE session_id = '%s' AND created_at < %d - ttl",
            req.SessionID, time.Now().Unix()))
        // Очистка тулов
        s.Exec.Execute(fmt.Sprintf(
            "DELETE FROM session_tools WHERE session_id = '%s' AND expires_at < %d",
            req.SessionID, time.Now().Unix()))

    case "major":
        // Сжатие старых мыслей
        s.Exec.Execute(fmt.Sprintf(
            "UPDATE reasoning_space SET content = '[compressed] ' || substr(content, 1, 100) WHERE session_id = '%s' AND epoch < (SELECT MAX(epoch) - 50 FROM reasoning_space WHERE session_id = '%s')",
            req.SessionID, req.SessionID))
        // Удаление буфера
        s.Exec.Execute(fmt.Sprintf(
            "DELETE FROM buffer_space WHERE session_id = '%s' AND rolled_back = 1",
            req.SessionID))

    case "full":
        // Удаление откаченных записей
        s.Exec.Execute(fmt.Sprintf("DELETE FROM buffer_space WHERE session_id = '%s' AND rolled_back = 1", req.SessionID))
        s.Exec.Execute(fmt.Sprintf("DELETE FROM reasoning_space WHERE session_id = '%s' AND rolled_back = 1", req.SessionID))
        s.Exec.Execute(fmt.Sprintf("DELETE FROM tool_calls WHERE session_id = '%s' AND rolled_back = 1", req.SessionID))
        s.Exec.Execute(fmt.Sprintf("DELETE FROM inference_space WHERE session_id = '%s' AND rolled_back = 1", req.SessionID))
    }

    writeJSON(w, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(data)
}


func (s *Server) initContextManager() {
    tables := []string{
        `CREATE TABLE IF NOT EXISTS metaspace (
            id INT PRIMARY KEY,
            agent_id TEXT NOT NULL,
            version INT NOT NULL,
            content_type TEXT NOT NULL,
            content TEXT NOT NULL,
            priority INT DEFAULT 0,
            is_active INT DEFAULT 1
        )`,
        `CREATE TABLE IF NOT EXISTS instruction_stack (
            id INT PRIMARY KEY,
            session_id INT NOT NULL,
            epoch INT NOT NULL,
            rolled_back INT DEFAULT 0,
            parent_id INT,
            depth INT DEFAULT 0,
            content TEXT NOT NULL
        )`,
        `CREATE TABLE IF NOT EXISTS reasoning_space (
            id INT PRIMARY KEY,
            session_id INT NOT NULL,
            epoch INT NOT NULL,
            rolled_back INT DEFAULT 0,
            parent_instruction_id INT,
            parent_thought_id INT,
            thought_type TEXT NOT NULL,
            content TEXT NOT NULL
        )`,
        `CREATE TABLE IF NOT EXISTS tool_registry (
            id INT PRIMARY KEY,
            agent_id TEXT NOT NULL,
            name TEXT NOT NULL,
            description TEXT,
            schema TEXT,
            default_ttl INT DEFAULT 300
        )`,
        `CREATE TABLE IF NOT EXISTS session_tools (
            id INT PRIMARY KEY,
            session_id INT NOT NULL,
            tool_id INT,
            loaded_at INT NOT NULL,
            expires_at INT
        )`,
        `CREATE TABLE IF NOT EXISTS tool_calls (
            id INT PRIMARY KEY,
            session_id INT NOT NULL,
            epoch INT NOT NULL,
            rolled_back INT DEFAULT 0,
            thought_id INT,
            tool_id INT,
            args TEXT,
            status TEXT DEFAULT 'pending'
        )`,
        `CREATE TABLE IF NOT EXISTS buffer_space (
            id INT PRIMARY KEY,
            session_id INT NOT NULL,
            epoch INT NOT NULL,
            rolled_back INT DEFAULT 0,
            tool_call_id INT,
            key TEXT,
            data TEXT,
            ttl INT DEFAULT 300,
            created_at INT NOT NULL
        )`,
        `CREATE TABLE IF NOT EXISTS inference_space (
            id INT PRIMARY KEY,
            session_id INT NOT NULL,
            epoch INT NOT NULL,
            rolled_back INT DEFAULT 0,
            conclusion TEXT NOT NULL,
            confidence REAL DEFAULT 0.5,
            inference_type TEXT DEFAULT 'assumption'
        )`,
        `CREATE TABLE IF NOT EXISTS inference_evidence (
            id INT PRIMARY KEY,
            inference_id INT,
            buffer_id INT,
            thought_id INT
        )`,
    }
    for _, t := range tables {
        s.Exec.Execute(t)
    }
    fmt.Println("✓ Контекст-менеджер инициализирован")
}