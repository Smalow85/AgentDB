package agent

import (
	"fmt"
	"agent-db/pkg/executor"
)

func (a *AgentLoop) Run(userMessage string) (string, error) {
	llm := NewLLMClient(a.LLMKey, a.Model, a.BaseURL)
	
	// Инициализируем Executor для работы с AgentDB
	// В реальном проекте нужно передавать существующий экземпляр
	exec, err := executor.NewExecutor(nil, nil, "agent.db")
	if err != nil {
		return "", fmt.Errorf("ошибка инициализации Executor: %w", err)
	}
	
	// Создаём ContextManager для управления памятью агента
	contextMgr, err := NewContextManager(exec, a.SessionID, "agent-1")
	if err != nil {
		return "", fmt.Errorf("ошибка инициализации ContextManager: %w", err)
	}
	
	// Передаём ContextManager и Executor в ToolExecutor
	executor := &ToolExecutor{
		PSIGraph:       a.PSIGraph,
		ContextManager: contextMgr,
		Executor:       exec,
	}

	messages := []Message{
		{Role: "system", Content: SystemPrompt()},
		{Role: "user", Content: userMessage},
	}

	for iteration := 0; iteration < 30; iteration++ {
		response, err := llm.Chat(messages)
		if err != nil {
			return "", fmt.Errorf("ошибка LLM: %w", err)
		}

		if len(response.Choices) == 0 {
			return "", fmt.Errorf("пустой ответ от LLM")
		}

		msg := response.Choices[0].Message

		if len(msg.ToolCalls) > 0 {
			messages = append(messages, Message{
				Role:      "assistant",
				ToolCalls: msg.ToolCalls,
			})

			for _, tc := range msg.ToolCalls {
				result := executor.Execute(tc)
				messages = append(messages, Message{
					Role:       "tool",
					Content:    result,
					ToolCallID: tc.ID,
				})
			}
			continue
		}

		return msg.Content, nil
	}

	return "", fmt.Errorf("превышен лимит итераций (30)")
}