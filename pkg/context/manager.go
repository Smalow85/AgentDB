package context

import (
	"agent-db/pkg/executor"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type ContextManager struct {
	exec *executor.Executor
}

type Session struct {
	ID           int
	CurrentEpoch int
}

func NewContextManager(exec *executor.Executor) *ContextManager {
	cm := &ContextManager{exec: exec}
	cm.initTables()
	return cm
}

func (cm *ContextManager) initTables() {
	tables := []string{
		`CREATE TABLE IF NOT EXISTS sessions (
            id INTEGER PRIMARY KEY,
            current_epoch INTEGER DEFAULT 0,
            created_at INTEGER NOT NULL
        )`,
		`CREATE TABLE IF NOT EXISTS instruction_stack (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            session_id INTEGER NOT NULL,
            epoch INTEGER NOT NULL,
            rolled_back INTEGER DEFAULT 0,
            parent_id INTEGER,
            depth INTEGER DEFAULT 0,
            content TEXT NOT NULL
        )`,
		`CREATE TABLE IF NOT EXISTS reasoning_space (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            session_id INTEGER NOT NULL,
            epoch INTEGER NOT NULL,
            rolled_back INTEGER DEFAULT 0,
            parent_thought_id INTEGER,
            thought_type TEXT NOT NULL,
            content TEXT NOT NULL
        )`,
		`CREATE TABLE IF NOT EXISTS buffer_space (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            session_id INTEGER NOT NULL,
            epoch INTEGER NOT NULL,
            rolled_back INTEGER DEFAULT 0,
            key TEXT,
            data TEXT,
            ttl INTEGER DEFAULT 300,
            created_at INTEGER NOT NULL
        )`,
		`CREATE TABLE IF NOT EXISTS inference_space (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            session_id INTEGER NOT NULL,
            epoch INTEGER NOT NULL,
            rolled_back INTEGER DEFAULT 0,
            conclusion TEXT NOT NULL,
            confidence REAL DEFAULT 0.5,
            inference_type TEXT DEFAULT 'assumption'
        )`,
		`CREATE TABLE IF NOT EXISTS metaspace (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            agent_id TEXT NOT NULL,
            version INTEGER NOT NULL,
            content_type TEXT NOT NULL,
            content TEXT NOT NULL,
            priority INTEGER DEFAULT 0,
            is_active INTEGER DEFAULT 1
        )`,
	}

	for _, sql := range tables {
		cm.exec.Execute(sql)
	}
	fmt.Println("✓ ContextManager tables initialized")
}

// getQueryResultText извлекает текст из QueryResult
func (cm *ContextManager) getQueryResultText(qr *executor.QueryResult) string {
	if qr == nil || qr.Type == "ERROR" {
		return ""
	}
	if len(qr.Rows) == 0 {
		return ""
	}
	var result strings.Builder
	for _, row := range qr.Rows {
		if len(row) > 0 && row[0] != nil {
			result.WriteString(fmt.Sprintf("%v", row[0]))
			result.WriteString("\n")
		}
	}
	return result.String()
}

// GetOrCreateSession - получить или создать сессию
func (cm *ContextManager) GetOrCreateSession(sessionID int) (*Session, error) {
	result, err := cm.exec.Execute(fmt.Sprintf("SELECT current_epoch FROM sessions WHERE id = %d", sessionID))

	if err != nil {
		return nil, err
	}

	text := cm.getQueryResultText(result)

	if text == "" {
		// Создаём новую сессию
		now := time.Now().Unix()
		_, err = cm.exec.Execute(fmt.Sprintf(
			"INSERT INTO sessions (id, current_epoch, created_at) VALUES (%d, 0, %d)",
			sessionID, now))
		if err != nil {
			return nil, fmt.Errorf("ошибка создания сессии: %w", err)
		}
		return &Session{ID: sessionID, CurrentEpoch: 0}, nil
	}

	// Парсим текущую эпоху
	epoch, _ := strconv.Atoi(strings.TrimSpace(text))

	return &Session{ID: sessionID, CurrentEpoch: epoch}, nil
}

// PushInstruction - добавить инструкцию
func (cm *ContextManager) PushInstruction(sessionID int, content string, parentID int) error {
	session, err := cm.GetOrCreateSession(sessionID)
	if err != nil {
		return err
	}

	newEpoch := session.CurrentEpoch + 1

	_, err = cm.exec.Execute(fmt.Sprintf(
		"UPDATE sessions SET current_epoch = %d WHERE id = %d",
		newEpoch, sessionID))
	if err != nil {
		return fmt.Errorf("ошибка обновления эпохи: %w", err)
	}

	escapedContent := escapeSQL(content)
	sql := fmt.Sprintf(
		"INSERT INTO instruction_stack (session_id, epoch, rolled_back, parent_id, depth, content) VALUES (%d, %d, 0, %d, 0, '%s')",
		sessionID, newEpoch, parentID, escapedContent)

	_, err = cm.exec.Execute(sql)
	return err
}

// AddThought - добавить мысль
func (cm *ContextManager) AddThought(sessionID int, thoughtType, content string, parentID int) error {
	epochResult, _ := cm.exec.Execute(fmt.Sprintf(
		"SELECT current_epoch FROM sessions WHERE id = %d", sessionID))

	epoch := 1
	if text := cm.getQueryResultText(epochResult); text != "" {
		if e, err := strconv.Atoi(strings.TrimSpace(text)); err == nil {
			epoch = e
		}
	}

	escapedContent := escapeSQL(content)
	escapedType := escapeSQL(thoughtType)

	sql := fmt.Sprintf(`
        INSERT INTO reasoning_space (session_id, epoch, rolled_back, parent_thought_id, thought_type, content) 
        VALUES (%d, %d, 0, %d, '%s', '%s')
    `, sessionID, epoch, parentID, escapedType, escapedContent)

	_, err := cm.exec.Execute(sql)
	return err
}

// AddToBuffer - добавить в буфер
func (cm *ContextManager) AddToBuffer(sessionID int, key, data string, ttlSeconds int) error {
	if ttlSeconds == 0 {
		ttlSeconds = 300
	}

	epochResult, _ := cm.exec.Execute(fmt.Sprintf(
		"SELECT current_epoch FROM sessions WHERE id = %d", sessionID))

	epoch := 1
	if text := cm.getQueryResultText(epochResult); text != "" {
		if e, err := strconv.Atoi(strings.TrimSpace(text)); err == nil {
			epoch = e
		}
	}

	now := time.Now().Unix()
	escapedKey := escapeSQL(key)
	escapedData := escapeSQL(data)

	sql := fmt.Sprintf(`
        INSERT INTO buffer_space (session_id, epoch, rolled_back, key, data, ttl, created_at) 
        VALUES (%d, %d, 0, '%s', '%s', %d, %d)
    `, sessionID, epoch, escapedKey, escapedData, ttlSeconds, now)

	_, err := cm.exec.Execute(sql)
	return err
}

// AddInference - добавить вывод
func (cm *ContextManager) AddInference(sessionID int, conclusion string, confidence float64, inferenceType string) error {
	if inferenceType == "" {
		inferenceType = "assumption"
	}

	escapedConclusion := escapeSQL(conclusion)

	sql := fmt.Sprintf(`
        INSERT INTO inference_space (session_id, epoch, rolled_back, conclusion, confidence, inference_type) 
        VALUES (%d, 1, 0, '%s', %f, '%s')
    `, sessionID, escapedConclusion, confidence, inferenceType)

	_, err := cm.exec.Execute(sql)
	return err
}

// GetContextForLLM - собрать контекст
func (cm *ContextManager) GetContextForLLM(sessionID int) string {
	var sb strings.Builder

	metaspace, _ := cm.exec.Execute(
		"SELECT content FROM metaspace WHERE is_active = 1 ORDER BY priority DESC LIMIT 10")
	if text := cm.getQueryResultText(metaspace); text != "" {
		sb.WriteString("=== KNOWLEDGE BASE ===\n")
		sb.WriteString(text)
		sb.WriteString("\n\n")
	}

	instructions, _ := cm.exec.Execute(fmt.Sprintf(
		"SELECT content FROM instruction_stack WHERE session_id = %d AND rolled_back = 0 ORDER BY depth", sessionID))
	if text := cm.getQueryResultText(instructions); text != "" {
		sb.WriteString("=== CURRENT TASK ===\n")
		sb.WriteString(text)
		sb.WriteString("\n\n")
	}

	return sb.String()
}

// Cleanup - очистка
func (cm *ContextManager) Cleanup(sessionID int) {
	now := time.Now().Unix()
	cm.exec.Execute(fmt.Sprintf(
		"DELETE FROM buffer_space WHERE session_id = %d AND created_at + ttl < %d",
		sessionID, now))
}

// Rollback - откат
func (cm *ContextManager) Rollback(sessionID int, steps int) error {
	cm.exec.Execute(fmt.Sprintf(
		"UPDATE reasoning_space SET rolled_back = 1 WHERE session_id = %d", sessionID))
	cm.exec.Execute(fmt.Sprintf(
		"UPDATE buffer_space SET rolled_back = 1 WHERE session_id = %d", sessionID))
	return nil
}
