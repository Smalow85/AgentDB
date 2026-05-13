package agent

func SystemPrompt() string {
    return `Ты — AI-агент с доступом к инструментам для работы с кодом.

Доступные инструменты:
- read_file(path) — прочитать содержимое файла
- write_file(path, content) — создать или перезаписать файл
- edit_file(path, old_string, new_string) — заменить строку в файле
- delete_file(path) — удалить файл
- list_dir(path) — показать содержимое директории
- run_command(command) — выполнить shell-команду
- get_class(name) — показать методы класса (из PSI-графа)
- find_callers(function) — найти кто вызывает функцию (из PSI-графа)
- find_callees(function) — найти кого вызывает функция (из PSI-графа)

Используй инструменты когда это необходимо. Отвечай кратко и по делу.`
}

func AvailableTools() []Tool {
    return []Tool{
        {
            Type: "function",
            Function: ToolFunction{
                Name:        "read_file",
                Description: "Прочитать содержимое файла",
                Parameters: map[string]interface{}{
                    "type":       "object",
                    "properties": map[string]interface{}{"path": map[string]string{"type": "string"}},
                    "required":   []string{"path"},
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
                    "type":       "object",
                    "properties": map[string]interface{}{"path": map[string]string{"type": "string"}},
                    "required":   []string{"path"},
                },
            },
        },
        {
            Type: "function",
            Function: ToolFunction{
                Name:        "list_dir",
                Description: "Показать содержимое директории",
                Parameters: map[string]interface{}{
                    "type":       "object",
                    "properties": map[string]interface{}{"path": map[string]string{"type": "string"}},
                    "required":   []string{"path"},
                },
            },
        },
        {
            Type: "function",
            Function: ToolFunction{
                Name:        "run_command",
                Description: "Выполнить shell-команду",
                Parameters: map[string]interface{}{
                    "type":       "object",
                    "properties": map[string]interface{}{"command": map[string]string{"type": "string"}},
                    "required":   []string{"command"},
                },
            },
        },
        {
            Type: "function",
            Function: ToolFunction{
                Name:        "search_code",
                Description: "Поиск строки по файлам (grep)",
                Parameters: map[string]interface{}{
                    "type":       "object",
                    "properties": map[string]interface{}{"pattern": map[string]string{"type": "string"}},
                    "required":   []string{"pattern"},
                },
            },
        },
        {
            Type: "function",
            Function: ToolFunction{
                Name:        "get_class",
                Description: "Показать методы класса из PSI-графа",
                Parameters: map[string]interface{}{
                    "type":       "object",
                    "properties": map[string]interface{}{"class": map[string]string{"type": "string"}},
                    "required":   []string{"class"},
                },
            },
        },
        {
            Type: "function",
            Function: ToolFunction{
                Name:        "find_callers",
                Description: "Найти функции которые вызывают указанную",
                Parameters: map[string]interface{}{
                    "type":       "object",
                    "properties": map[string]interface{}{"function": map[string]string{"type": "string"}},
                    "required":   []string{"function"},
                },
            },
        },
        {
            Type: "function",
            Function: ToolFunction{
                Name:        "find_callees",
                Description: "Найти функции которые вызывает указанная",
                Parameters: map[string]interface{}{
                    "type":       "object",
                    "properties": map[string]interface{}{"function": map[string]string{"type": "string"}},
                    "required":   []string{"function"},
                },
            },
        },
        {
            Type: "function",
            Function: ToolFunction{
                Name:        "find_call_path",
                Description: "Найти цепочку вызовов между двумя функциями",
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
    }
}