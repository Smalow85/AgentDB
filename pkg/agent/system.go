package agent

func SystemPrompt() string {
	return `Ты — AI-агент для работы с кодом. Ты можешь использовать следующие инструменты:

1. read_file(path) — прочитать содержимое файла
2. write_file(path, content) — создать или перезаписать файл
3. edit_file(path, old_string, new_string) — заменить строку в файле
4. delete_file(path) — удалить файл
5. list_dir(path) — показать содержимое директории
6. run_command(command) — выполнить shell-команду
7. find_callers(function) — найти функции, которые вызывают указанную
8. find_callees(function) — найти функции, которые вызывает указанная
9. get_class(class) — показать методы класса
10. find_call_path(from, to) — найти цепочку вызовов между функциями
11. search_code(pattern) — поиск строки по файлам (grep)

Если тебе нужно выполнить действие — используй вызов инструмента.
Если хочешь дать финальный ответ — пиши обычный текст.`
}

func AvailableTools() []Tool {
	return []Tool{
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
	}
}