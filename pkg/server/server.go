package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"

	"agent-db/pkg/executor"
)

//go:embed static
var staticFiles embed.FS

type Server struct {
	Exec *executor.Executor
}

func NewServer(exec *executor.Executor) *Server {
	return &Server{Exec: exec}
}

func (s *Server) Start(addr string) error {
	http.HandleFunc("/api/query", s.handleQuery)
	http.HandleFunc("/api/tables", s.handleTables)
	http.HandleFunc("/api/schema", s.handleSchema)

	// Раздаём встроенную статику
	staticFS, _ := fs.Sub(staticFiles, "static")
	http.Handle("/", http.FileServer(http.FS(staticFS)))

	fmt.Printf("🌐 AgentDB Web UI: http://localhost%s\n", addr)
	return http.ListenAndServe(addr, nil)
}

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

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(data)
}