package agent

import (
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

func (c *LLMClient) Chat(messages []Message) (*ChatResponse, error) {
    req := ChatRequest{
        Model:    c.Model,
        Messages: messages,
        Tools:    AvailableTools(),
    }

    body, _ := json.Marshal(req)

    url := strings.TrimRight(c.BaseURL, "/") + "/chat/completions"
    httpReq, _ := http.NewRequest("POST", url, bytes.NewBuffer(body))
    httpReq.Header.Set("Content-Type", "application/json")
    if c.APIKey != "" {
        httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
    }

    resp, err := http.DefaultClient.Do(httpReq)
    if err != nil {
        return nil, fmt.Errorf("сетевая ошибка: %w", err)
    }
    defer resp.Body.Close()

    respBody, _ := io.ReadAll(resp.Body)

    if resp.StatusCode != 200 {
        return nil, fmt.Errorf("LLM ошибка %d: %s", resp.StatusCode, string(respBody[:300]))
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