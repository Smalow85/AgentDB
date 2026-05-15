// pkg/agent/types.go
package agent

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
    Arguments string `json:"arguments"`
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
    Stream   bool      `json:"stream,omitempty"`
}

// ChatResponse — ответ от LLM API (не-стриминг)
type ChatResponse struct {
    Choices []struct {
        Message struct {
            Content   string     `json:"content"`
            ToolCalls []ToolCall `json:"tool_calls"`
        } `json:"message"`
    } `json:"choices"`
}

// StreamResponse — ответ от LLM API (стриминг)
type StreamResponse struct {
    Choices []struct {
        Delta struct {
            Content   string     `json:"content"`
            ToolCalls []ToolCall `json:"tool_calls"`
        } `json:"delta"`
    } `json:"choices"`
}