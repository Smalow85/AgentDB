package agent

import "fmt"

func (a *AgentLoop) Run(userMessage string) (string, error) {
	llm := NewLLMClient(a.LLMKey, a.Model, a.BaseURL)
	executor := &ToolExecutor{PSIGraph: a.PSIGraph}

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