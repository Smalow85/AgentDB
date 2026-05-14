package agent

func SystemPrompt() string {
	return `Ты — AI-агент для работы с кодом. Ты можешь использовать следующие инструменты:

**Инструменты для работы с файлами:**
1. read_file(path) — прочитать содержимое файла
2. write_file(path, content) — создать или перезаписать файл
3. edit_file(path, old_string, new_string) — заменить строку в файле
4. delete_file(path) — удалить файл
5. list_dir(path) — показать содержимое директории
6. run_command(command) — выполнить shell-команду

**Инструменты PSI (анализ кода):**
7. find_callers(function) — найти функции, которые вызывают указанную
8. find_callees(function) — найти функции, которые вызывает указанная
9. get_class(class) — показать методы класса
10. find_call_path(from, to) — найти цепочку вызовов между функциями
11. search_code(pattern) — поиск строки по файлам (grep)

**Инструменты управления контекстом (памятью):**
12. context_snapshot() — создать точку восстановления состояния
13. context_restore(epoch) — восстановить состояние до указанной эпохи
14. context_rollback(steps) — откатиться на N шагов назад
15. context_gc(type) — запустить сборщик мусора (minor/major/full)
16. add_thought(type, content) — добавить мысль в reasoning space
17. add_to_buffer(key, data, ttl) — сохранить временные данные в буфер
18. add_inference(conclusion, confidence, type) — добавить вывод с уверенностью

**Инструменты кодогенерации:**
19. generate_code(target, description, context) — создать запрос на генерацию кода
20. validate_syntax(path) — проверить синтаксис файла
21. search_context(query, language) — поиск релевантного контекста в PSI графе

**Стратегия работы с памятью:**
- Используй context_snapshot перед рискованными изменениями
- Добавляй мысли через add_thought для отслеживания хода рассуждений
- Сохраняй промежуточные результаты в буфер через add_to_buffer
- При ошибке используй context_rollback для отката
- Периодически запускай context_gc("minor") для очистки памяти

**Стратегия кодогенерации:**
- Перед генерацией используй search_context для поиска похожих паттернов
- Сохраняй запрос через generate_code с подробным описанием
- После генерации проверяй синтаксис через validate_syntax
- При необходимости делай rollback для отката неудачных изменений

Если тебе нужно выполнить действие — используй вызов инструмента.
Если хочешь дать финальный ответ — пиши обычный текст.`
}

func AvailableTools() []Tool {
	return []Tool{
		// Инструменты для работы с файлами
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "read_file",
				Description: "Прочитать содержимое файла",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]string{"type": "string"},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "write_file",
				Description: "Создать или перезаписать файл",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path":    map[string]string{"type": "string"},
						"content": map[string]string{"type": "string"},
					},
					"required": []string{"path", "content"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "edit_file",
				Description: "Заменить фрагмент в файле",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path":       map[string]string{"type": "string"},
						"old_string": map[string]string{"type": "string"},
						"new_string": map[string]string{"type": "string"},
					},
					"required": []string{"path", "old_string", "new_string"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "delete_file",
				Description: "Удалить файл",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]string{"type": "string"},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "list_dir",
				Description: "Показать содержимое директории",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]string{"type": "string"},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "run_command",
				Description: "Выполнить shell-команду",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"command": map[string]string{"type": "string"},
					},
					"required": []string{"command"},
				},
			},
		},
		// PSI инструменты
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "find_callers",
				Description: "Найти функции, которые вызывают указанную",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"function": map[string]string{"type": "string"},
					},
					"required": []string{"function"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "find_callees",
				Description: "Найти функции, которые вызывает указанная",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"function": map[string]string{"type": "string"},
					},
					"required": []string{"function"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "get_class",
				Description: "Показать методы класса",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"class": map[string]string{"type": "string"},
					},
					"required": []string{"class"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "find_call_path",
				Description: "Найти цепочку вызовов между функциями",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"from": map[string]string{"type": "string"},
						"to":   map[string]string{"type": "string"},
					},
					"required": []string{"from", "to"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "search_code",
				Description: "Поиск строки по файлам (grep)",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"pattern": map[string]string{"type": "string"},
					},
					"required": []string{"pattern"},
				},
			},
		},
		// Инструменты управления контекстом
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "context_snapshot",
				Description: "Создать точку восстановления состояния",
				Parameters: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "context_restore",
				Description: "Восстановить состояние до указанной эпохи",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"epoch": map[string]string{"type": "string"},
					},
					"required": []string{"epoch"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "context_rollback",
				Description: "Откатиться на N шагов назад",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"steps": map[string]string{"type": "string"},
					},
					"required": []string{"steps"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "context_gc",
				Description: "Запустить сборщик мусора (minor/major/full)",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"type": map[string]string{"type": "string"},
					},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "add_thought",
				Description: "Добавить мысль в reasoning space",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"type":    map[string]string{"type": "string"},
						"content": map[string]string{"type": "string"},
					},
					"required": []string{"type", "content"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "add_to_buffer",
				Description: "Сохранить временные данные в буфер",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"key":  map[string]string{"type": "string"},
						"data": map[string]string{"type": "string"},
						"ttl":  map[string]string{"type": "string"},
					},
					"required": []string{"key", "data"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "add_inference",
				Description: "Добавить вывод с уровнем уверенности",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"conclusion": map[string]string{"type": "string"},
						"confidence": map[string]string{"type": "string"},
						"type":       map[string]string{"type": "string"},
					},
					"required": []string{"conclusion"},
				},
			},
		},
		// Инструменты кодогенерации
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "generate_code",
				Description: "Создать запрос на генерацию кода с описанием и контекстом",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"target":      map[string]string{"type": "string"},
						"description": map[string]string{"type": "string"},
						"context":     map[string]string{"type": "string"},
					},
					"required": []string{"target", "description"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "validate_syntax",
				Description: "Проверить синтаксис файла (для Go использует gofmt)",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]string{"type": "string"},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "search_context",
				Description: "Поиск релевантного контекста в PSI графе по запросу",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query":    map[string]string{"type": "string"},
						"language": map[string]string{"type": "string"},
					},
					"required": []string{"query"},
				},
			},
		},
	}
}