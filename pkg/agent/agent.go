// pkg/agent/agent.go
package agent

import (
    "fmt"
    "log"
    "strings"
    "encoding/json"
    
    "agent-db/pkg/context"
    "agent-db/pkg/graph"
)

type AgentLoop struct {
    SessionID    string
    PSIGraph     *graph.Graph
    LLMKey       string
    Model        string
    BaseURL      string
    ContextMgr   *contextmgr.ContextManager
}

type RunResult struct {
    FinalAnswer string
    Messages    []Message
    ToolsCalled int
    Iterations  int
}

type StreamEvent struct {
    Type    string `json:"type"`    // "chunk", "tool_start", "tool_result", "done", "error"
    Content string `json:"content"`
    Tool    string `json:"tool,omitempty"`
}

func (a *AgentLoop) RunStream(
    userMessage string,
    onEvent func(StreamEvent),
) error {
    log.Println("[AGENT] RunStream called")
    log.Printf("[AGENT] userMessage: %s", userMessage)
    sessionID := parseIntSafe(a.SessionID)
    
    // 1. Получаем или создаём сессию
    session, err := a.ContextMgr.GetOrCreateSession(sessionID)
    if err != nil {
        onEvent(StreamEvent{Type: "error", Content: err.Error()})
        return err
    }
    _ = session
    
    // 2. Сохраняем инструкцию пользователя
    a.ContextMgr.PushInstruction(sessionID, userMessage, 0)
    a.ContextMgr.AddThought(sessionID, "observation", 
        fmt.Sprintf("Получена инструкция: %s", truncate(userMessage, 100)), 0)
    
    // 3. Собираем контекст из памяти
    historicalContext := a.ContextMgr.GetContextForLLM(sessionID)
    psiContext := a.buildPSIContext()
    
    // 4. Создаём LLM-клиента
    llmClient := NewLLMClient(a.LLMKey, a.Model, a.BaseURL)
    executor := &ToolExecutor{PSIGraph: a.PSIGraph}
    
    // 5. Формируем начальные сообщения
    messages := []Message{
        {Role: "system", Content: SystemPrompt()},
        {Role: "user", Content: a.buildPrompt(historicalContext, psiContext, userMessage)},
    }
    
    // 6. Основной цикл агента
    maxIterations := 10
    var fullResponse strings.Builder
    
    for iteration := 0; iteration < maxIterations; iteration++ {
        a.ContextMgr.AddThought(sessionID, "analysis", 
            fmt.Sprintf("Итерация %d: стриминг ответа", iteration+1), 0)
        
        // Используем стриминг
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
            onEvent(StreamEvent{Type: "error", Content: err.Error()})
            return err
        }
        
        // Если нет вызовов инструментов — завершаем
        if len(currentToolCalls) == 0 {
            onEvent(StreamEvent{Type: "done", Content: ""})
            break
        }
        
        // Сохраняем assistant сообщение с тулколлами
        // Важно: включаем Content: "" чтобы провайдеры вроде DeepInfra корректно обрабатывали tool_calls
        messages = append(messages, Message{
            Role:      "assistant",
            Content:   "",
            ToolCalls: currentToolCalls,
        })
        
        // Выполняем инструменты и отправляем результаты
        for _, toolCall := range currentToolCalls {
            result := executor.Execute(toolCall)
            
            // Сохраняем в контекст
            a.ContextMgr.AddToBuffer(sessionID, toolCall.Function.Name, result, 300)
            a.ContextMgr.AddThought(sessionID, "observation",
                truncate(result, 500), 0)
            
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
        
        // Продолжаем цикл — LLM получит результаты тулколлов и сгенерирует финальный ответ
        onEvent(StreamEvent{
            Type:    "chunk",
            Content: "\n💭 Продолжаю анализ...\n\n",
        })
    }
    
    // Сохраняем финальный ответ
    if fullResponse.Len() > 0 {
        a.ContextMgr.AddThought(sessionID, "conclusion", truncate(fullResponse.String(), 500), 0)
        a.ContextMgr.AddInference(sessionID, fullResponse.String(), 0.9, "fact")
    }
    
    onEvent(StreamEvent{Type: "done", Content: ""})
    return nil
}

func (a *AgentLoop) Run(userMessage string) (string, []Message, error) {
    sessionID := parseIntSafe(a.SessionID)
    
    // 1. Получаем или создаём сессию
    session, err := a.ContextMgr.GetOrCreateSession(sessionID)
    if err != nil {
        return "", nil, fmt.Errorf("session error: %w", err)
    }
    _ = session // используем позже для эпох
    
    // 2. Сохраняем инструкцию пользователя
    a.ContextMgr.PushInstruction(sessionID, userMessage, 0)
    a.ContextMgr.AddThought(sessionID, "observation", 
        fmt.Sprintf("Получена инструкция: %s", truncate(userMessage, 100)), 0)
    
    // 3. Собираем контекст из памяти
    historicalContext := a.ContextMgr.GetContextForLLM(sessionID)
    psiContext := a.buildPSIContext()
    
    // 4. Создаём LLM-клиента
    llmClient := NewLLMClient(a.LLMKey, a.Model, a.BaseURL)
    executor := &ToolExecutor{PSIGraph: a.PSIGraph}
    
    // 5. Формируем начальные сообщения
    messages := []Message{
        {Role: "system", Content: SystemPrompt()},
        {Role: "user", Content: a.buildPrompt(historicalContext, psiContext, userMessage)},
    }
    
    // 6. Основной цикл агента
    maxIterations := 10
    allMessages := []Message{} // для ответа API
    var finalAnswer string
    
    for iteration := 0; iteration < maxIterations; iteration++ {
        // Сохраняем мысль о начале итерации
        a.ContextMgr.AddThought(sessionID, "analysis", 
            fmt.Sprintf("Итерация %d: отправляем запрос в LLM", iteration+1), 0)
        
        // Запрашиваем LLM
        chatResp, err := llmClient.Chat(messages)
        if err != nil {
            a.ContextMgr.AddThought(sessionID, "error", fmt.Sprintf("LLM ошибка: %v", err), 0)
            return "", nil, err
        }
        
        if len(chatResp.Choices) == 0 {
            err := fmt.Errorf("пустой ответ от LLM")
            a.ContextMgr.AddThought(sessionID, "error", err.Error(), 0)
            return "", nil, err
        }
        
        assistantMsg := chatResp.Choices[0].Message
        
        // Сохраняем assistant сообщение в историю
        messages = append(messages, Message{
            Role:    "assistant",
            Content: assistantMsg.Content,
            ToolCalls: assistantMsg.ToolCalls,
        })
        
        // Сохраняем для ответа API
        allMessages = append(allMessages, Message{
            Role:    "assistant", 
            Content: assistantMsg.Content,
        })
        
        // Проверяем, есть ли вызовы инструментов
        if len(assistantMsg.ToolCalls) == 0 {
            // Нет инструментов — это финальный ответ
            finalAnswer = assistantMsg.Content
            a.ContextMgr.AddThought(sessionID, "conclusion", 
                fmt.Sprintf("Финальный ответ: %s", truncate(finalAnswer, 200)), 0)
            a.ContextMgr.AddInference(sessionID, finalAnswer, 0.9, "fact")
            break
        }
        
        // Обрабатываем вызовы инструментов
        a.ContextMgr.AddThought(sessionID, "action", 
            fmt.Sprintf("Вызываем %d инструментов", len(assistantMsg.ToolCalls)), 0)
        
        for _, toolCall := range assistantMsg.ToolCalls {
            // Сохраняем вызов в буфер
            toolResult := executor.Execute(toolCall)
            
            // Сохраняем результат в контекст-менеджер
            a.ContextMgr.AddToBuffer(sessionID, toolCall.Function.Name, toolResult, 300)
            a.ContextMgr.AddThought(sessionID, "observation",
                fmt.Sprintf("Результат %s: %s", toolCall.Function.Name, truncate(toolResult, 200)), 0)
            
            // Добавляем результат в историю для следующего запроса к LLM
            messages = append(messages, Message{
                Role:       "tool",
                ToolCallID: toolCall.ID,
                Content:    toolResult,
            })
            
            // Сохраняем для ответа API
            allMessages = append(allMessages, Message{
                Role:       "tool",
                ToolCallID: toolCall.ID,
                Content:    fmt.Sprintf("%s: %s", toolCall.Function.Name, truncate(toolResult, 200)),
            })
        }
        
        // Периодическая очистка
        if iteration > 0 && iteration%5 == 0 {
            a.ContextMgr.Cleanup(sessionID)
        }
    }
    
    if finalAnswer == "" {
        finalAnswer = "Достигнут лимит итераций без получения ответа"
        a.ContextMgr.AddInference(sessionID, finalAnswer, 0.3, "error")
    }
    
    return finalAnswer, allMessages, nil
}

func (a *AgentLoop) buildPrompt(context, psiContext, userMessage string) string {
    return fmt.Sprintf(`=== ТВОЙ КОНТЕКСТ (память, выводы, буфер) ===
%s

=== СТРУКТУРА ПРОЕКТА (PSI-граф) ===
%s

=== ТЕКУЩАЯ ИНСТРУКЦИЯ ===
%s

Важно: используй историю рассуждений и выводы из контекста. Не повторяй уже сделанные выводы.
Если в буфере уже есть нужные данные — используй их, не вызывай инструменты повторно.`,
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