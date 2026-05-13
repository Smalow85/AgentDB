package agent

import (
    "encoding/json"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "strings"

    "agent-db/pkg/graph"
)

type ToolExecutor struct {
    PSIGraph *graph.Graph
}

// ParsedCall — ToolCall с распарсенными аргументами
type ParsedCall struct {
    Name string
    Args map[string]string
}

func parseToolCall(call ToolCall) ParsedCall {
    var args map[string]string
    json.Unmarshal([]byte(call.Function.Arguments), &args)
    if args == nil {
        args = make(map[string]string)
    }
    return ParsedCall{
        Name: call.Function.Name,
        Args: args,
    }
}

func (e *ToolExecutor) Execute(call ToolCall) string {
    pc := parseToolCall(call)
    switch pc.Name {
    case "read_file":
        return e.readFile(pc.Args["path"])
    case "write_file":
        return e.writeFile(pc.Args["path"], pc.Args["content"])
    case "edit_file":
        return e.editFile(pc.Args["path"], pc.Args["old_string"], pc.Args["new_string"])
    case "delete_file":
        return e.deleteFile(pc.Args["path"])
    case "list_dir":
        return e.listDir(pc.Args["path"])
    case "run_command":
        return e.runCommand(pc.Args["command"])
    case "get_class":
        return e.psiGetClass(pc.Args["class"])
    case "find_callers":
        return e.psiFindCallers(pc.Args["function"])
    case "find_callees":
        return e.psiFindCallees(pc.Args["function"])
    default:
        return fmt.Sprintf("Неизвестный инструмент: %s", pc.Name)
    }
}

func (e *ToolExecutor) readFile(path string) string {
    content, err := os.ReadFile(path)
    if err != nil {
        return fmt.Sprintf("Ошибка чтения %s: %v", path, err)
    }
    return string(content)
}

func (e *ToolExecutor) writeFile(path, content string) string {
    os.MkdirAll(filepath.Dir(path), 0755)
    if err := os.WriteFile(path, []byte(content), 0644); err != nil {
        return fmt.Sprintf("Ошибка записи: %v", err)
    }
    return fmt.Sprintf("✓ Файл %s записан (%d байт)", path, len(content))
}

func (e *ToolExecutor) editFile(path, oldStr, newStr string) string {
    content, err := os.ReadFile(path)
    if err != nil {
        return fmt.Sprintf("Ошибка чтения: %v", err)
    }
    updated := strings.Replace(string(content), oldStr, newStr, 1)
    if updated == string(content) {
        return "Ошибка: строка не найдена"
    }
    os.WriteFile(path, []byte(updated), 0644)
    return fmt.Sprintf("✓ Файл %s обновлён", path)
}

func (e *ToolExecutor) deleteFile(path string) string {
    os.Remove(path)
    return fmt.Sprintf("✓ Файл %s удалён", path)
}

func (e *ToolExecutor) listDir(path string) string {
    entries, err := os.ReadDir(path)
    if err != nil {
        return fmt.Sprintf("Ошибка: %v", err)
    }
    var result strings.Builder
    for _, entry := range entries {
        if entry.IsDir() {
            result.WriteString(fmt.Sprintf("📁 %s/\n", entry.Name()))
        } else {
            info, _ := entry.Info()
            result.WriteString(fmt.Sprintf("📄 %s (%d байт)\n", entry.Name(), info.Size()))
        }
    }
    return result.String()
}

func (e *ToolExecutor) runCommand(command string) string {
    cmd := exec.Command("bash", "-c", command)
    output, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Sprintf("Ошибка: %v\n%s", err, string(output))
    }
    return string(output)
}

func (e *ToolExecutor) psiGetClass(className string) string {
    nodes := e.PSIGraph.FindNodes(graph.Query{Label: "class", Property: "name", Value: className})
    if len(nodes) == 0 {
        return fmt.Sprintf("Класс '%s' не найден", className)
    }
    var result strings.Builder
    result.WriteString(fmt.Sprintf("Класс '%s':\n", className))
    children := e.PSIGraph.GetNeighbors(nodes[0].ID, graph.DirectionOutgoing)
    for _, child := range children {
        if child.HasLabel("function") {
            name, _ := child.GetProp("name")
            result.WriteString(fmt.Sprintf("  - %v\n", name))
        }
    }
    return result.String()
}

func (e *ToolExecutor) psiFindCallers(funcName string) string {
    callers := e.PSIGraph.GetCallersByName(funcName)
    if len(callers) == 0 {
        return fmt.Sprintf("Функция '%s' никем не вызывается", funcName)
    }
    return fmt.Sprintf("Вызывают '%s': %s", funcName, strings.Join(callers, ", "))
}

func (e *ToolExecutor) psiFindCallees(funcName string) string {
    callees := e.PSIGraph.GetCalleesByName(funcName)
    if len(callees) == 0 {
        return fmt.Sprintf("Функция '%s' никого не вызывает", funcName)
    }
    return fmt.Sprintf("'%s' вызывает: %s", funcName, strings.Join(callees, ", "))
}