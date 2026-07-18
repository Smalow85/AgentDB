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

	url := strings.TrimRight(c.BaseURL, "/") + "/v1/chat/completions"
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
				fmt.Printf("[LLM_CHATSTREAM] Incoming delta ToolCall: ID=%s, Type=%s, Function.Name='%s', Function.Arguments='%s'\n", tc.ID, tc.Type, tc.Function.Name, tc.Function.Arguments)

				// Проверяем, есть ли уже currentToolCall для обработки и соответствует ли он ID этого чанка
				if currentToolCall == nil {
					// Если currentToolCall не существует, создаём новый ТОЛЬКО если tc.ID не пустой
					if tc.ID != "" {
						currentToolCall = &ToolCall{
							ID:   tc.ID,   // Берем ID из чанка
							Type: tc.Type, // Берем Type из чанка
							Function: ToolCallFunction{
								Name: tc.Function.Name, // Берем Name из чанка (может быть пустым)
							},
						}
						// Arguments добавляем, если есть
						if tc.Function.Arguments != "" {
							currentToolCall.Function.Arguments += tc.Function.Arguments
						}
					} else {
						// Если tc.ID пустой, а currentToolCall не существует, это ошибка или неожиданный формат
						fmt.Printf("[LLM_CHATSTREAM] ERROR: Received tool call chunk without ID and no active currentToolCall.\n")
						// Можно пропустить или обработать особым образом
						continue
					}
				} else {
					// currentToolCall существует
					if tc.ID != "" && currentToolCall.ID != tc.ID {
						// Если пришел чанк с НОВЫМ ID, а старый не был завершен, это ошибка или параллельный вызов
						// Для простоты текущей реализации предполагаем, что в одном delta приходят чанки только для одного инструмента за раз
						// или что чанки строго последовательны для одного вызова.
						// Если tc.ID пустой, это продолжение текущего вызова.
						// Если tc.ID не пустой и не совпадает, это ошибка.
						fmt.Printf("[LLM_CHATSTREAM] ERROR: Received chunk for different ToolCall ID (%s) while processing another (%s). Expected same ID or empty ID.\n", tc.ID, currentToolCall.ID)
						// Попробуем завершить старый, если он был готов, и начать новый (хотя это спорное поведение)
						if currentToolCall.Function.Arguments != "" && isValidJSON(currentToolCall.Function.Arguments) {
							fmt.Printf("[LLM_CHATSTREAM] Flushing incomplete ToolCall %s before handling new ID %s\n", currentToolCall.ID, tc.ID)
							if onToolCall != nil {
								onToolCall(*currentToolCall)
							}
						}
						// Начинаем новый, игнорируя старый
						currentToolCall = &ToolCall{
							ID:   tc.ID,
							Type: tc.Type,
							Function: ToolCallFunction{
								Name: tc.Function.Name, // Обычно Name будет пустым во втором чанке, но на всякий случай
							},
						}
						if tc.Function.Arguments != "" {
							currentToolCall.Function.Arguments += tc.Function.Arguments
						}
					} else {
						// tc.ID пустой ИЛИ совпадает с currentToolCall.ID - это продолжение текущего вызова
						// Обновляем только те поля, которые пришли в чанке
						// Name обычно приходит в первом чанке, но если вдруг придет и тут, обновим (хотя вряд ли)
						if tc.Function.Name != "" && currentToolCall.Function.Name == "" {
							currentToolCall.Function.Name = tc.Function.Name
						}
						// Arguments добавляем
						if tc.Function.Arguments != "" {
							currentToolCall.Function.Arguments += tc.Function.Arguments
						}
						// Type и ID не должны меняться в рамках одного вызова, но на всякий случай:
						if tc.Type != "" && currentToolCall.Type == "" {
							currentToolCall.Type = tc.Type
						}
						if tc.ID != "" && currentToolCall.ID == "" { // Это маловероятно, но на всякий случай
							currentToolCall.ID = tc.ID
						}
					}
				}

				fmt.Printf("[LLM_CHATSTREAM] After update, currentToolCall ID=%s, Name='%s', Arguments='%s'\n", currentToolCall.ID, currentToolCall.Function.Name, currentToolCall.Function.Arguments)

				// Если аргументы закончены (проверяем по валидному JSON)
				if currentToolCall.Function.Arguments != "" && isValidJSON(currentToolCall.Function.Arguments) {
					fmt.Printf("[LLM_CHATSTREAM] Completed ToolCall before onToolCall: ID=%s, Name='%s', Arguments='%s'\n", currentToolCall.ID, currentToolCall.Function.Name, currentToolCall.Function.Arguments)
					if onToolCall != nil {
						onToolCall(*currentToolCall) // <-- Теперь Name должно быть заполнено
					}
					currentToolCall = nil // Сброс для следующего вызова
				}
			}
		}

	}

	return nil
}

func isValidJSON(s string) bool {
	var js json.RawMessage
	return json.Unmarshal([]byte(s), &js) == nil
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
