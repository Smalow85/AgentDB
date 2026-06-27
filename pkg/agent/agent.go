// pkg/agent/agent.go
package agent

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"agent-db/pkg/context"
	"agent-db/pkg/graph"
)

type AgentLoop struct {
	SessionID      string
	PSIGraph       *graph.Graph
	LLMKey         string
	Model          string
	BaseURL        string
	MemoryMgr      *context.MemoryManager // ← новый MemoryManager
	ProjectPath    string
	CurrentVersion int // ← текущая версия кода
}

type RunResult struct {
	FinalAnswer string
	Messages    []Message
	ToolsCalled int
	Iterations  int
}

type StreamEvent struct {
	Type    string `json:"type"` // "chunk", "tool_start", "tool_result", "done", "error"
	Content string `json:"content"`
	Tool    string `json:"tool,omitempty"`
}

func (a *AgentLoop) RunStream(
	userMessage string,
	onEvent func(StreamEvent),
) error {
	log.Println("[AGENT] RunStream called")
	log.Printf("[AGENT] userMessage: %s", userMessage)
	log.Printf("[AGENT] CurrentVersion: %d", a.CurrentVersion)

	if a.MemoryMgr == nil {
		onEvent(StreamEvent{Type: "error", Content: "MemoryManager не инициализирован"})
		return fmt.Errorf("MemoryManager is nil")
	}

	sessionID := parseIntSafe(a.SessionID)

	_, err := a.MemoryMgr.GetOrCreateSession(sessionID)
	if err != nil {
		onEvent(StreamEvent{Type: "error", Content: fmt.Sprintf("Ошибка сессии: %v", err)})
		return err
	}

	// 1. Сохраняем инструкцию с привязкой к версии
	instructionID, err := a.MemoryMgr.PushInstruction(sessionID, a.CurrentVersion, userMessage, 0)
	if err != nil {
		onEvent(StreamEvent{Type: "error", Content: err.Error()})
		return err
	}

	// 2. Добавляем мысль о получении инструкции
	a.MemoryMgr.AddThought(sessionID, a.CurrentVersion, instructionID, "observation",
		fmt.Sprintf("Получена инструкция: %s", truncate(userMessage, 100)), 0.9)

	// 3. Собираем структурированный контекст через LLM Context Engine
	contextEngine := context.NewLLMContextEngine(a.MemoryMgr)
	llmContext := contextEngine.BuildContextForLLM(sessionID, a.CurrentVersion, 4000)

	// 4. Строим PSI контекст
	psiContext := a.buildPSIContext()

	// 5. Создаём LLM-клиента
	llmClient := NewLLMClient(a.LLMKey, a.Model, a.BaseURL)
	executor := &ToolExecutor{
		PSIGraph:    a.PSIGraph,
		ProjectPath: a.ProjectPath,
		MemoryMgr:   a.MemoryMgr,      // ← передаём для логирования
		VersionID:   a.CurrentVersion, // ← передаём версию
	}

	// 6. Формируем начальные сообщения
	messages := []Message{
		{Role: "system", Content: SystemPrompt()},
		{Role: "user", Content: a.buildPrompt(llmContext, psiContext, userMessage)},
	}

	// 7. Основной цикл агента
	maxIterations := 10
	var fullResponse strings.Builder
	var toolsCalled int

	for iteration := 0; iteration < maxIterations; iteration++ {
		a.MemoryMgr.AddThought(sessionID, a.CurrentVersion, instructionID, "analysis",
			fmt.Sprintf("Итерация %d: стриминг ответа", iteration+1), 0.8)

		var currentToolCalls []ToolCall

		err := llmClient.ChatStream(messages,
			// onChunk — обработка текстовых чанков
			func(chunk string) {
				fullResponse.WriteString(chunk)
				onEvent(StreamEvent{
					Type:    "chunk",
					Content: chunk,
				})
			},
			// onToolCall — обработка вызовов инструментов
			func(tool ToolCall) {
				currentToolCalls = append(currentToolCalls, tool)
				toolsCalled++

				// Парсим аргументы для красивого отображения
				var args map[string]interface{}
				json.Unmarshal([]byte(tool.Function.Arguments), &args)

				argsStr := ""
				if args != nil {
					if path, ok := args["path"]; ok {
						argsStr = fmt.Sprintf(" (%v)", path)
					} else if pattern, ok := args["pattern"]; ok {
						argsStr = fmt.Sprintf(" (%v)", pattern)
					} else if command, ok := args["command"]; ok {
						argsStr = fmt.Sprintf(" (%v)", command)
					}
				}

				onEvent(StreamEvent{
					Type:    "tool_start",
					Content: fmt.Sprintf("🔧 Вызываю %s%s...\n", tool.Function.Name, argsStr),
					Tool:    tool.Function.Name,
				})
			},
		)

		if err != nil {
			a.MemoryMgr.AddThought(sessionID, a.CurrentVersion, instructionID, "error",
				fmt.Sprintf("LLM ошибка: %v", err), 0.5)
			onEvent(StreamEvent{Type: "error", Content: err.Error()})
			return err
		}

		// Если нет вызовов инструментов — завершаем
		if len(currentToolCalls) == 0 {
			onEvent(StreamEvent{Type: "done", Content: ""})
			break
		}

		// Сохраняем assistant сообщение с тулколлами
		messages = append(messages, Message{
			Role:      "assistant",
			Content:   "",
			ToolCalls: currentToolCalls,
		})

		// Выполняем инструменты и отправляем результаты
		for _, toolCall := range currentToolCalls {
			result := executor.Execute(toolCall)

			// Сохраняем в буфер
			a.MemoryMgr.SetBuffer(sessionID, a.CurrentVersion, toolCall.Function.Name, result, 300)
			a.MemoryMgr.AddThought(sessionID, a.CurrentVersion, instructionID, "observation",
				truncate(result, 500), 0.7)

			// Отправляем результат клиенту
			onEvent(StreamEvent{
				Type:    "tool_result",
				Content: fmt.Sprintf("📋 Результат:\n%s\n\n", truncate(result, 500)),
				Tool:    toolCall.Function.Name,
			})

			// Добавляем в историю для следующего запроса
			messages = append(messages, Message{
				Role:       "tool",
				ToolCallID: toolCall.ID,
				Content:    result,
			})
		}

		// Продолжаем цикл
		onEvent(StreamEvent{
			Type:    "chunk",
			Content: "\n💭 Продолжаю анализ...\n\n",
		})
	}

	// Сохраняем финальный ответ как вывод
	if fullResponse.Len() > 0 {
		a.MemoryMgr.AddThought(sessionID, a.CurrentVersion, instructionID, "conclusion",
			truncate(fullResponse.String(), 500), 0.9)
		a.MemoryMgr.AddInference(sessionID, a.CurrentVersion, fullResponse.String(), 0.9, "fact", "agent")
	}

	// Завершаем инструкцию
	a.MemoryMgr.PopInstruction(instructionID, true)

	onEvent(StreamEvent{Type: "done", Content: ""})
	return nil
}

func (a *AgentLoop) Run(userMessage string) (string, []Message, error) {
	log.Printf("[AGENT] Run called, CurrentVersion: %d", a.CurrentVersion)

	sessionID := parseIntSafe(a.SessionID)

	// 1. Сохраняем инструкцию
	instructionID, err := a.MemoryMgr.PushInstruction(sessionID, a.CurrentVersion, userMessage, 0)
	if err != nil {
		return "", nil, fmt.Errorf("push instruction error: %w", err)
	}

	// 2. Добавляем мысль
	a.MemoryMgr.AddThought(sessionID, a.CurrentVersion, instructionID, "observation",
		fmt.Sprintf("Получена инструкция: %s", truncate(userMessage, 100)), 0.9)

	// 3. Собираем контекст
	contextEngine := context.NewLLMContextEngine(a.MemoryMgr)
	llmContext := contextEngine.BuildContextForLLM(sessionID, a.CurrentVersion, 4000)
	psiContext := a.buildPSIContext()

	// 4. Создаём LLM-клиента
	llmClient := NewLLMClient(a.LLMKey, a.Model, a.BaseURL)
	executor := &ToolExecutor{
		PSIGraph:    a.PSIGraph,
		ProjectPath: a.ProjectPath,
		MemoryMgr:   a.MemoryMgr,
		VersionID:   a.CurrentVersion,
	}

	// 5. Начальные сообщения
	messages := []Message{
		{Role: "system", Content: SystemPrompt()},
		{Role: "user", Content: a.buildPrompt(llmContext, psiContext, userMessage)},
	}

	// 6. Основной цикл
	maxIterations := 10
	allMessages := []Message{}
	var finalAnswer string
	var toolsCalled int

	for iteration := 0; iteration < maxIterations; iteration++ {
		a.MemoryMgr.AddThought(sessionID, a.CurrentVersion, instructionID, "analysis",
			fmt.Sprintf("Итерация %d: отправляем запрос в LLM", iteration+1), 0.8)

		chatResp, err := llmClient.Chat(messages)
		if err != nil {
			a.MemoryMgr.AddThought(sessionID, a.CurrentVersion, instructionID, "error",
				fmt.Sprintf("LLM ошибка: %v", err), 0.5)
			return "", nil, err
		}

		if len(chatResp.Choices) == 0 {
			err := fmt.Errorf("пустой ответ от LLM")
			a.MemoryMgr.AddThought(sessionID, a.CurrentVersion, instructionID, "error", err.Error(), 0.5)
			return "", nil, err
		}

		assistantMsg := chatResp.Choices[0].Message

		messages = append(messages, Message{
			Role:      "assistant",
			Content:   assistantMsg.Content,
			ToolCalls: assistantMsg.ToolCalls,
		})

		allMessages = append(allMessages, Message{
			Role:    "assistant",
			Content: assistantMsg.Content,
		})

		if len(assistantMsg.ToolCalls) == 0 {
			finalAnswer = assistantMsg.Content
			a.MemoryMgr.AddThought(sessionID, a.CurrentVersion, instructionID, "conclusion",
				fmt.Sprintf("Финальный ответ: %s", truncate(finalAnswer, 200)), 0.9)
			a.MemoryMgr.AddInference(sessionID, a.CurrentVersion, finalAnswer, 0.9, "fact", "agent")
			break
		}

		a.MemoryMgr.AddThought(sessionID, a.CurrentVersion, instructionID, "action",
			fmt.Sprintf("Вызываем %d инструментов", len(assistantMsg.ToolCalls)), 0.8)

		for _, toolCall := range assistantMsg.ToolCalls {
			toolsCalled++
			toolResult := executor.Execute(toolCall)

			a.MemoryMgr.SetBuffer(sessionID, a.CurrentVersion, toolCall.Function.Name, toolResult, 300)
			a.MemoryMgr.AddThought(sessionID, a.CurrentVersion, instructionID, "observation",
				fmt.Sprintf("Результат %s: %s", toolCall.Function.Name, truncate(toolResult, 200)), 0.7)

			messages = append(messages, Message{
				Role:       "tool",
				ToolCallID: toolCall.ID,
				Content:    toolResult,
			})

			allMessages = append(allMessages, Message{
				Role:       "tool",
				ToolCallID: toolCall.ID,
				Content:    fmt.Sprintf("%s: %s", toolCall.Function.Name, truncate(toolResult, 200)),
			})
		}

		if iteration > 0 && iteration%5 == 0 {
			a.MemoryMgr.GC(sessionID, a.CurrentVersion)
		}
	}

	if finalAnswer == "" {
		finalAnswer = "Достигнут лимит итераций без получения ответа"
		a.MemoryMgr.AddInference(sessionID, a.CurrentVersion, finalAnswer, 0.3, "error", "system")
	}

	// Завершаем инструкцию
	a.MemoryMgr.PopInstruction(instructionID, finalAnswer != "")

	return finalAnswer, allMessages, nil
}

func (a *AgentLoop) buildPrompt(context, psiContext, userMessage string) string {
	return fmt.Sprintf(`Ты — AI-агент для анализа кода и выполнения задач.

=== ТВОЙ КОНТЕКСТ (память, выводы, данные) ===
%s

=== СТРУКТУРА ПРОЕКТА ===
%s

=== ТЕКУЩАЯ ЗАДАЧА ===
%s

ПРАВИЛА:
1. Используй доступные инструменты (read_file, write_file, list_dir, get_class, find_callers, find_callees)
2. Не повторяй уже сделанные выводы
3. Если данные уже есть в буфере — используй их
4. Отвечай на русском языке
5. Будь конкретным и полезным`,
		context, psiContext, userMessage)
}

func (a *AgentLoop) buildPSIContext() string {
	if a.PSIGraph == nil {
		return "Граф кода не загружен. Используй инструменты для работы с файловой системой."
	}

	var sb strings.Builder

	classes := a.PSIGraph.FindNodes(graph.Query{Label: "class"})
	functions := a.PSIGraph.FindNodes(graph.Query{Label: "function"})
	calls := a.PSIGraph.FindNodes(graph.Query{Label: "call"})

	sb.WriteString(fmt.Sprintf("📊 Статистика: %d классов, %d функций, %d вызовов\n",
		len(classes), len(functions), len(calls)))

	if len(classes) > 0 {
		sb.WriteString("\n📦 Основные классы:\n")
		for i, class := range classes {
			if i >= 7 {
				sb.WriteString(fmt.Sprintf("  ... и ещё %d классов\n", len(classes)-7))
				break
			}
			name, _ := class.GetProp("name")
			sb.WriteString(fmt.Sprintf("  - %v\n", name))
		}
	}

	if len(functions) > 0 && len(functions) <= 20 {
		sb.WriteString("\n🔧 Функции:\n")
		for i, fn := range functions {
			if i >= 10 {
				break
			}
			name, _ := fn.GetProp("name")
			sb.WriteString(fmt.Sprintf("  - %v\n", name))
		}
	}

	return sb.String()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func parseIntSafe(s string) int {
	var i int
	fmt.Sscanf(s, "%d", &i)
	return i
}
