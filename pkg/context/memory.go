package context

import (
	"agent-db/pkg/executor"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"log"
	"strings"
	"time"
)

// ========== ТИПЫ ДАННЫХ ==========

type MemoryLevel int

const (
	Metaspace MemoryLevel = iota
	Heap
	Stack
)

type Inference struct {
	ID         int64
	SessionID  int
	Conclusion string
	Confidence float64
	Type       string
	Source     string
	VersionID  int
	CreatedAt  int64
	ExpiresAt  int64
	Metadata   map[string]interface{}
}

type Thought struct {
	ID            int64
	SessionID     int
	InstructionID int64
	Type          string
	Content       string
	Confidence    float64
	VersionID     int
	CreatedAt     int64
}

type Instruction struct {
	ID          int64
	SessionID   int
	ParentID    int64
	Content     string
	Status      string
	Depth       int
	VersionID   int
	CreatedAt   int64
	CompletedAt int64
}

type BufferItem struct {
	Key         string
	Value       string
	TTL         int
	VersionID   int
	CreatedAt   int64
	AccessCount int
	LastAccess  int64
}

type Observation struct {
	ID        int64
	SessionID int
	ToolName  string
	Input     string
	Output    string
	Success   bool
	VersionID int
	CreatedAt int64
}

type InstructionNode struct {
	Instruction Instruction
	Children    []InstructionNode
}

type Session struct {
	ID           int
	CurrentEpoch int
}

// ========== MEMORY MANAGER ==========

type MemoryManager struct {
	exec *executor.Executor
}

func NewMemoryManager(exec *executor.Executor) *MemoryManager {
	mm := &MemoryManager{exec: exec}
	mm.initTables()
	return mm
}

// generateID — криптографически стойкий случайный int64
func generateID() int64 {
	b := make([]byte, 8)
	_, err := rand.Read(b)
	if err != nil {
		// Fallback на timestamp + случайное
		return time.Now().UnixNano() ^ int64(time.Now().Nanosecond())
	}
	return int64(binary.BigEndian.Uint64(b))
}

func (mm *MemoryManager) initTables() {
	tables := []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			id INT PRIMARY_KEY,
			current_epoch INT DEFAULT 0,
			created_at INT NOT_NULL
		)`,
		`CREATE TABLE IF NOT EXISTS instruction_stack (
			id INT PRIMARY_KEY,
			session_id INT NOT_NULL,
			version_id INT NOT_NULL,
			rolled_back INT DEFAULT 0,
			parent_id INT,
			depth INT DEFAULT 0,
			content TEXT NOT_NULL,
			status TEXT DEFAULT 'pending',
			created_at INT NOT_NULL,
			completed_at INT
		)`,
		`CREATE TABLE IF NOT EXISTS reasoning_space (
			id INT PRIMARY_KEY,
			session_id INT NOT_NULL,
			version_id INT NOT_NULL,
			rolled_back INT DEFAULT 0,
			instruction_id INT,
			thought_type TEXT NOT_NULL,
			content TEXT NOT_NULL,
			confidence REAL DEFAULT 0.5,
			created_at INT NOT_NULL
		)`,
		`CREATE TABLE IF NOT EXISTS buffer_space (
			id INT PRIMARY_KEY,
			session_id INT NOT_NULL,
			version_id INT NOT_NULL,
			rolled_back INT DEFAULT 0,
			key TEXT,
			value TEXT,
			ttl INT DEFAULT 300,
			created_at INT NOT_NULL,
			access_count INT DEFAULT 0,
			last_access INT
		)`,
		`CREATE TABLE IF NOT EXISTS inference_space (
			id INT PRIMARY_KEY,
			session_id INT NOT_NULL,
			version_id INT NOT_NULL,
			rolled_back INT DEFAULT 0,
			conclusion TEXT NOT_NULL,
			confidence REAL DEFAULT 0.5,
			inference_type TEXT DEFAULT 'assumption',
			source TEXT,
			created_at INT NOT_NULL,
			expires_at INT
		)`,
		`CREATE TABLE IF NOT EXISTS metaspace (
			id INT PRIMARY_KEY,
			agent_id TEXT NOT_NULL,
			version INT NOT_NULL,
			content_type TEXT NOT_NULL,
			content TEXT NOT_NULL,
			priority INT DEFAULT 0,
			is_active INT DEFAULT 1
		)`,
		`CREATE TABLE IF NOT EXISTS version_memory (
			version_id INT PRIMARY_KEY,
			instructions TEXT,
			thoughts TEXT,
			inferences TEXT,
			buffer TEXT,
			created_at INT
		)`,
		`CREATE TABLE IF NOT EXISTS observations (
			id INT PRIMARY_KEY,
			session_id INT NOT_NULL,
			version_id INT NOT_NULL,
			tool_name TEXT,
			input TEXT,
			output TEXT,
			success INT,
			created_at INT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_instructions_version ON instruction_stack(version_id)`,
		`CREATE INDEX IF NOT EXISTS idx_thoughts_version ON reasoning_space(version_id)`,
		`CREATE INDEX IF NOT EXISTS idx_inferences_version ON inference_space(version_id)`,
		`CREATE INDEX IF NOT EXISTS idx_buffer_version ON buffer_space(version_id)`,
	}
	for _, sql := range tables {
		mm.exec.Execute(sql)
	}
}

// ========== METASPACE ==========

func (mm *MemoryManager) SetMetaspace(key, value, contentType string, priority int) error {
	id := generateID()
	_, err := mm.exec.Execute(fmt.Sprintf(`
		INSERT INTO metaspace (id, agent_id, version, content_type, content, priority, is_active)
		VALUES (%d, '%s', 1, '%s', '%s', %d, 1)
	`, id, escapeSQL(key), contentType, escapeSQL(value), priority))
	return err
}

func (mm *MemoryManager) GetMetaspace(key string) (string, error) {
	result, _ := mm.exec.Execute(fmt.Sprintf(
		`SELECT content FROM metaspace WHERE agent_id = '%s' AND is_active = 1`, escapeSQL(key)))
	return mm.firstValue(result), nil
}

func (mm *MemoryManager) GetAllMetaspace() map[string]string {
	result, _ := mm.exec.Execute(`
		SELECT agent_id, content FROM metaspace 
		WHERE is_active = 1 
		ORDER BY priority DESC
	`)
	return mm.toMap(result)
}

// ========== HEAP ==========

func (mm *MemoryManager) AddInference(sessionID, versionID int, conclusion string, confidence float64, inferenceType, source string) error {
	now := time.Now().Unix()
	expiresAt := now + 86400
	id := generateID()

	_, err := mm.exec.Execute(fmt.Sprintf(`
		INSERT INTO inference_space (id, session_id, version_id, conclusion, confidence, inference_type, source, created_at, expires_at)
		VALUES (%d, %d, %d, '%s', %f, '%s', '%s', %d, %d)
	`, id, sessionID, versionID, escapeSQL(conclusion), confidence, inferenceType, source, now, expiresAt))
	return err
}

func (mm *MemoryManager) GetActiveInferences(sessionID, versionID int, minConfidence float64) []Inference {
	result, _ := mm.exec.Execute(fmt.Sprintf(`
		SELECT id, conclusion, confidence, inference_type, source, created_at
		FROM inference_space 
		WHERE session_id = %d 
		  AND version_id = %d
		  AND confidence >= %f 
		  AND (expires_at = 0 OR expires_at > %d)
		  AND rolled_back = 0
		ORDER BY confidence DESC
		LIMIT 20
	`, sessionID, versionID, minConfidence, time.Now().Unix()))

	return mm.parseInferences(result)
}

func (mm *MemoryManager) SetBuffer(sessionID, versionID int, key, value string, ttl int) error {
	now := time.Now().Unix()
	if ttl == 0 {
		ttl = 300
	}
	id := generateID()

	_, err := mm.exec.Execute(fmt.Sprintf(`
		INSERT INTO buffer_space (id, session_id, version_id, key, value, ttl, created_at)
		VALUES (%d, %d, %d, '%s', '%s', %d, %d)
	`, id, sessionID, versionID, escapeSQL(key), escapeSQL(value), ttl, now))
	return err
}

func (mm *MemoryManager) GetBuffer(sessionID, versionID int, key string) (string, bool) {
	result, _ := mm.exec.Execute(fmt.Sprintf(`
		SELECT value FROM buffer_space 
		WHERE session_id = %d 
		  AND version_id = %d
		  AND key = '%s' 
		  AND (created_at + ttl > %d)
		  AND rolled_back = 0
	`, sessionID, versionID, escapeSQL(key), time.Now().Unix()))

	val := mm.firstValue(result)
	if val != "" {
		mm.exec.Execute(fmt.Sprintf(`
			UPDATE buffer_space SET access_count = access_count + 1, last_access = %d
			WHERE session_id = %d AND version_id = %d AND key = '%s'
		`, time.Now().Unix(), sessionID, versionID, escapeSQL(key)))
		return val, true
	}
	return "", false
}

func (mm *MemoryManager) AddObservation(sessionID, versionID int, toolName, input, output string, success bool) error {
	now := time.Now().Unix()
	successInt := 0
	if success {
		successInt = 1
	}
	id := generateID()

	_, err := mm.exec.Execute(fmt.Sprintf(`
		INSERT INTO observations (id, session_id, version_id, tool_name, input, output, success, created_at)
		VALUES (%d, %d, %d, '%s', '%s', '%s', %d, %d)
	`, id, sessionID, versionID, escapeSQL(toolName), escapeSQL(input), escapeSQL(output), successInt, now))
	return err
}

// ========== STACK ==========

func (mm *MemoryManager) PushInstruction(sessionID, versionID int, content string, parentID int64) (int64, error) {
	log.Printf("[DEBUG] PushInstruction START: session=%d, version=%d, parent=%d", sessionID, versionID, parentID)

	if err := mm.exec.BeginTransaction(); err != nil {
		return 0, err
	}

	depth := 0
	if parentID > 0 {
		result, _ := mm.exec.ExecuteInTxWithResult(fmt.Sprintf(
			`SELECT depth FROM instruction_stack WHERE id = %d`, parentID))
		if d := mm.firstValue(result); d != "" {
			fmt.Sscanf(d, "%d", &depth)
			depth++
		}
	}

	id := generateID()
	log.Printf("[DEBUG] PushInstruction: id=%d", id)

	now := time.Now().Unix()
	sql := fmt.Sprintf(`
		INSERT INTO instruction_stack (id, session_id, version_id, parent_id, content, depth, created_at)
		VALUES (%d, %d, %d, %d, '%s', %d, %d)
	`, id, sessionID, versionID, parentID, escapeSQL(content), depth, now)

	_, err := mm.exec.ExecuteInTx(sql)
	if err != nil {
		mm.exec.Rollback()
		return 0, err
	}

	if err := mm.exec.Commit(); err != nil {
		mm.exec.Rollback()
		return 0, err
	}

	log.Printf("[DEBUG] PushInstruction success, id=%d", id)
	return id, nil
}

func (mm *MemoryManager) PopInstruction(id int64, success bool) error {
	status := "completed"
	if !success {
		status = "failed"
	}
	now := time.Now().Unix()
	_, err := mm.exec.Execute(fmt.Sprintf(`
		UPDATE instruction_stack SET status = '%s', completed_at = %d
		WHERE id = %d
	`, status, now, id))
	return err
}

func (mm *MemoryManager) GetCurrentStack(sessionID, versionID int) []Instruction {
	result, _ := mm.exec.Execute(fmt.Sprintf(`
		SELECT id, parent_id, content, status, depth, created_at
		FROM instruction_stack 
		WHERE session_id = %d 
		  AND version_id = %d
		  AND status = 'pending'
		ORDER BY depth ASC
	`, sessionID, versionID))
	return mm.parseInstructions(result)
}

func (mm *MemoryManager) AddThought(sessionID, versionID int, instructionID int64, thoughtType, content string, confidence float64) error {
	now := time.Now().Unix()
	id := generateID()

	_, err := mm.exec.Execute(fmt.Sprintf(`
		INSERT INTO reasoning_space (id, session_id, version_id, instruction_id, thought_type, content, confidence, created_at)
		VALUES (%d, %d, %d, %d, '%s', '%s', %f, %d)
	`, id, sessionID, versionID, instructionID, thoughtType, escapeSQL(content), confidence, now))
	return err
}

// ========== GC ==========

func (mm *MemoryManager) GC(sessionID, versionID int) {
	now := time.Now().Unix()

	mm.exec.Execute(fmt.Sprintf(`
		DELETE FROM buffer_space 
		WHERE session_id = %d AND version_id = %d
		  AND (created_at + ttl < %d OR rolled_back = 1)
	`, sessionID, versionID, now))

	weekAgo := now - 7*86400
	mm.exec.Execute(fmt.Sprintf(`
		DELETE FROM observations 
		WHERE session_id = %d AND created_at < %d
	`, sessionID, weekAgo))

	mm.exec.Execute(fmt.Sprintf(`
		DELETE FROM reasoning_space WHERE session_id = %d AND rolled_back = 1
	`, sessionID))
	mm.exec.Execute(fmt.Sprintf(`
		DELETE FROM inference_space WHERE session_id = %d AND rolled_back = 1
	`, sessionID))
}

// ========== ВСПОМОГАТЕЛЬНЫЕ ==========

func (mm *MemoryManager) firstValue(qr *executor.QueryResult) string {
	if qr == nil || qr.Type == "ERROR" || len(qr.Rows) == 0 || len(qr.Rows[0]) == 0 {
		return ""
	}
	if qr.Rows[0][0] == nil {
		return ""
	}
	return fmt.Sprintf("%v", qr.Rows[0][0])
}

func (mm *MemoryManager) toMap(qr *executor.QueryResult) map[string]string {
	result := make(map[string]string)
	if qr == nil || qr.Type == "ERROR" {
		return result
	}
	for _, row := range qr.Rows {
		if len(row) >= 2 && row[0] != nil && row[1] != nil {
			result[fmt.Sprintf("%v", row[0])] = fmt.Sprintf("%v", row[1])
		}
	}
	return result
}

func (mm *MemoryManager) parseInferences(qr *executor.QueryResult) []Inference {
	var inferences []Inference
	if qr == nil || qr.Type == "ERROR" {
		return inferences
	}
	for _, row := range qr.Rows {
		if len(row) >= 5 {
			inf := Inference{}
			inf.ID = parseInt64(fmt.Sprintf("%v", row[0]))
			inf.Conclusion = fmt.Sprintf("%v", row[1])
			inf.Confidence = parseFloat(fmt.Sprintf("%v", row[2]))
			inf.Type = fmt.Sprintf("%v", row[3])
			inf.Source = fmt.Sprintf("%v", row[4])
			inferences = append(inferences, inf)
		}
	}
	return inferences
}

func (mm *MemoryManager) parseInstructions(qr *executor.QueryResult) []Instruction {
	var instructions []Instruction
	if qr == nil || qr.Type == "ERROR" {
		return instructions
	}
	for _, row := range qr.Rows {
		if len(row) >= 5 {
			inst := Instruction{}
			inst.ID = parseInt64(fmt.Sprintf("%v", row[0]))
			if len(row) > 1 {
				inst.ParentID = parseInt64(fmt.Sprintf("%v", row[1]))
			}
			inst.Content = fmt.Sprintf("%v", row[2])
			inst.Status = fmt.Sprintf("%v", row[3])
			inst.Depth = parseInt(fmt.Sprintf("%v", row[4]))
			if len(row) > 5 {
				inst.CreatedAt = parseInt64(fmt.Sprintf("%v", row[5]))
			}
			instructions = append(instructions, inst)
		}
	}
	return instructions
}

func (mm *MemoryManager) parseThoughts(qr *executor.QueryResult) []Thought {
	var thoughts []Thought
	if qr == nil || qr.Type == "ERROR" {
		return thoughts
	}
	for _, row := range qr.Rows {
		if len(row) >= 5 {
			t := Thought{}
			t.ID = parseInt64(fmt.Sprintf("%v", row[0]))
			t.InstructionID = parseInt64(fmt.Sprintf("%v", row[1]))
			t.Type = fmt.Sprintf("%v", row[2])
			t.Content = fmt.Sprintf("%v", row[3])
			t.Confidence = parseFloat(fmt.Sprintf("%v", row[4]))
			if len(row) > 5 {
				t.CreatedAt = parseInt64(fmt.Sprintf("%v", row[5]))
			}
			thoughts = append(thoughts, t)
		}
	}
	return thoughts
}

func (mm *MemoryManager) parseStringList(qr *executor.QueryResult) []string {
	var result []string
	if qr == nil || qr.Type == "ERROR" {
		return result
	}
	for _, row := range qr.Rows {
		if len(row) > 0 && row[0] != nil {
			result = append(result, fmt.Sprintf("%v", row[0]))
		}
	}
	return result
}

func (mm *MemoryManager) getInstructionsFlat(sessionID, versionID int) []Instruction {
	result, _ := mm.exec.Execute(fmt.Sprintf(`
		SELECT id, parent_id, content, status, depth, created_at
		FROM instruction_stack 
		WHERE session_id = %d 
		  AND version_id = %d
		  AND rolled_back = 0
		ORDER BY depth ASC, created_at ASC
	`, sessionID, versionID))
	return mm.parseInstructions(result)
}

// ========== ДОПОЛНИТЕЛЬНЫЕ МЕТОДЫ ==========

func (mm *MemoryManager) GetRecentThoughts(sessionID, versionID int, limit int) []Thought {
	if limit <= 0 {
		limit = 10
	}
	result, _ := mm.exec.Execute(fmt.Sprintf(`
		SELECT id, instruction_id, thought_type, content, confidence, created_at
		FROM reasoning_space 
		WHERE session_id = %d 
		  AND version_id = %d
		  AND rolled_back = 0
		ORDER BY created_at DESC
		LIMIT %d
	`, sessionID, versionID, limit))
	return mm.parseThoughts(result)
}

func (mm *MemoryManager) GetThoughtsByInstruction(sessionID, versionID int, instructionID int64) []Thought {
	result, _ := mm.exec.Execute(fmt.Sprintf(`
		SELECT id, instruction_id, thought_type, content, confidence, created_at
		FROM reasoning_space 
		WHERE session_id = %d 
		  AND version_id = %d
		  AND instruction_id = %d
		  AND rolled_back = 0
		ORDER BY created_at ASC
	`, sessionID, versionID, instructionID))
	return mm.parseThoughts(result)
}

func (mm *MemoryManager) GetBufferKeys(sessionID, versionID int) []string {
	result, _ := mm.exec.Execute(fmt.Sprintf(`
		SELECT DISTINCT key 
		FROM buffer_space 
		WHERE session_id = %d 
		  AND version_id = %d
		  AND rolled_back = 0
		  AND (created_at + ttl > %d)
		ORDER BY access_count DESC
	`, sessionID, versionID, time.Now().Unix()))
	return mm.parseStringList(result)
}

func (mm *MemoryManager) GetBufferAll(sessionID, versionID int) map[string]string {
	result, _ := mm.exec.Execute(fmt.Sprintf(`
		SELECT key, value 
		FROM buffer_space 
		WHERE session_id = %d 
		  AND version_id = %d
		  AND rolled_back = 0
		  AND (created_at + ttl > %d)
		ORDER BY access_count DESC
	`, sessionID, versionID, time.Now().Unix()))
	return mm.toMap(result)
}

func (mm *MemoryManager) GetBufferByPrefix(sessionID, versionID int, prefix string) map[string]string {
	result, _ := mm.exec.Execute(fmt.Sprintf(`
		SELECT key, value 
		FROM buffer_space 
		WHERE session_id = %d 
		  AND version_id = %d
		  AND key LIKE '%s%%'
		  AND rolled_back = 0
		  AND (created_at + ttl > %d)
	`, sessionID, versionID, escapeSQL(prefix), time.Now().Unix()))
	return mm.toMap(result)
}

func (mm *MemoryManager) DeleteBufferKey(sessionID, versionID int, key string) error {
	_, err := mm.exec.Execute(fmt.Sprintf(`
		UPDATE buffer_space SET rolled_back = 1
		WHERE session_id = %d AND version_id = %d AND key = '%s'
	`, sessionID, versionID, escapeSQL(key)))
	return err
}

func (mm *MemoryManager) IncrementBufferAccess(sessionID, versionID int, key string) {
	mm.exec.Execute(fmt.Sprintf(`
		UPDATE buffer_space 
		SET access_count = access_count + 1, last_access = %d
		WHERE session_id = %d AND version_id = %d AND key = '%s'
	`, time.Now().Unix(), sessionID, versionID, escapeSQL(key)))
}

func (mm *MemoryManager) GetInstructionTree(sessionID, versionID int) []InstructionNode {
	instructions := mm.getInstructionsFlat(sessionID, versionID)
	return mm.buildInstructionTree(instructions, 0)
}

func (mm *MemoryManager) buildInstructionTree(instructions []Instruction, parentID int64) []InstructionNode {
	var result []InstructionNode
	for _, inst := range instructions {
		if inst.ParentID == parentID {
			node := InstructionNode{
				Instruction: inst,
				Children:    mm.buildInstructionTree(instructions, inst.ID),
			}
			result = append(result, node)
		}
	}
	return result
}

func (mm *MemoryManager) GetCurrentInstruction(sessionID, versionID int) *Instruction {
	result, _ := mm.exec.Execute(fmt.Sprintf(`
		SELECT id, parent_id, content, status, depth, created_at
		FROM instruction_stack 
		WHERE session_id = %d 
		  AND version_id = %d
		  AND status = 'executing'
		LIMIT 1
	`, sessionID, versionID))
	instructions := mm.parseInstructions(result)
	if len(instructions) > 0 {
		return &instructions[0]
	}
	return nil
}

func (mm *MemoryManager) GetInferencesByType(sessionID, versionID int, inferenceType string) []Inference {
	result, _ := mm.exec.Execute(fmt.Sprintf(`
		SELECT id, conclusion, confidence, inference_type, source, created_at
		FROM inference_space 
		WHERE session_id = %d 
		  AND version_id = %d
		  AND inference_type = '%s'
		  AND rolled_back = 0
		  AND (expires_at = 0 OR expires_at > %d)
		ORDER BY confidence DESC
	`, sessionID, versionID, inferenceType, time.Now().Unix()))
	return mm.parseInferences(result)
}

func (mm *MemoryManager) GetInferencesBySource(sessionID, versionID int, source string) []Inference {
	result, _ := mm.exec.Execute(fmt.Sprintf(`
		SELECT id, conclusion, confidence, inference_type, source, created_at
		FROM inference_space 
		WHERE session_id = %d 
		  AND version_id = %d
		  AND source = '%s'
		  AND rolled_back = 0
		  AND (expires_at = 0 OR expires_at > %d)
		ORDER BY confidence DESC
	`, sessionID, versionID, source, time.Now().Unix()))
	return mm.parseInferences(result)
}

func (mm *MemoryManager) InvalidateInference(id int64) error {
	_, err := mm.exec.Execute(fmt.Sprintf(`
		UPDATE inference_space SET rolled_back = 1, expires_at = %d
		WHERE id = %d
	`, time.Now().Unix(), id))
	return err
}

func (mm *MemoryManager) InvalidateInferencesByVersion(versionID int) error {
	_, err := mm.exec.Execute(fmt.Sprintf(`
		UPDATE inference_space SET rolled_back = 1, expires_at = %d
		WHERE version_id = %d
	`, time.Now().Unix(), versionID))
	return err
}

// ========== GC СТАТИСТИКА ==========

type GCStats struct {
	DeletedBuffer       int
	DeletedThoughts     int
	DeletedInferences   int
	DeletedObservations int
	FreedBytes          int64
}

func (mm *MemoryManager) GetGCStats(sessionID int) *GCStats {
	stats := &GCStats{}
	result, _ := mm.exec.Execute(fmt.Sprintf(`
		SELECT 
			(SELECT COUNT(*) FROM buffer_space WHERE session_id = %d AND rolled_back = 1),
			(SELECT COUNT(*) FROM reasoning_space WHERE session_id = %d AND rolled_back = 1),
			(SELECT COUNT(*) FROM inference_space WHERE session_id = %d AND rolled_back = 1),
			(SELECT COUNT(*) FROM observations WHERE session_id = %d)
	`, sessionID, sessionID, sessionID, sessionID))

	if result != nil && result.Type == "SELECT" && len(result.Rows) > 0 && len(result.Rows[0]) >= 4 {
		stats.DeletedBuffer = parseInt(fmt.Sprintf("%v", result.Rows[0][0]))
		stats.DeletedThoughts = parseInt(fmt.Sprintf("%v", result.Rows[0][1]))
		stats.DeletedInferences = parseInt(fmt.Sprintf("%v", result.Rows[0][2]))
		stats.DeletedObservations = parseInt(fmt.Sprintf("%v", result.Rows[0][3]))
	}
	return stats
}

// ========== МЕТОДЫ ДЛЯ СНАПШОТОВ ==========

func (mm *MemoryManager) GetAllInstructions(sessionID int) []Instruction {
	var instructions []Instruction
	result, _ := mm.exec.Execute(fmt.Sprintf(`
		SELECT id, session_id, content, depth, status, created_at
		FROM instruction_stack 
		WHERE session_id = %d AND rolled_back = 0
		ORDER BY created_at ASC
	`, sessionID))

	if result == nil || result.Type == "ERROR" {
		return instructions
	}
	for _, row := range result.Rows {
		if len(row) >= 6 {
			inst := Instruction{}
			inst.ID = parseInt64(fmt.Sprintf("%v", row[0]))
			inst.SessionID = parseInt(fmt.Sprintf("%v", row[1]))
			inst.Content = fmt.Sprintf("%v", row[2])
			inst.Depth = parseInt(fmt.Sprintf("%v", row[3]))
			inst.Status = fmt.Sprintf("%v", row[4])
			inst.CreatedAt = parseInt64(fmt.Sprintf("%v", row[5]))
			instructions = append(instructions, inst)
		}
	}
	return instructions
}

func (mm *MemoryManager) GetAllThoughts(sessionID int) []Thought {
	var thoughts []Thought
	result, _ := mm.exec.Execute(fmt.Sprintf(`
		SELECT id, session_id, thought_type, content, confidence, created_at
		FROM reasoning_space 
		WHERE session_id = %d AND rolled_back = 0
		ORDER BY created_at ASC
	`, sessionID))

	if result == nil || result.Type == "ERROR" {
		return thoughts
	}
	for _, row := range result.Rows {
		if len(row) >= 6 {
			t := Thought{}
			t.ID = parseInt64(fmt.Sprintf("%v", row[0]))
			t.SessionID = parseInt(fmt.Sprintf("%v", row[1]))
			t.Type = fmt.Sprintf("%v", row[2])
			t.Content = fmt.Sprintf("%v", row[3])
			t.Confidence = parseFloat(fmt.Sprintf("%v", row[4]))
			t.CreatedAt = parseInt64(fmt.Sprintf("%v", row[5]))
			thoughts = append(thoughts, t)
		}
	}
	return thoughts
}

func (mm *MemoryManager) GetAllInferences(sessionID int) []Inference {
	var inferences []Inference
	result, _ := mm.exec.Execute(fmt.Sprintf(`
		SELECT id, session_id, conclusion, confidence, inference_type, source, created_at
		FROM inference_space 
		WHERE session_id = %d AND rolled_back = 0
		ORDER BY created_at ASC
	`, sessionID))

	if result == nil || result.Type == "ERROR" {
		return inferences
	}
	for _, row := range result.Rows {
		if len(row) >= 7 {
			inf := Inference{}
			inf.ID = parseInt64(fmt.Sprintf("%v", row[0]))
			inf.SessionID = parseInt(fmt.Sprintf("%v", row[1]))
			inf.Conclusion = fmt.Sprintf("%v", row[2])
			inf.Confidence = parseFloat(fmt.Sprintf("%v", row[3]))
			inf.Type = fmt.Sprintf("%v", row[4])
			inf.Source = fmt.Sprintf("%v", row[5])
			inf.CreatedAt = parseInt64(fmt.Sprintf("%v", row[6]))
			inferences = append(inferences, inf)
		}
	}
	return inferences
}

func (mm *MemoryManager) GetAllBuffer(sessionID int) map[string]BufferItem {
	buffer := make(map[string]BufferItem)
	result, _ := mm.exec.Execute(fmt.Sprintf(`
		SELECT key, value, ttl, created_at
		FROM buffer_space 
		WHERE session_id = %d AND rolled_back = 0
	`, sessionID))

	if result == nil || result.Type == "ERROR" {
		return buffer
	}
	for _, row := range result.Rows {
		if len(row) >= 4 && row[0] != nil {
			item := BufferItem{}
			item.Key = fmt.Sprintf("%v", row[0])
			if row[1] != nil {
				item.Value = fmt.Sprintf("%v", row[1])
			}
			item.TTL = parseInt(fmt.Sprintf("%v", row[2]))
			item.CreatedAt = parseInt64(fmt.Sprintf("%v", row[3]))
			buffer[item.Key] = item
		}
	}
	return buffer
}

// ========== ПАРСИНГ ==========

func parseInt(s string) int {
	var i int
	fmt.Sscanf(s, "%d", &i)
	return i
}

func parseInt64(s string) int64 {
	var i int64
	fmt.Sscanf(s, "%d", &i)
	return i
}

func parseFloat(s string) float64 {
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}

func escapeSQL(s string) string {
	s = strings.ReplaceAll(s, "'", "''")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

func (mm *MemoryManager) GetOrCreateSession(sessionID int) (*Session, error) {
	result, err := mm.exec.Execute(fmt.Sprintf(
		"SELECT id, current_epoch FROM sessions WHERE id = %d", sessionID))
	if err != nil {
		return nil, err
	}

	if result == nil || result.Type == "ERROR" || len(result.Rows) == 0 {
		now := time.Now().Unix()
		_, err := mm.exec.Execute(fmt.Sprintf(`
			INSERT INTO sessions (id, current_epoch, created_at)
			VALUES (%d, 0, %d)
		`, sessionID, now))
		if err != nil {
			return nil, fmt.Errorf("ошибка создания сессии: %w", err)
		}
		return &Session{ID: sessionID, CurrentEpoch: 0}, nil
	}

	epoch := 0
	if len(result.Rows) > 0 && len(result.Rows[0]) > 1 && result.Rows[0][1] != nil {
		switch v := result.Rows[0][1].(type) {
		case int64:
			epoch = int(v)
		case int:
			epoch = v
		}
	}
	return &Session{ID: sessionID, CurrentEpoch: epoch}, nil
}
