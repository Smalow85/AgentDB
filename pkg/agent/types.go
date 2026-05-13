package agent

import "agent-db/pkg/graph"

// AgentLoop — основной цикл агента
type AgentLoop struct {
    SessionID string
    PSIGraph  *graph.Graph
    LLMKey    string
    Model     string
    BaseURL   string
}

// Message — сообщение в диалоге с LLM
type Message struct {
    Role       string     `json:"role"`
    Content    string     `json:"content,omitempty"`
    ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
    ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolCall — вызов инструмента от LLM
type ToolCall struct {
    ID       string           `json:"id"`
    Type     string           `json:"type,omitempty"`
    Function ToolCallFunction `json:"function"`
}

type ToolCallFunction struct {
    Name      string `json:"name"`
    Arguments string `json:"arguments"` // JSON-строка с параметрами
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

// ChatRequest — запрос к LLM API
type ChatRequest struct {
    Model    string    `json:"model"`
    Messages []Message `json:"messages"`
    Tools    []Tool    `json:"tools,omitempty"`
}

// ChatResponse — ответ от LLM API
type ChatResponse struct {
    Choices []struct {
        Message struct {
            Content   string     `json:"content"`
            ToolCalls []ToolCall `json:"tool_calls"`
        } `json:"message"`
    } `json:"choices"`
}