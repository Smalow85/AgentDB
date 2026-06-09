package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"agent-db/pkg/agent"
	"agent-db/pkg/config"
	contextmgr "agent-db/pkg/context"
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
	ctxMgr   *contextmgr.ContextManager
	config   *config.ConfigManager
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
		ctxMgr:   contextmgr.NewContextManager(exec),
		config:   config.NewConfigManager(exec),
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
	http.HandleFunc("/api/agent/loop", s.handleAgentLoop)
	http.HandleFunc("/api/agent/stream", s.handleAgentLoopStream)
	http.HandleFunc("/api/config/models", s.handleModels)
	http.HandleFunc("/api/config/models/add", s.handleAddModel)
	http.HandleFunc("/api/config/models/active", s.handleSetActiveModel)
	http.HandleFunc("/api/config/projects", s.handleProjects)
	http.HandleFunc("/api/config/projects/add", s.handleAddProject)
	http.HandleFunc("/api/config/projects/active", s.handleSetActiveProject)
	http.HandleFunc("/api/config/settings", s.handleSettings)

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
		Type  string `json:"type"` // "class", "method", "callers", "callees"
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
				"class":   req.Name,
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

	// Получаем все узлы из графа
	allNodes := s.PSIGraph.FindNodes(graph.Query{})

	// Карта для уникальных рёбер
	seenEdges := make(map[string]bool)

	for _, node := range allNodes {
		// Определяем тип узла по его меткам
		nodeType := "unknown"
		if node.HasLabel("file") {
			nodeType = "file"
		} else if node.HasLabel("class") {
			nodeType = "class"
		} else if node.HasLabel("function") {
			nodeType = "function"
		} else if node.HasLabel("method") {
			nodeType = "function"
		} else if node.HasLabel("call") {
			nodeType = "call"
		}

		// Получаем имя узла
		label := ""
		if name, ok := node.GetProp("name"); ok {
			label = fmt.Sprintf("%v", name)
		} else if path, ok := node.GetProp("path"); ok {
			label = fmt.Sprintf("%v", path)
		} else {
			label = fmt.Sprintf("node_%d", node.ID)
		}

		result.Nodes = append(result.Nodes, GraphNode{
			ID:    node.ID,
			Label: label,
			Type:  nodeType,
			Props: node.Properties,
		})

		// Собираем рёбра (исходящие связи)
		edges := s.PSIGraph.GetEdges(node.ID, graph.DirectionOutgoing)
		for _, edge := range edges {
			key := fmt.Sprintf("%d->%d", edge.FromID, edge.ToID)
			if !seenEdges[key] {
				result.Edges = append(result.Edges, GraphEdge{
					From: edge.FromID,
					To:   edge.ToID,
					Type: edge.Type,
				})
				seenEdges[key] = true
			}
		}

		// Собираем ссылки (calls)
		refs := s.PSIGraph.GetReferences(node.ID, graph.DirectionOutgoing)
		for _, ref := range refs {
			if ref.IsResolved && ref.Type == "call" {
				key := fmt.Sprintf("%d->%d", ref.SourceID, ref.TargetID)
				if !seenEdges[key] {
					result.Edges = append(result.Edges, GraphEdge{
						From: ref.SourceID,
						To:   ref.TargetID,
						Type: "call",
					})
					seenEdges[key] = true
				}
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
		SessionID int    `json:"session_id"`
		Message   string `json:"message"`
		LLMKey    string `json:"llm_key"`
		Model     string `json:"model"`
		BaseURL   string `json:"base_url"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]string{"error": "неверный JSON"})
		return
	}

	if req.BaseURL == "" || req.LLMKey == "" {
		writeJSON(w, map[string]string{"error": "нужен base_url и llm_key"})
		return
	}
	if req.SessionID == 0 {
		req.SessionID = 1
	}

	// Сохраняем инструкцию пользователя через контекст-менеджер
	s.ctxMgr.PushInstruction(req.SessionID, req.Message, 0)
	s.ctxMgr.AddThought(req.SessionID, "user_input", req.Message, 0)

	// Создаём агента с контекст-менеджером
	a := &agent.AgentLoop{
		SessionID: fmt.Sprintf("%d", req.SessionID),
		PSIGraph:  s.PSIGraph,
		LLMKey:    req.LLMKey,
		Model:     req.Model,
		BaseURL:   req.BaseURL,
	}

	// Запускаем агента (без контекст-менеджера внутри, он будет использовать внешний)
	result, messages, err := a.Run(req.Message)

	// Сохраняем результат
	for _, msg := range messages {
		if msg.Role == "assistant" {
			s.ctxMgr.AddThought(req.SessionID, "assistant_response", msg.Content, 0)
		}
		if msg.Role == "tool" {
			s.ctxMgr.AddToBuffer(req.SessionID, "tool_result", msg.Content, 300)
		}
	}

	if err != nil {
		writeJSON(w, map[string]string{"error": err.Error()})
		return
	}

	// Делаем вывод на основе ответа
	s.ctxMgr.AddInference(req.SessionID, result, 0.85, "fact")

	writeJSON(w, map[string]string{"result": result})
}

func (s *Server) saveToReasoning(sessionID, thoughtType, content string) {
	// Обрежем слишком длинный контент
	if len(content) > 500 {
		content = content[:500]
	}

	sql := fmt.Sprintf(
		"INSERT INTO reasoning_space (session_id, epoch, thought_type, content) VALUES ('%s', 1, '%s', '%s')",
		sessionID, thoughtType, escapeSQL(content),
	)
	fmt.Printf("[DEBUG] SQL: %s\n", sql)

	result, err := s.Exec.Execute(sql)
	if err != nil {
		fmt.Printf("[ERROR] saveToReasoning: %v\n", err)
	} else {
		fmt.Printf("[DEBUG] Saved: %s\n", result)
	}
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
		"INSERT INTO metaspace VALUES ('%s', 1, '%s', '%s', %d)",
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
		SessionID       string `json:"session_id"`
		Content         string `json:"content"`
		ThoughtType     string `json:"thought_type"`
		ParentThoughtID int    `json:"parent_thought_id"`
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

func escapeSQL(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "'", "''"), "\n", " ")
}

func (s *Server) initContextManager() {
	tables := []string{
		`CREATE TABLE IF NOT EXISTS sessions (
            id INT PRIMARY KEY,
            current_epoch INT DEFAULT 0,
            created_at INT NOT NULL
        )`,
		`CREATE TABLE IF NOT EXISTS metaspace (
            id INT PRIMARY KEY AUTOINCREMENT,
            agent_id TEXT NOT NULL,
            version INT NOT NULL,
            content_type TEXT NOT NULL,
            content TEXT NOT NULL,
            priority INT DEFAULT 0,
            is_active INT DEFAULT 1
        )`,
		`CREATE TABLE IF NOT EXISTS instruction_stack (
            id INT PRIMARY KEY AUTOINCREMENT,
            session_id INT NOT NULL,
            epoch INT NOT NULL,
            rolled_back INT DEFAULT 0,
            parent_id INT,
            depth INT DEFAULT 0,
            content TEXT NOT NULL
        )`,
		`CREATE TABLE IF NOT EXISTS reasoning_space (
            id INT PRIMARY KEY AUTOINCREMENT,
            session_id INT NOT NULL,
            epoch INT NOT NULL,
            rolled_back INT DEFAULT 0,
            parent_instruction_id INT,
            parent_thought_id INT,
            thought_type TEXT NOT NULL,
            content TEXT NOT NULL
        )`,
		`CREATE TABLE IF NOT EXISTS tool_registry (
            id INT PRIMARY KEY AUTOINCREMENT,
            agent_id TEXT NOT NULL,
            name TEXT NOT NULL,
            description TEXT,
            schema TEXT,
            default_ttl INT DEFAULT 300
        )`,
		`CREATE TABLE IF NOT EXISTS session_tools (
            id INT PRIMARY KEY AUTOINCREMENT,
            session_id INT NOT NULL,
            tool_id INT,
            loaded_at INT NOT NULL,
            expires_at INT
        )`,
		`CREATE TABLE IF NOT EXISTS tool_calls (
            id INT PRIMARY KEY AUTOINCREMENT,
            session_id INT NOT NULL,
            epoch INT NOT NULL,
            rolled_back INT DEFAULT 0,
            thought_id INT,
            tool_id INT,
            args TEXT,
            status TEXT DEFAULT 'pending'
        )`,
		`CREATE TABLE IF NOT EXISTS buffer_space (
            id INT PRIMARY KEY AUTOINCREMENT,
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
            id INT PRIMARY KEY AUTOINCREMENT,
            session_id INT NOT NULL,
            epoch INT NOT NULL,
            rolled_back INT DEFAULT 0,
            conclusion TEXT NOT NULL,
            confidence REAL DEFAULT 0.5,
            inference_type TEXT DEFAULT 'assumption'
        )`,
		`CREATE TABLE IF NOT EXISTS inference_evidence (
            id INT PRIMARY KEY AUTOINCREMENT,
            inference_id INT,
            buffer_id INT,
            thought_id INT
        )`,
		`CREATE TABLE IF NOT EXISTS _sequences (
            table_name TEXT PRIMARY KEY,
            col_name TEXT, 
            next_val INT
        )`,
	}
	for _, t := range tables {
		s.Exec.Execute(t)
	}
	fmt.Println("✓ Контекст-менеджер инициализирован")
}

func (s *Server) handleContextGC(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID string `json:"session_id"`
		GCType    string `json:"gc_type"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	switch req.GCType {
	case "minor":
		s.Exec.Execute(fmt.Sprintf(
			"DELETE FROM buffer_space WHERE session_id = '%s' AND created_at < %d - ttl",
			req.SessionID, time.Now().Unix()))
		s.Exec.Execute(fmt.Sprintf(
			"DELETE FROM session_tools WHERE session_id = '%s' AND expires_at < %d",
			req.SessionID, time.Now().Unix()))
	case "major":
		s.Exec.Execute(fmt.Sprintf(
			"UPDATE reasoning_space SET content = '[compressed] ' || substr(content, 1, 100) WHERE session_id = '%s' AND epoch < (SELECT MAX(epoch) - 50 FROM reasoning_space WHERE session_id = '%s')",
			req.SessionID, req.SessionID))
		s.Exec.Execute(fmt.Sprintf(
			"DELETE FROM buffer_space WHERE session_id = '%s' AND rolled_back = 1",
			req.SessionID))
	case "full":
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

func (s *Server) buildContextSummary(sessionID int) map[string]string {
	sid := fmt.Sprintf("%d", sessionID)
	result := make(map[string]string)

	// Metaspace
	metaspace, _ := s.Exec.Execute("SELECT content FROM metaspace WHERE is_active = 1 ORDER BY priority DESC LIMIT 3")
	result["metaspace"] = metaspace

	// Инструкции
	instructions, _ := s.Exec.Execute(fmt.Sprintf(
		"SELECT content FROM instruction_stack WHERE session_id = '%s' AND rolled_back = 0 ORDER BY depth LIMIT 5", sid))
	result["instructions"] = instructions

	// Мысли
	thoughts, _ := s.Exec.Execute(fmt.Sprintf(
		"SELECT thought_type || ': ' || content FROM reasoning_space WHERE session_id = '%s' AND rolled_back = 0 ORDER BY epoch LIMIT 5", sid))
	result["thoughts"] = thoughts

	// Буфер
	buffer, _ := s.Exec.Execute(fmt.Sprintf(
		"SELECT key || ': ' || data FROM buffer_space WHERE session_id = '%s' AND rolled_back = 0 LIMIT 5", sid))
	result["buffer"] = buffer

	return result
}

func (s *Server) saveInstruction(sessionID int, content string) {
	if len(content) > 500 {
		content = content[:500]
	}
	s.Exec.Execute(fmt.Sprintf(
		"INSERT INTO instruction_stack (session_id, epoch, rolled_back, parent_id, depth, content) VALUES ('%d', 1, 0, 0, 0, '%s')",
		sessionID, escapeSQL(content),
	))
}

func (s *Server) handleAgentLoopStream(w http.ResponseWriter, r *http.Request) {
	log.Println("[DEBUG] === handleAgentLoopStream called ===")
	log.Printf("[DEBUG] Method: %s", r.Method)
	log.Printf("[DEBUG] URL: %s", r.URL.String())

	// Настраиваем SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		log.Println("[ERROR] Streaming unsupported - no flusher")
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}
	log.Println("[DEBUG] SSE headers set, flusher OK")

	// Парсим запрос
	var req struct {
		SessionID int    `json:"session_id"`
		Message   string `json:"message"`
		LLMKey    string `json:"llm_key"`
		Model     string `json:"model"`
		BaseURL   string `json:"base_url"`
	}

	// Поддерживаем и GET, и POST
	if r.Method == "POST" {
		log.Println("[DEBUG] Parsing POST JSON body")
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			log.Printf("[ERROR] Failed to parse JSON: %v", err)
			s.sendSSEEvent(w, flusher, "error", fmt.Sprintf("Неверный JSON: %v", err), "")
			return
		}
		log.Printf("[DEBUG] POST parsed: SessionID=%d, MessageLen=%d, Model=%s, BaseURL=%s, HasLLMKey=%v",
			req.SessionID, len(req.Message), req.Model, req.BaseURL, req.LLMKey != "")
	} else {
		log.Println("[DEBUG] Parsing GET query parameters")
		req.SessionID = parseIntSafe(r.URL.Query().Get("session_id"))
		req.Message = r.URL.Query().Get("message")
		req.LLMKey = r.URL.Query().Get("llm_key")
		req.Model = r.URL.Query().Get("model")
		req.BaseURL = r.URL.Query().Get("base_url")
		log.Printf("[DEBUG] GET parsed: SessionID=%d, MessageLen=%d, Model=%s, BaseURL=%s, HasLLMKey=%v",
			req.SessionID, len(req.Message), req.Model, req.BaseURL, req.LLMKey != "")
	}

	if req.SessionID == 0 {
		log.Println("[DEBUG] SessionID was 0, setting to 1")
		req.SessionID = 1
	}

	// ПРОВЕРКА: что пришло от клиента
	log.Println("[DEBUG] === FINAL REQUEST VALUES ===")
	log.Printf("[DEBUG] SessionID: %d", req.SessionID)
	log.Printf("[DEBUG] Message: %s", truncate(req.Message, 100))
	log.Printf("[DEBUG] Model: %s", req.Model)
	log.Printf("[DEBUG] BaseURL: %s", req.BaseURL)
	log.Printf("[DEBUG] LLMKey (first 20 chars): %s", truncate(req.LLMKey, 20))

	if req.BaseURL == "" {
		log.Println("[ERROR] BaseURL is empty!")
		s.sendSSEEvent(w, flusher, "error", "base_url is required", "")
		return
	}

	if req.LLMKey == "" {
		log.Println("[WARN] LLMKey is empty! Some providers may require it")
		// Не возвращаем ошибку, просто предупреждаем
		s.sendSSEEvent(w, flusher, "warning", "LLM Key не указан, продолжение может не работать", "")
	}

	if req.Model == "" {
		log.Println("[WARN] Model is empty, using default")
		req.Model = "gpt-3.5-turbo"
	}

	// Проверяем, инициализирован ли контекст-менеджер
	if s.ctxMgr == nil {
		log.Println("[ERROR] ContextManager is nil!")
		s.sendSSEEvent(w, flusher, "error", "Контекст-менеджер не инициализирован", "")
		return
	}
	log.Println("[DEBUG] ContextManager OK")

	// Проверяем PSIGraph
	if s.PSIGraph == nil {
		log.Println("[WARN] PSIGraph is nil, continuing anyway")
	} else {
		log.Println("[DEBUG] PSIGraph OK")
	}

	log.Println("[DEBUG] Saving instruction for session", req.SessionID)
	if err := s.ctxMgr.PushInstruction(req.SessionID, req.Message, 0); err != nil {
		log.Printf("[ERROR] Failed to push instruction: %v", err)
		s.sendSSEEvent(w, flusher, "error", fmt.Sprintf("Ошибка сохранения инструкции: %v", err), "")
		return
	}
	log.Println("[DEBUG] PushInstruction completed successfully")

	if err := s.ctxMgr.AddThought(req.SessionID, "user_input", req.Message, 0); err != nil {
		log.Printf("[WARN] Failed to add thought: %v", err)
	}
	log.Println("[DEBUG] AddThought completed successfully") // ← добавить

	// Создаём агента
	log.Println("[DEBUG] Creating AgentLoop...") // ← добавить
	a := &agent.AgentLoop{
		SessionID:  fmt.Sprintf("%d", req.SessionID),
		PSIGraph:   s.PSIGraph,
		LLMKey:     req.LLMKey,
		Model:      req.Model,
		BaseURL:    req.BaseURL,
		ContextMgr: s.ctxMgr,
	}
	log.Printf("[DEBUG] AgentLoop created: SessionID=%s", a.SessionID) // ← добавить

	// Отправляем стартовое событие
	log.Println("[DEBUG] Sending start event...") // ← добавить
	s.sendSSEEvent(w, flusher, "start", "Начинаю обработку...", "")
	log.Println("[DEBUG] Start event sent") // ← добавить

	// Запускаем стриминг
	log.Println("[DEBUG] Calling a.RunStream...") // ← добавить
	err := a.RunStream(req.Message, func(event agent.StreamEvent) {
		log.Printf("[DEBUG] Stream event received: type=%s, content_len=%d", event.Type, len(event.Content))
		s.sendSSEEvent(w, flusher, event.Type, event.Content, event.Tool)
	})
	log.Println("[DEBUG] a.RunStream returned") // ← добавить

	if err != nil {
		log.Printf("[ERROR] RunStream failed: %v", err)
		s.sendSSEEvent(w, flusher, "error", err.Error(), "")
	} else {
		log.Println("[DEBUG] RunStream completed successfully")
		s.sendSSEEvent(w, flusher, "done", "", "")
	}

	log.Println("[DEBUG] === handleAgentLoopStream finished ===")
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// sendSSEEvent — отправка SSE события
func (s *Server) sendSSEEvent(w http.ResponseWriter, flusher http.Flusher, eventType, content, tool string) {
	data := map[string]string{
		"type":    eventType,
		"content": content,
	}
	if tool != "" {
		data["tool"] = tool
	}

	jsonData, _ := json.Marshal(data)
	fmt.Fprintf(w, "data: %s\n\n", jsonData)
	flusher.Flush()
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	result, err := s.Exec.Execute(`
        SELECT id, name, display_name, base_url, api_key, is_default
        FROM model_configs
        ORDER BY is_default DESC, name ASC
    `)

	if err != nil {
		writeJSON(w, map[string]string{"error": err.Error()})
		return
	}

	var models []map[string]interface{}

	if result != "" && !strings.Contains(result, "0 rows") {
		lines := strings.Split(result, "\n")

		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "id|") || strings.HasPrefix(line, "---") {
				continue
			}

			parts := strings.Split(line, "|")
			if len(parts) >= 6 {
				id, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
				isDefault := strings.TrimSpace(parts[5]) == "1"

				models = append(models, map[string]interface{}{
					"id":           id,
					"name":         strings.TrimSpace(parts[1]),
					"display_name": strings.TrimSpace(parts[2]),
					"base_url":     strings.TrimSpace(parts[3]),
					"api_key":      strings.TrimSpace(parts[4]),
					"is_default":   isDefault,
				})
			}
		}
	}

	writeJSON(w, map[string]interface{}{"models": models})
}

func (s *Server) handleAddModel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
		BaseURL     string `json:"base_url"`
		APIKey      string `json:"api_key"`
		IsDefault   bool   `json:"is_default"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[ERROR] Failed to parse add model request: %v", err)
		writeJSON(w, map[string]string{"error": "неверный JSON"})
		return
	}

	log.Printf("[DEBUG] AddModel request: name=%s, baseURL=%s, isDefault=%v",
		req.Name, req.BaseURL, req.IsDefault)

	if req.Name == "" || req.BaseURL == "" {
		writeJSON(w, map[string]string{"error": "name и base_url обязательны"})
		return
	}

	// Добавляем модель через config менеджер
	model, err := s.config.AddModel(req.Name, req.DisplayName, req.BaseURL, req.APIKey, req.IsDefault)
	if err != nil {
		log.Printf("[ERROR] AddModel failed: %v", err)
		writeJSON(w, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, map[string]interface{}{"success": true, "model": model})
}

// POST /api/config/models/active — установить активную модель
func (s *Server) handleSetActiveModel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ModelID int `json:"model_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]string{"error": "неверный JSON"})
		return
	}

	if err := s.config.SetActiveModel(req.ModelID); err != nil {
		writeJSON(w, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, map[string]interface{}{"success": true})
}

// GET /api/config/projects — список проектов
// pkg/server/server.go — исправленный handleProjects

// GET /api/config/projects — список проектов
func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request) {
	if s.config == nil {
		writeJSON(w, map[string]string{"error": "config manager not initialized"})
		return
	}

	// Получаем проекты из БД
	result, err := s.Exec.Execute(`
        SELECT id, name, root_path, description, is_active 
        FROM project_configs 
        ORDER BY id DESC
    `)

	if err != nil {
		writeJSON(w, map[string]string{"error": err.Error()})
		return
	}

	fmt.Printf("[DEBUG] SQL Result: '%s'\n", result) // Отладка

	var projects []map[string]interface{}

	// Проверяем, есть ли данные
	if result != "" && !strings.Contains(result, "0 rows") {
		// Разбиваем на строки
		lines := strings.Split(result, "\n")

		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			// Пропускаем заголовок
			if strings.HasPrefix(line, "id|") || strings.HasPrefix(line, "---") {
				continue
			}

			// Пропускаем рамки
			if !strings.HasPrefix(line, "│") {
				continue
			}

			// Убираем рамку
			line = strings.TrimPrefix(line, "│")
			line = strings.TrimSuffix(line, "│")
			line = strings.TrimSpace(line)

			// Парсим строку
			parts := strings.Split(line, "|")
			if len(parts) >= 5 {
				// Преобразуем id в int
				id, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
				isActive := strings.TrimSpace(parts[4]) == "1"

				project := map[string]interface{}{
					"id":          id,
					"name":        strings.TrimSpace(parts[1]),
					"root_path":   strings.TrimSpace(parts[2]),
					"description": strings.TrimSpace(parts[3]),
					"is_active":   isActive,
				}
				projects = append(projects, project)

				fmt.Printf("[DEBUG] Parsed project: %+v\n", project)
			}
		}
	}

	// Всегда возвращаем массив, даже пустой
	if projects == nil {
		projects = []map[string]interface{}{}
	}

	writeJSON(w, map[string]interface{}{
		"projects": projects,
		"count":    len(projects),
	})
}

// POST /api/config/projects/add — добавить проект
func (s *Server) handleAddProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		RootPath    string `json:"root_path"`
		Description string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]string{"error": "неверный JSON"})
		return
	}

	project, err := s.config.AddProject(req.Name, req.RootPath, req.Description)
	if err != nil {
		writeJSON(w, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, map[string]interface{}{"success": true, "project": project})
}

// POST /api/config/projects/active — установить активный проект
func (s *Server) handleSetActiveProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProjectID int `json:"project_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]string{"error": "неверный JSON"})
		return
	}

	if err := s.config.SetActiveProject(req.ProjectID); err != nil {
		writeJSON(w, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, map[string]interface{}{"success": true})
}

// GET/POST /api/config/settings — настройки пользователя
func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		settings, err := s.config.GetSettings()
		if err != nil {
			writeJSON(w, map[string]string{"error": err.Error()})
			return
		}
		fmt.Printf("[handleSettings] GetSettings returned: %+v\n", settings)
		writeJSON(w, map[string]interface{}{"settings": settings})
		return
	}

	if r.Method == "POST" {
		var req struct {
			StreamingEnabled *bool  `json:"streaming_enabled"`
			Theme            string `json:"theme"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]string{"error": "неверный JSON"})
			return
		}

		if req.StreamingEnabled != nil {
			s.config.SetStreamingEnabled(*req.StreamingEnabled)
		}

		writeJSON(w, map[string]interface{}{"success": true})
		return
	}
}

func (s *Server) saveToolCall(sessionID int, toolName, toolResult string) {
	if len(toolName) > 100 {
		toolName = toolName[:100]
	}
	if len(toolResult) > 500 {
		toolResult = toolResult[:500]
	}

	// Сохраняем вызов
	s.Exec.Execute(fmt.Sprintf(
		"INSERT INTO tool_calls (session_id, epoch, rolled_back, thought_id, tool_id, args, status) VALUES ('%d', 1, 0, 0, 0, '', 'success')",
		sessionID,
	))

	// Сохраняем результат в буфер
	now := time.Now().Unix()
	s.Exec.Execute(fmt.Sprintf(
		"INSERT INTO buffer_space (session_id, epoch, rolled_back, tool_call_id, key, data, ttl, created_at) VALUES ('%d', 1, 0, 0, '%s', '%s', 300, %d)",
		sessionID, toolName, escapeSQL(toolResult), now,
	))
}

func (s *Server) saveReasoning(sessionID int, thoughtType, content string) {
	if len(content) > 500 {
		content = content[:500]
	}
	s.Exec.Execute(fmt.Sprintf(
		"INSERT INTO reasoning_space (session_id, epoch, rolled_back, parent_instruction_id, parent_thought_id, thought_type, content) VALUES ('%d', 1, 0, 0, 0, '%s', '%s')",
		sessionID, thoughtType, escapeSQL(content),
	))
}

func parseIntSafe(s string) int {
	var i int
	fmt.Sscanf(s, "%d", &i)
	return i
}
