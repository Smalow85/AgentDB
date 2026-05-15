// pkg/agent/llm.go
package agent

import (
    "bufio"
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "strings"
)

type LLMClient struct {
    APIKey  string
    Model   string
    BaseURL string
}

func NewLLMClient(apiKey, model, baseURL string) *LLMClient {
    return &LLMClient{
        APIKey:  apiKey,
        Model:   model,
        BaseURL: baseURL,
    }
}

// ChatStream — стриминговый вызов LLM с поддержкой инструментов
func (c *LLMClient) ChatStream(messages []Message, onChunk func(chunk string), onToolCall func(tool ToolCall)) error {
    req := ChatRequest{
        Model:    c.Model,
        Messages: messages,
        Tools:    AvailableTools(),
        Stream:   true,
    }

    body, err := json.Marshal(req)
    if err != nil {
        return fmt.Errorf("ошибка маршалинга: %w", err)
    }

    url := strings.TrimRight(c.BaseURL, "/") + "/chat/completions"
    httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
    if err != nil {
        return fmt.Errorf("ошибка создания запроса: %w", err)
    }
    
    httpReq.Header.Set("Content-Type", "application/json")
    httpReq.Header.Set("Accept", "text/event-stream")
    if c.APIKey != "" {
        httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
    }

    client := &http.Client{}
    resp, err := client.Do(httpReq)
    if err != nil {
        return fmt.Errorf("сетевая ошибка: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != 200 {
        body, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("LLM ошибка %d: %s", resp.StatusCode, string(body))
    }

    // Парсим SSE поток
    reader := bufio.NewReader(resp.Body)
    var fullContent strings.Builder
    var currentToolCall *ToolCall
    
    for {
        line, err := reader.ReadString('\n')
        if err != nil {
            if err == io.EOF {
                break
            }
            return fmt.Errorf("ошибка чтения потока: %w", err)
        }
        
        line = strings.TrimSpace(line)
        if !strings.HasPrefix(line, "data: ") {
            continue
        }
        
        data := strings.TrimPrefix(line, "data: ")
        if data == "[DONE]" {
            break
        }
        
        var streamResp StreamResponse
        if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
            continue // пропускаем некорректные чанки
        }
        
        if len(streamResp.Choices) == 0 {
            continue
        }
        
        delta := streamResp.Choices[0].Delta
        
        // Обрабатываем текстовый контент
        if delta.Content != "" {
            fullContent.WriteString(delta.Content)
            if onChunk != nil {
                onChunk(delta.Content)
            }
        }
        
        // Обрабатываем вызовы инструментов
        if len(delta.ToolCalls) > 0 {
            for _, tc := range delta.ToolCalls {
                if currentToolCall == nil || currentToolCall.ID != tc.ID {
                    currentToolCall = &ToolCall{
                        ID:       tc.ID,
                        Type:     tc.Type,
                        Function: ToolCallFunction{},
                    }
                }
                // Собираем аргументы по частям
                currentToolCall.Function.Name += tc.Function.Name
                currentToolCall.Function.Arguments += tc.Function.Arguments
                
                // Если аргументы закончены (проверяем по валидному JSON)
                if strings.HasSuffix(currentToolCall.Function.Arguments, "}") {
                    if onToolCall != nil {
                        onToolCall(*currentToolCall)
                    }
                    currentToolCall = nil
                }
            }
        }
    }
    
    return nil
}

// Chat — обычный (не стриминговый) вызов для совместимости
func (c *LLMClient) Chat(messages []Message) (*ChatResponse, error) {
    req := ChatRequest{
        Model:    c.Model,
        Messages: messages,
        Tools:    AvailableTools(),
        Stream:   false,
    }

    body, err := json.Marshal(req)
    if err != nil {
        return nil, fmt.Errorf("ошибка маршалинга: %w", err)
    }

    url := strings.TrimRight(c.BaseURL, "/") + "/chat/completions"
    httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
    if err != nil {
        return nil, fmt.Errorf("ошибка создания запроса: %w", err)
    }
    
    httpReq.Header.Set("Content-Type", "application/json")
    if c.APIKey != "" {
        httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
    }

    client := &http.Client{}
    resp, err := client.Do(httpReq)
    if err != nil {
        return nil, fmt.Errorf("сетевая ошибка: %w", err)
    }
    defer resp.Body.Close()

    respBody, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, fmt.Errorf("ошибка чтения ответа: %w", err)
    }

    if resp.StatusCode != 200 {
        return nil, fmt.Errorf("LLM ошибка %d: %s", resp.StatusCode, string(respBody))
    }

    var chatResp ChatResponse
    if err := json.Unmarshal(respBody, &chatResp); err != nil {
        return nil, fmt.Errorf("ошибка парсинга ответа: %w", err)
    }

    if len(chatResp.Choices) == 0 {
        return nil, fmt.Errorf("пустой ответ от LLM")
    }

    return &chatResp, nil
}