package server

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"agent-db/pkg/agent"
	"agent-db/pkg/config"
	"agent-db/pkg/context"
	"agent-db/pkg/executor"
	"agent-db/pkg/graph"
	"agent-db/pkg/psi"
	"agent-db/pkg/storage"
)

//go:embed static
var staticFiles embed.FS

type Server struct {
	Exec      *executor.Executor
	PSIGraph  *graph.Graph
	PSIDisk   *storage.DiskManager
	parser    *psi.PSIParser
	memoryMgr *context.MemoryManager // JVM-style менеджер памяти
	config    *config.ConfigManager
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

	// Создаём MemoryManager
	memoryMgr := context.NewMemoryManager(exec)

	s := &Server{
		Exec:      exec,
		PSIGraph:  g,
		PSIDisk:   psiDisk,
		parser:    psi.NewPSIParser(g),
		memoryMgr: memoryMgr,
		config:    config.NewConfigManager(exec),
	}
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
	http.HandleFunc("/api/agent/loop", s.handleAgentLoop)
	http.HandleFunc("/api/agent/stream", s.handleAgentLoopStream)
	http.HandleFunc("/api/config/models", s.handleModels)
	http.HandleFunc("/api/config/models/add", s.handleAddModel)
	http.HandleFunc("/api/config/models/active", s.handleSetActiveModel)
	http.HandleFunc("/api/config/projects", s.handleProjects)
	http.HandleFunc("/api/config/projects/add", s.handleAddProject)
	http.HandleFunc("/api/config/projects/active", s.handleSetActiveProject)
	http.HandleFunc("/api/config/settings", s.handleSettings)
	http.HandleFunc("/api/context/rollback", s.handleContextRollback)
	http.HandleFunc("/api/context/gc", s.handleContextGC)
	http.HandleFunc("/api/code", s.handleCode)

	staticFS, _ := fs.Sub(staticFiles, "static")
	http.Handle("/", http.FileServer(http.FS(staticFS)))

	fmt.Printf("🌐 AgentDB Web UI: http://localhost%s\n", addr)
	return http.ListenAndServe(addr, nil)
}

// ========== SQL API ==========

type QueryRequest struct {
	SQL string `json:"sql"`
}

func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}

	var req QueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]string{"error": "неверный формат запроса"})
		return
	}

	result, err := s.Exec.Execute(req.SQL)
	if err != nil {
		writeJSON(w, &executor.QueryResult{
			Type:  "ERROR",
			Error: err.Error(),
		})
		return
	}

	writeJSON(w, result)
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
	writeJSON(w, result)
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

	absPath, err := filepath.Abs(req.Path)
	if err != nil {
		writeJSON(w, map[string]string{"error": "неверный путь: " + err.Error()})
		return
	}

	// Создаём новый граф
	psiBP := storage.NewBufferPool(100, s.PSIDisk)
	psiStore := graph.NewGraphStore(psiBP, s.PSIDisk)
	s.PSIGraph = graph.NewGraph("psigraph", psiStore)
	s.parser = psi.NewPSIParser(s.PSIGraph)

	startTime := time.Now()
	err = s.parser.ParseRepo(absPath)
	elapsed := time.Since(startTime)

	if err != nil {
		writeJSON(w, map[string]string{"error": err.Error()})
		return
	}

	files := s.PSIGraph.FindNodes(graph.Query{Label: "file"})
	classes := s.PSIGraph.FindNodes(graph.Query{Label: "class"})
	functions := s.PSIGraph.FindNodes(graph.Query{Label: "function"})
	calls := s.PSIGraph.FindNodes(graph.Query{Label: "call"})

	fmt.Printf("[DEBUG] После парсинга: files=%d classes=%d functions=%d calls=%d\n",
		len(files), len(classes), len(functions), len(calls))

	if err := s.PSIGraph.SaveToDisk(); err != nil {
		fmt.Printf("[ERROR] Ошибка сохранения графа: %v\n", err)
	} else {
		fmt.Println("[DEBUG] Граф сохранён успешно")
	}

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
		Type  string `json:"type"`
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
	g := s.PSIGraph
	if g == nil {
		writeJSON(w, map[string]string{"error": "граф не инициализирован"})
		return
	}

	// Узлы
	nodes := make([]map[string]interface{}, 0)
	for _, node := range g.GetAllNodes() { // используем метод
		label := fmt.Sprintf("%v", node.Properties["name"])
		if label == "<nil>" {
			label = fmt.Sprintf("node_%d", node.ID)
		}
		typ := ""
		if len(node.Labels) > 0 {
			typ = node.Labels[0]
		}
		nodes = append(nodes, map[string]interface{}{
			"id":    node.ID,
			"label": label,
			"type":  typ,
			"props": node.Properties,
		})
	}

	// Рёбра
	edges := make([]map[string]interface{}, 0)
	for _, edge := range g.GetAllEdges() {
		edges = append(edges, map[string]interface{}{
			"from": edge.FromID,
			"to":   edge.ToID,
			"type": edge.Type,
		})
	}

	writeJSON(w, map[string]interface{}{
		"nodes": nodes,
		"edges": edges,
	})
}

func (s *Server) handleGraphList(w http.ResponseWriter, r *http.Request) {
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

		children := s.PSIGraph.GetNeighbors(class.ID, graph.DirectionOutgoing)
		for _, child := range children {
			if child.HasLabel("function") {
				methodName, _ := child.GetProp("name")
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

	sb.WriteString(fmt.Sprintf("\n📊 Статистика: %d классов, %d функций, %d вызовов\n",
		len(classes), len(functions), len(calls)))

	writeJSON(w, map[string]string{"context": sb.String()})
}

func (s *Server) handleAgentLoop(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID   int    `json:"session_id"`
		Message     string `json:"message"`
		LLMKey      string `json:"llm_key"`
		Model       string `json:"model"`
		BaseURL     string `json:"base_url"`
		ProjectPath string `json:"project_path"`
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

	// Получаем текущую версию (пока 1, позже из snapshot менеджера)
	currentVersion := 1

	a := &agent.AgentLoop{
		SessionID:      fmt.Sprintf("%d", req.SessionID),
		PSIGraph:       s.PSIGraph,
		LLMKey:         req.LLMKey,
		Model:          req.Model,
		BaseURL:        req.BaseURL,
		MemoryMgr:      s.memoryMgr,
		ProjectPath:    req.ProjectPath,
		CurrentVersion: currentVersion,
	}

	result, messages, err := a.Run(req.Message)

	if err != nil {
		writeJSON(w, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, map[string]interface{}{
		"result":   result,
		"messages": messages,
	})
}

// ========== Agent Stream ==========

func (s *Server) handleAgentLoopStream(w http.ResponseWriter, r *http.Request) {
	log.Println("[DEBUG] === handleAgentLoopStream called ===")

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

	// Парсим запрос
	var req struct {
		SessionID   int    `json:"session_id"`
		Message     string `json:"message"`
		LLMKey      string `json:"llm_key"`
		Model       string `json:"model"`
		BaseURL     string `json:"base_url"`
		ProjectPath string `json:"project_path"`
	}

	if r.Method == "POST" {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			s.sendSSEEvent(w, flusher, "error", fmt.Sprintf("Ошибка чтения: %v", err), "")
			return
		}
		if err := json.Unmarshal(body, &req); err != nil {
			s.sendSSEEvent(w, flusher, "error", fmt.Sprintf("Неверный JSON: %v", err), "")
			return
		}
	} else {
		req.SessionID = parseIntSafe(r.URL.Query().Get("session_id"))
		req.Message = r.URL.Query().Get("message")
		req.LLMKey = r.URL.Query().Get("llm_key")
		req.Model = r.URL.Query().Get("model")
		req.BaseURL = r.URL.Query().Get("base_url")
		req.ProjectPath = r.URL.Query().Get("project_path")
	}

	if req.SessionID == 0 {
		req.SessionID = 1
	}

	if req.BaseURL == "" {
		s.sendSSEEvent(w, flusher, "error", "base_url is required", "")
		return
	}

	if req.Model == "" {
		req.Model = "gpt-3.5-turbo"
	}

	// ✅ Создаём сессию и сохраняем инструкцию через MemoryManager
	currentVersion := 1

	// 1. Проверяем/создаём сессию (через MemoryManager, у него есть exec)
	_, err := s.memoryMgr.GetOrCreateSession(req.SessionID)
	if err != nil {
		s.sendSSEEvent(w, flusher, "error", fmt.Sprintf("Ошибка сессии: %v", err), "")
		return
	}

	// 2. Сохраняем инструкцию
	instructionID, err := s.memoryMgr.PushInstruction(req.SessionID, currentVersion, req.Message, 0)
	if err != nil {
		s.sendSSEEvent(w, flusher, "error", fmt.Sprintf("Ошибка сохранения инструкции: %v", err), "")
		return
	}

	// 3. Сохраняем мысль
	s.memoryMgr.AddThought(req.SessionID, currentVersion, instructionID, "user_input", req.Message, 0.9)

	log.Printf("[DEBUG] Session %d, Instruction %d saved, version %d", req.SessionID, instructionID, currentVersion)

	// Создаём агента с новым MemoryManager
	a := &agent.AgentLoop{
		SessionID:      fmt.Sprintf("%d", req.SessionID),
		PSIGraph:       s.PSIGraph,
		LLMKey:         req.LLMKey,
		Model:          req.Model,
		BaseURL:        req.BaseURL,
		MemoryMgr:      s.memoryMgr,
		ProjectPath:    req.ProjectPath,
		CurrentVersion: currentVersion,
	}

	log.Printf("[DEBUG] Agent created: SessionID=%s, ProjectPath=%s, CurrentVersion=%d",
		a.SessionID, a.ProjectPath, a.CurrentVersion)

	// Отправляем стартовое событие
	s.sendSSEEvent(w, flusher, "start", "Начинаю обработку...", "")
	flusher.Flush()

	// Запускаем стриминг
	err = a.RunStream(req.Message, func(event agent.StreamEvent) {
		s.sendSSEEvent(w, flusher, event.Type, event.Content, event.Tool)
		flusher.Flush()
	})

	if err != nil {
		log.Printf("[ERROR] RunStream failed: %v", err)
		s.sendSSEEvent(w, flusher, "error", err.Error(), "")
	} else {
		s.sendSSEEvent(w, flusher, "done", "", "")
	}
}

// ========== Config API ==========

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	result, err := s.Exec.Execute(`
		SELECT name, display_name, base_url, api_key, is_default, created_at, updated_at
		FROM model_configs
		ORDER BY is_default DESC, name ASC
	`)

	if err != nil {
		writeJSON(w, map[string]string{"error": err.Error()})
		return
	}

	var models []map[string]interface{}

	if result.Type == "SELECT" && len(result.Rows) > 0 {
		for _, row := range result.Rows {
			if len(row) >= 6 {

				isDefault := false
				if len(row) > 5 && row[5] != nil {
					switch v := row[5].(type) {
					case int64:
						isDefault = v == 1
					case int:
						isDefault = v == 1
					case float64:
						isDefault = v == 1
					case bool:
						isDefault = v
					}
				}

				models = append(models, map[string]interface{}{
					"name":         row[0],
					"display_name": row[1],
					"base_url":     row[2],
					"api_key":      row[3],
					"is_default":   isDefault,
				})
			}
		}
	}

	writeJSON(w, map[string]interface{}{
		"models": models,
	})
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
		writeJSON(w, &executor.QueryResult{
			Type:  "ERROR",
			Error: "неверный JSON",
		})
		return
	}

	log.Printf("[DEBUG] AddModel request: name=%s, baseURL=%s, isDefault=%v",
		req.Name, req.BaseURL, req.IsDefault)

	if req.Name == "" || req.BaseURL == "" {
		writeJSON(w, &executor.QueryResult{
			Type:  "ERROR",
			Error: "name и base_url обязательны",
		})
		return
	}

	model, err := s.config.AddModel(req.Name, req.DisplayName, req.BaseURL, req.APIKey, req.IsDefault)
	if err != nil {
		writeJSON(w, &executor.QueryResult{
			Type:  "ERROR",
			Error: err.Error(),
		})
		return
	}

	writeJSON(w, map[string]interface{}{
		"success": true,
		"model":   model,
	})
}

func (s *Server) handleSetActiveModel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ModelID int `json:"model_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, &executor.QueryResult{
			Type:  "ERROR",
			Error: "неверный JSON",
		})
		return
	}

	log.Printf("[SetActiveModel] Received model_id: %d", req.ModelID)

	if req.ModelID <= 0 {
		writeJSON(w, &executor.QueryResult{
			Type:  "ERROR",
			Error: "неверный ID модели",
		})
		return
	}

	if err := s.config.SetActiveModel(req.ModelID); err != nil {
		writeJSON(w, &executor.QueryResult{
			Type:  "ERROR",
			Error: err.Error(),
		})
		return
	}

	writeJSON(w, map[string]interface{}{
		"success": true,
	})
}

func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request) {
	if s.config == nil {
		writeJSON(w, &executor.QueryResult{
			Type:  "ERROR",
			Error: "config manager not initialized",
		})
		return
	}

	result, err := s.Exec.Execute(`
		SELECT name, root_path, description, is_active, last_used, created_at 
		FROM project_configs 
		ORDER BY name DESC
	`)

	if err != nil {
		writeJSON(w, &executor.QueryResult{
			Type:  "ERROR",
			Error: err.Error(),
		})
		return
	}

	var projects []map[string]interface{}

	if result.Type == "SELECT" && len(result.Rows) > 0 {
		for _, row := range result.Rows {
			if len(row) >= 5 {
				var idStr string
				switch v := row[0].(type) {
				case int64:
					idStr = fmt.Sprintf("%d", v)
				case int:
					idStr = fmt.Sprintf("%d", v)
				case float64:
					idStr = fmt.Sprintf("%.0f", v)
				case string:
					idStr = v
				default:
					idStr = fmt.Sprintf("%v", row[0])
				}
				isActive := false
				if val, ok := row[4].(int64); ok && val == 1 {
					isActive = true
				}
				projects = append(projects, map[string]interface{}{
					"id":          idStr,
					"name":        row[1],
					"root_path":   row[2],
					"description": row[3],
					"is_active":   isActive,
				})
			}
		}
	}

	if projects == nil {
		projects = []map[string]interface{}{}
	}

	writeJSON(w, map[string]interface{}{
		"projects": projects,
		"count":    len(projects),
	})
}

func (s *Server) handleAddProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		RootPath    string `json:"root_path"`
		Description string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, &executor.QueryResult{
			Type:  "ERROR",
			Error: "неверный JSON",
		})
		return
	}

	absPath, err := filepath.Abs(req.RootPath)
	if err != nil {
		writeJSON(w, &executor.QueryResult{
			Type:  "ERROR",
			Error: "неверный путь: " + err.Error(),
		})
		return
	}

	project, err := s.config.AddProject(req.Name, absPath, req.Description)
	if err != nil {
		writeJSON(w, &executor.QueryResult{
			Type:  "ERROR",
			Error: err.Error(),
		})
		return
	}

	writeJSON(w, map[string]interface{}{
		"success": true,
		"project": project,
	})
}

func (s *Server) handleSetActiveProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProjectID int `json:"project_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, &executor.QueryResult{
			Type:  "ERROR",
			Error: "неверный JSON",
		})
		return
	}

	if err := s.config.SetActiveProject(req.ProjectID); err != nil {
		writeJSON(w, &executor.QueryResult{
			Type:  "ERROR",
			Error: err.Error(),
		})
		return
	}

	writeJSON(w, map[string]interface{}{
		"success": true,
	})
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		settings, err := s.config.GetSettings()
		if err != nil {
			writeJSON(w, &executor.QueryResult{
				Type:  "ERROR",
				Error: err.Error(),
			})
			return
		}
		writeJSON(w, map[string]interface{}{
			"settings": settings,
		})
		return
	}

	if r.Method == "POST" {
		var req struct {
			StreamingEnabled *bool  `json:"streaming_enabled"`
			Theme            string `json:"theme"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, &executor.QueryResult{
				Type:  "ERROR",
				Error: "неверный JSON",
			})
			return
		}

		if req.StreamingEnabled != nil {
			s.config.SetStreamingEnabled(*req.StreamingEnabled)
		}

		writeJSON(w, map[string]interface{}{
			"success": true,
		})
		return
	}
}

func (s *Server) handleContextRollback(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID string `json:"session_id"`
		Steps     int    `json:"steps"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	// Откатываем через SQL
	s.Exec.Execute(fmt.Sprintf(
		"UPDATE reasoning_space SET rolled_back = 1 WHERE session_id = %s", req.SessionID))
	s.Exec.Execute(fmt.Sprintf(
		"UPDATE buffer_space SET rolled_back = 1 WHERE session_id = %s", req.SessionID))
	s.Exec.Execute(fmt.Sprintf(
		"UPDATE inference_space SET rolled_back = 1 WHERE session_id = %s", req.SessionID))

	writeJSON(w, map[string]interface{}{"status": "ok"})
}

func (s *Server) handleContextGC(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID string `json:"session_id"`
		GCType    string `json:"gc_type"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	// Очистка
	s.Exec.Execute(fmt.Sprintf(
		"DELETE FROM buffer_space WHERE session_id = %s AND (created_at + ttl < strftime('%%s', 'now') OR rolled_back = 1)", req.SessionID))
	s.Exec.Execute(fmt.Sprintf(
		"DELETE FROM reasoning_space WHERE session_id = %s AND rolled_back = 1", req.SessionID))
	s.Exec.Execute(fmt.Sprintf(
		"DELETE FROM inference_space WHERE session_id = %s AND rolled_back = 1", req.SessionID))

	writeJSON(w, map[string]interface{}{"status": "ok"})
}

func (s *Server) handleCode(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("file")
	startByteStr := r.URL.Query().Get("start_byte")
	endByteStr := r.URL.Query().Get("end_byte")

	if filePath == "" || startByteStr == "" || endByteStr == "" {
		writeJSON(w, map[string]string{"error": "недостаточно параметров: нужны file, start_byte, end_byte"})
		return
	}

	startByte, err := strconv.ParseInt(startByteStr, 10, 64)
	if err != nil {
		writeJSON(w, map[string]string{"error": "неверный start_byte"})
		return
	}
	endByte, err := strconv.ParseInt(endByteStr, 10, 64)
	if err != nil {
		writeJSON(w, map[string]string{"error": "неверный end_byte"})
		return
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		writeJSON(w, map[string]string{"error": err.Error()})
		return
	}

	// Валидация границ
	if startByte < 0 {
		startByte = 0
	}
	if endByte > int64(len(content)) {
		endByte = int64(len(content))
	}
	if startByte > endByte {
		startByte, endByte = endByte, startByte
	}

	// Вырезаем код
	code := string(content[startByte:endByte])

	// Вычисляем номера строк для отображения
	linesBefore := bytes.Count(content[:startByte], []byte{'\n'})
	startLine := linesBefore + 1
	endLine := startLine + bytes.Count(content[startByte:endByte], []byte{'\n'})

	writeJSON(w, map[string]interface{}{
		"file":       filePath,
		"start_byte": startByte,
		"end_byte":   endByte,
		"start_line": startLine,
		"end_line":   endLine,
		"code":       code,
	})
}

// ========== Helper Functions ==========

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("JSON encode error: %v", err)
	}
}

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

func parseIntSafe(s string) int {
	var i int
	fmt.Sscanf(s, "%d", &i)
	return i
}
