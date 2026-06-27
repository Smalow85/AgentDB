// pkg/agent/tools.go
package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"agent-db/pkg/context"
	"agent-db/pkg/graph"
)

type ToolExecutor struct {
	PSIGraph    *graph.Graph
	ProjectPath string
	MemoryMgr   *context.MemoryManager
	VersionID   int
}

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

	// Логируем вызов инструмента
	if e.MemoryMgr != nil {
		e.MemoryMgr.AddObservation(0, e.VersionID, pc.Name, fmt.Sprintf("%+v", pc.Args), "", false)
	}

	var result string
	switch pc.Name {
	case "read_file":
		result = e.readFile(pc.Args["path"])
	case "write_file":
		result = e.writeFile(pc.Args["path"], pc.Args["content"])
	case "edit_file":
		result = e.editFile(pc.Args["path"], pc.Args["old_string"], pc.Args["new_string"])
	case "delete_file":
		result = e.deleteFile(pc.Args["path"])
	case "list_dir":
		result = e.listDir(pc.Args["path"])
	case "run_command":
		result = e.runCommand(pc.Args["command"])
	case "get_class":
		result = e.psiGetClass(pc.Args["class"])
	case "find_callers":
		result = e.psiFindCallers(pc.Args["function"])
	case "find_callees":
		result = e.psiFindCallees(pc.Args["function"])
	default:
		result = fmt.Sprintf("Неизвестный инструмент: %s", pc.Name)
	}

	// Обновляем наблюдение с результатом
	if e.MemoryMgr != nil {
		e.MemoryMgr.AddObservation(0, e.VersionID, pc.Name, fmt.Sprintf("%+v", pc.Args), result, true)
	}

	return result
}

func (e *ToolExecutor) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	if e.ProjectPath != "" {
		return filepath.Join(e.ProjectPath, path)
	}
	return path
}

func (e *ToolExecutor) readFile(path string) string {
	fullPath := e.resolvePath(path)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return fmt.Sprintf("Ошибка чтения %s: %v", path, err)
	}
	return string(content)
}

func (e *ToolExecutor) writeFile(path, content string) string {
	fullPath := e.resolvePath(path)
	os.MkdirAll(filepath.Dir(fullPath), 0755)
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return fmt.Sprintf("Ошибка записи: %v", err)
	}
	return fmt.Sprintf("✓ Файл %s записан (%d байт)", fullPath, len(content))
}

func (e *ToolExecutor) editFile(path, oldStr, newStr string) string {
	fullPath := e.resolvePath(path)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return fmt.Sprintf("Ошибка чтения: %v", err)
	}
	updated := strings.Replace(string(content), oldStr, newStr, 1)
	if updated == string(content) {
		return "Ошибка: строка не найдена"
	}
	os.WriteFile(fullPath, []byte(updated), 0644)
	return fmt.Sprintf("✓ Файл %s обновлён", fullPath)
}

func (e *ToolExecutor) deleteFile(path string) string {
	fullPath := e.resolvePath(path)
	os.Remove(fullPath)
	return fmt.Sprintf("✓ Файл %s удалён", fullPath)
}

func (e *ToolExecutor) listDir(path string) string {
	fullPath := e.resolvePath(path)
	entries, err := os.ReadDir(fullPath)
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
