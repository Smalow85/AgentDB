package agent

import (
	"fmt"
	"time"

	"agent-db/pkg/executor"
)

// ContextManager управляет контекстом агента через AgentDB
// Реализует паттерн JVM Memory Management:
// - Metaspace: постоянное хранилище инструкций и знаний
// - Eden/Survivor: временный контекст текущей задачи (buffer_space)
// - Old Generation: долгосрочная история (reasoning_space, instruction_stack)
type ContextManager struct {
	exec      *executor.Executor
	sessionID string
	agentID   string
	currentEpoch int
}

// ContextSnapshot — снимок контекста для отката
type ContextSnapshot struct {
	SessionID string
	Epoch     int
	Timestamp time.Time
}

// NewContextManager создаёт менеджер контекста для сессии
func NewContextManager(exec *executor.Executor, sessionID, agentID string) (*ContextManager, error) {
	if exec == nil {
		return nil, fmt.Errorf("executor не инициализирован")
	}

	cm := &ContextManager{
		exec:       exec,
		sessionID:  sessionID,
		agentID:    agentID,
		currentEpoch: 0,
	}

	// Инициализируем сессию если нужно
	cm.initSession()
	
	return cm, nil
}

// initSession создаёт сессию если она не существует
func (cm *ContextManager) initSession() {
	// Проверяем существование сессии
	result, err := cm.exec.Execute(fmt.Sprintf("SELECT id FROM sessions WHERE id = '%s'", cm.sessionID))
	if err != nil || result == "" {
		// Создаём новую сессию
		insertSQL := fmt.Sprintf(
			"INSERT INTO sessions (id, name, created_at) VALUES ('%s', 'Session %s', %d)",
			cm.sessionID, cm.sessionID, time.Now().Unix(),
		)
		cm.exec.Execute(insertSQL)
		
		// Инициализируем epoch
		cm.exec.Execute(fmt.Sprintf(
			"UPDATE sessions SET current_epoch = 0 WHERE id = '%s'",
			cm.sessionID,
		))
	}
}

// LoadMetaspace загружает постоянные знания из Metaspace
func (cm *ContextManager) LoadMetaspace() ([]string, error) {
	result, err := cm.exec.Execute(fmt.Sprintf(
		"SELECT content FROM metaspace WHERE agent_id = '%s' AND is_active = 1 ORDER BY priority DESC",
		cm.agentID,
	))
	if err != nil {
		return nil, err
	}
	
	// Парсим результат (каждая строка - отдельный элемент знаний)
	var knowledge []string
	// Упрощённый парсинг - в реальности нужно лучше обрабатывать формат
	if result != "" {
		knowledge = append(knowledge, result)
	}
	
	return knowledge, nil
}

// AddToMetaspace добавляет знание в постоянное хранилище
func (cm *ContextManager) AddToMetaspace(contentType, content string, priority int) error {
	sql := fmt.Sprintf(
		"INSERT INTO metaspace (agent_id, version, content_type, content, priority) VALUES ('%s', 1, '%s', '%s', %d)",
		cm.agentID, contentType, escapeSQL(content), priority,
	)
	_, err := cm.exec.Execute(sql)
	return err
}

// PushInstruction добавляет инструкцию в стек
func (cm *ContextManager) PushInstruction(content string, parentID int) (int, error) {
	// Увеличиваем эпоху
	cm.incrementEpoch()
	
	sql := fmt.Sprintf(
		"INSERT INTO instruction_stack (session_id, epoch, parent_id, content) VALUES ('%s', %d, %d, '%s')",
		cm.sessionID, cm.currentEpoch, parentID, escapeSQL(content),
	)
	_, err := cm.exec.Execute(sql)
	if err != nil {
		return 0, err
	}
	
	// В реальности нужно получить ID вставленной записи
	// Для упрощения возвращаем эпоху как идентификатор
	return cm.currentEpoch, nil
}

// AddThought добавляет мысль в reasoning space
func (cm *ContextManager) AddThought(thoughtType, content string, parentThoughtID int) (int, error) {
	cm.incrementEpoch()
	
	sql := fmt.Sprintf(
		"INSERT INTO reasoning_space (session_id, epoch, parent_thought_id, thought_type, content) VALUES ('%s', %d, %d, '%s', '%s')",
		cm.sessionID, cm.currentEpoch, parentThoughtID, thoughtType, escapeSQL(content),
	)
	_, err := cm.exec.Execute(sql)
	if err != nil {
		return 0, err
	}
	
	return cm.currentEpoch, nil
}

// AddToBuffer добавляет временные данные в буфер
func (cm *ContextManager) AddToBuffer(key, data string, ttlSeconds int) error {
	if ttlSeconds == 0 {
		ttlSeconds = 300 // 5 минут по умолчанию
	}
	
	cm.incrementEpoch()
	
	sql := fmt.Sprintf(
		"INSERT INTO buffer_space (session_id, epoch, key, data, ttl, created_at) VALUES ('%s', %d, '%s', '%s', %d, %d)",
		cm.sessionID, cm.currentEpoch, key, escapeSQL(data), ttlSeconds, time.Now().Unix(),
	)
	_, err := cm.exec.Execute(sql)
	return err
}

// AddInference добавляет вывод с уровнем уверенности
func (cm *ContextManager) AddInference(conclusion string, confidence float64, inferenceType string) error {
	if inferenceType == "" {
		inferenceType = "assumption"
	}
	
	cm.incrementEpoch()
	
	sql := fmt.Sprintf(
		"INSERT INTO inference_space (session_id, epoch, conclusion, confidence, inference_type) VALUES ('%s', %d, '%s', %f, '%s')",
		cm.sessionID, cm.currentEpoch, escapeSQL(conclusion), confidence, inferenceType,
	)
	_, err := cm.exec.Execute(sql)
	return err
}

// GetCurrentContext получает полный контекст для передачи LLM
func (cm *ContextManager) GetCurrentContext() (*ContextView, error) {
	// Metaspace - постоянные знания
	metaspace, _ := cm.exec.Execute(
		"SELECT content FROM metaspace WHERE is_active = 1 ORDER BY priority DESC",
	)
	
	// Инструкции - активные инструкции
	instructions, _ := cm.exec.Execute(fmt.Sprintf(
		"SELECT content FROM instruction_stack WHERE session_id = '%s' AND rolled_back = 0 ORDER BY depth",
		cm.sessionID,
	))
	
	// Мысли - последние 10 мыслей
	thoughts, _ := cm.exec.Execute(fmt.Sprintf(
		"SELECT thought_type || ': ' || content FROM reasoning_space WHERE session_id = '%s' AND rolled_back = 0 ORDER BY epoch DESC LIMIT 10",
		cm.sessionID,
	))
	
	// Буфер - активные временные данные
	buffer, _ := cm.exec.Execute(fmt.Sprintf(
		"SELECT key || ': ' || data FROM buffer_space WHERE session_id = '%s' AND rolled_back = 0",
		cm.sessionID,
	))
	
	// Выводы - активные инференсы
	inferences, _ := cm.exec.Execute(fmt.Sprintf(
		"SELECT conclusion || ' (confidence: ' || CAST(confidence AS TEXT) || ')' FROM inference_space WHERE session_id = '%s' AND rolled_back = 0",
		cm.sessionID,
	))
	
	return &ContextView{
		Metaspace:    metaspace,
		Instructions: instructions,
		Thoughts:     thoughts,
		Buffer:       buffer,
		Inferences:   inferences,
		Epoch:        cm.currentEpoch,
	}, nil
}

// ContextView — представление контекста для LLM
type ContextView struct {
	Metaspace    string
	Instructions string
	Thoughts     string
	Buffer       string
	Inferences   string
	Epoch        int
}

// Rollback откатывает контекст на N шагов назад
func (cm *ContextManager) Rollback(steps int) error {
	if steps <= 0 {
		return fmt.Errorf("шаги должны быть положительными")
	}
	
	targetEpoch := fmt.Sprintf("(SELECT MAX(epoch) - %d FROM reasoning_space WHERE session_id = '%s')", steps, cm.sessionID)
	
	// Отмечаем записи как откаченные
	tables := []string{"reasoning_space", "buffer_space", "tool_calls", "inference_space"}
	for _, table := range tables {
		sql := fmt.Sprintf(
			"UPDATE %s SET rolled_back = 1 WHERE session_id = '%s' AND epoch > %s",
			table, cm.sessionID, targetEpoch,
		)
		cm.exec.Execute(sql)
	}
	
	// Обновляем текущую эпоху
	cm.currentEpoch -= steps
	if cm.currentEpoch < 0 {
		cm.currentEpoch = 0
	}
	
	return nil
}

// GC запускает сборщик мусора
func (cm *ContextManager) GC(gcType string) error {
	switch gcType {
	case "minor":
		// Очистка буфера по TTL
		cm.exec.Execute(fmt.Sprintf(
			"DELETE FROM buffer_space WHERE session_id = '%s' AND created_at < %d - ttl",
			cm.sessionID, time.Now().Unix(),
		))
		
	case "major":
		// Сжатие старых мыслей
		cm.exec.Execute(fmt.Sprintf(
			"UPDATE reasoning_space SET content = '[compressed] ' || substr(content, 1, 100) WHERE session_id = '%s' AND epoch < (SELECT MAX(epoch) - 50 FROM reasoning_space WHERE session_id = '%s')",
			cm.sessionID, cm.sessionID,
		))
		// Удаление откаченного буфера
		cm.exec.Execute(fmt.Sprintf(
			"DELETE FROM buffer_space WHERE session_id = '%s' AND rolled_back = 1",
			cm.sessionID,
		))
		
	case "full":
		// Полная очистка откаченных записей
		tables := []string{"buffer_space", "reasoning_space", "tool_calls", "inference_space"}
		for _, table := range tables {
			cm.exec.Execute(fmt.Sprintf(
				"DELETE FROM %s WHERE session_id = '%s' AND rolled_back = 1",
				table, cm.sessionID,
			))
		}
	}
	
	return nil
}

// CreateSnapshot создаёт точку восстановления
func (cm *ContextManager) CreateSnapshot() (*ContextSnapshot, error) {
	snapshot := &ContextSnapshot{
		SessionID: cm.sessionID,
		Epoch:     cm.currentEpoch,
		Timestamp: time.Now(),
	}
	
	// В будущем можно сохранять состояние в отдельную таблицу
	
	return snapshot, nil
}

// Restore восстанавливает контекст из снимка
func (cm *ContextManager) Restore(snapshot *ContextSnapshot) error {
	if snapshot == nil {
		return fmt.Errorf("снимок не предоставлен")
	}
	
	// Откатываем до эпохи снимка
	steps := cm.currentEpoch - snapshot.Epoch
	if steps > 0 {
		return cm.Rollback(steps)
	}
	
	cm.currentEpoch = snapshot.Epoch
	return nil
}

// incrementEpoch увеличивает счётчик эпох
func (cm *ContextManager) incrementEpoch() {
	cm.currentEpoch++
	cm.exec.Execute(fmt.Sprintf(
		"UPDATE sessions SET current_epoch = current_epoch + 1 WHERE id = '%s'",
		cm.sessionID,
	))
}

// GetSessionID возвращает ID сессии
func (cm *ContextManager) GetSessionID() string {
	return cm.sessionID
}

// GetEpoch возвращает текущую эпоху
func (cm *ContextManager) GetEpoch() int {
	return cm.currentEpoch
}

// escapeSQL экранирует специальные символы в SQL-строках
func escapeSQL(s string) string {
	var result []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\'' {
			result = append(result, '\'', '\'')
		} else {
			result = append(result, c)
		}
	}
	return string(result)
}
