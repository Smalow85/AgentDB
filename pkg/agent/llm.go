package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type LLMClient struct {
	APIKey string
	Model  string
}

func (c *LLMClient) Chat(messages []Message) (*OpenAIChatResponse, error) {
	req := OpenAIChatRequest{
		Model:    c.Model,
		Messages: messages,
		Tools:    AvailableTools(),
	}

	body, _ := json.Marshal(req)

	httpReq, _ := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(body))
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