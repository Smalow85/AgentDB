package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type LLMClient struct {
	APIKey  string
	Model   string
	BaseURL string // "https://api.openai.com/v1"
}

func NewLLMClient(apiKey, model, baseURL string) *LLMClient {
	return &LLMClient{
		APIKey:  apiKey,
		Model:   model,
		BaseURL: baseURL,
	}
}

func (c *LLMClient) Chat(messages []Message) (*OpenAIChatResponse, error) {
	req := OpenAIChatRequest{
		Model:    c.Model,
		Messages: messages,
		Tools:    AvailableTools(),
	}

	body, _ := json.Marshal(req)

	url := c.BaseURL + "/chat/completions"
	httpReq, _ := http.NewRequest("POST", url, bytes.NewBuffer(body))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var chatResp OpenAIChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("ошибка парсинга ответа: %w\n%s", err, string(respBody))
	}

	return &chatResp, nil
}