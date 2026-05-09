package agent

import "agent-db/pkg/graph"

// Message — сообщение в диалоге
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

// ToolCall — вызов инструмента от LLM
type ToolCall struct {
	ID   string            `json:"id"`
	Name string            `json:"name"`
	Args map[string]string `json:"arguments"`
}

// Tool — описание инструмента для LLM
type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// OpenAIChatRequest — запрос к OpenAI API
type OpenAIChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Tools    []Tool    `json:"tools,omitempty"`
}

// OpenAIChatResponse — ответ от OpenAI API
type OpenAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content   string     `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
}

// AgentLoop — основной цикл агента
type AgentLoop struct {
	SessionID string
	PSIGraph  *graph.Graph
	LLMKey    string
	Model     string
	BaseURL   string
}