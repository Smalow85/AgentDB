package agent

import "fmt"

func (a *AgentLoop) Run(userMessage string) (string, []Message, error) {
    llm := NewLLMClient(a.LLMKey, a.Model, a.BaseURL)
    executor := &ToolExecutor{PSIGraph: a.PSIGraph}

    messages := []Message{
        {Role: "system", Content: SystemPrompt()},
        {Role: "user", Content: userMessage},
    }

    var history []Message
    history = append(history, Message{Role: "user", Content: userMessage})

    for iteration := 0; iteration < 30; iteration++ {
        response, err := llm.Chat(messages)
        if err != nil {
            return "", history, err
        }

        msg := response.Choices[0].Message

        // Есть tool_calls — выполняем
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

                history = append(history, Message{
                    Role:    "tool",
                    Content: fmt.Sprintf("%s: %s", tc.Function.Name, result),
                })
            }
            continue
        }

        // Финальный ответ
        history = append(history, Message{
            Role:    "assistant",
            Content: msg.Content,
        })
        return msg.Content, history, nil
    }

    return "", history, fmt.Errorf("превышен лимит итераций (30)")
}