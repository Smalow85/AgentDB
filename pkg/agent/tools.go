package agent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"agent-db/pkg/graph"
	"agent-db/pkg/executor"
)

type ToolExecutor struct {
	PSIGraph       *graph.Graph
	ContextManager *ContextManager
	Executor       *executor.Executor
}

func (e *ToolExecutor) Execute(call ToolCall) string {
	switch call.Name {
	case "read_file":
		return e.readFile(call.Args["path"])
	case "write_file":
		return e.writeFile(call.Args["path"], call.Args["content"])
	case "edit_file":
		return e.editFile(call.Args["path"], call.Args["old_string"], call.Args["new_string"])
	case "delete_file":
		return e.deleteFile(call.Args["path"])
	case "list_dir":
		return e.listDir(call.Args["path"])
	case "run_command":
		return e.runCommand(call.Args["command"])
	case "find_callers":
		return e.psiFindCallers(call.Args["function"])
	case "find_callees":
		return e.psiFindCallees(call.Args["function"])
	case "get_class":
		return e.psiGetClass(call.Args["class"])
	case "find_call_path":
		return e.psiFindPath(call.Args["from"], call.Args["to"])
	case "search_code":
		return e.searchCode(call.Args["pattern"])
	// Инструменты управления контекстом
	case "context_snapshot":
		return e.contextSnapshot()
	case "context_restore":
		return e.contextRestore(call.Args["epoch"])
	case "context_rollback":
		return e.contextRollback(call.Args["steps"])
	case "context_gc":
		return e.contextGC(call.Args["type"])
	case "add_thought":
		return e.addThought(call.Args["type"], call.Args["content"])
	case "add_to_buffer":
		return e.addToBuffer(call.Args["key"], call.Args["data"], call.Args["ttl"])
	case "add_inference":
		return e.addInference(call.Args["conclusion"], call.Args["confidence"], call.Args["type"])
	default:
		return fmt.Sprintf("Неизвестный инструмент: %s", call.Name)
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
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Sprintf("Ошибка создания директории: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Sprintf("Ошибка записи %s: %v", path, err)
	}
	return fmt.Sprintf("✓ Файл %s записан (%d байт)", path, len(content))
}

func (e *ToolExecutor) editFile(path, oldStr, newStr string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("Ошибка чтения %s: %v", path, err)
	}
	updated := strings.Replace(string(content), oldStr, newStr, 1)
	if updated == string(content) {
		return fmt.Sprintf("Ошибка: строка не найдена в %s", path)
	}
	os.WriteFile(path, []byte(updated), 0644)
	return fmt.Sprintf("✓ Файл %s обновлён", path)
}

func (e *ToolExecutor) deleteFile(path string) string {
	if err := os.Remove(path); err != nil {
		return fmt.Sprintf("Ошибка удаления %s: %v", path, err)
	}
	return fmt.Sprintf("✓ Файл %s удалён", path)
}

func (e *ToolExecutor) listDir(path string) string {
	entries, err := os.ReadDir(path)
	if err != nil {
		return fmt.Sprintf("Ошибка чтения директории %s: %v", path, err)
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
		return fmt.Sprintf("Ошибка выполнения: %v\n%s", err, string(output))
	}
	return string(output)
}

func (e *ToolExecutor) searchCode(pattern string) string {
	cmd := exec.Command("grep", "-rn", "--include=*.go", pattern, ".")
	output, _ := cmd.CombinedOutput()
	if len(output) == 0 {
		return "Ничего не найдено"
	}
	return string(output)
}

func (e *ToolExecutor) psiFindCallers(funcName string) string {
	callers := e.PSIGraph.GetCallersByName(funcName)
	if len(callers) == 0 {
		return fmt.Sprintf("Функция '%s' никем не вызывается", funcName)
	}
	return fmt.Sprintf("Функцию '%s' вызывают: %s", funcName, strings.Join(callers, ", "))
}

func (e *ToolExecutor) psiFindCallees(funcName string) string {
	callees := e.PSIGraph.GetCalleesByName(funcName)
	if len(callees) == 0 {
		return fmt.Sprintf("Функция '%s' никого не вызывает", funcName)
	}
	return fmt.Sprintf("Функция '%s' вызывает: %s", funcName, strings.Join(callees, ", "))
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

func (e *ToolExecutor) psiFindPath(from, to string) string {
	paths, err := e.PSIGraph.FindCallPath(from, to)
	if err != nil {
		return fmt.Sprintf("Ошибка: %v", err)
	}
	if len(paths) == 0 {
		return fmt.Sprintf("Путь от '%s' до '%s' не найден", from, to)
	}
	var result strings.Builder
	result.WriteString(fmt.Sprintf("Цепочки от '%s' до '%s':\n", from, to))
	for i, path := range paths {
		result.WriteString(fmt.Sprintf("  %d: %s\n", i+1, strings.Join(path, " → ")))
	}
	return result.String()
}

// ========== Инструменты управления контекстом ==========

func (e *ToolExecutor) contextSnapshot() string {
	if e.ContextManager == nil {
		return "❌ ContextManager не инициализирован"
	}
	
	snapshot, err := e.ContextManager.CreateSnapshot()
	if err != nil {
		return fmt.Sprintf("❌ Ошибка создания снимка: %v", err)
	}
	
	return fmt.Sprintf("✓ Снимок создан: epoch=%d, time=%s", snapshot.Epoch, snapshot.Timestamp.Format("15:04:05"))
}

func (e *ToolExecutor) contextRestore(epochStr string) string {
	if e.ContextManager == nil {
		return "❌ ContextManager не инициализирован"
	}
	
	// Парсим эпоху из строки
	var epoch int
	fmt.Sscanf(epochStr, "%d", &epoch)
	
	snapshot := &ContextSnapshot{
		SessionID: e.ContextManager.GetSessionID(),
		Epoch:     epoch,
	}
	
	err := e.ContextManager.Restore(snapshot)
	if err != nil {
		return fmt.Sprintf("❌ Ошибка восстановления: %v", err)
	}
	
	return fmt.Sprintf("✓ Контекст восстановлен до эпохи %d", epoch)
}

func (e *ToolExecutor) contextRollback(stepsStr string) string {
	if e.ContextManager == nil {
		return "❌ ContextManager не инициализирован"
	}
	
	var steps int
	fmt.Sscanf(stepsStr, "%d", &steps)
	if steps <= 0 {
		steps = 1
	}
	
	err := e.ContextManager.Rollback(steps)
	if err != nil {
		return fmt.Sprintf("❌ Ошибка отката: %v", err)
	}
	
	return fmt.Sprintf("✓ Откат на %d шагов выполнен. Текущая эпоха: %d", steps, e.ContextManager.GetEpoch())
}

func (e *ToolExecutor) contextGC(gcType string) string {
	if e.ContextManager == nil {
		return "❌ ContextManager не инициализирован"
	}
	
	if gcType == "" {
		gcType = "minor"
	}
	
	err := e.ContextManager.GC(gcType)
	if err != nil {
		return fmt.Sprintf("❌ Ошибка GC: %v", err)
	}
	
	return fmt.Sprintf("✓ GC типа '%s' выполнен", gcType)
}

func (e *ToolExecutor) addThought(thoughtType, content string) string {
	if e.ContextManager == nil {
		return "❌ ContextManager не инициализирован"
	}
	
	if thoughtType == "" {
		thoughtType = "observation"
	}
	
	epoch, err := e.ContextManager.AddThought(thoughtType, content, 0)
	if err != nil {
		return fmt.Sprintf("❌ Ошибка добавления мысли: %v", err)
	}
	
	return fmt.Sprintf("✓ Мысль добавлена в эпоху %d: [%s] %s", epoch, thoughtType, truncateString(content, 50))
}

func (e *ToolExecutor) addToBuffer(key, data, ttlStr string) string {
	if e.ContextManager == nil {
		return "❌ ContextManager не инициализирован"
	}
	
	if key == "" {
		return "❌ Ключ буфера не указан"
	}
	
	var ttl int
	fmt.Sscanf(ttlStr, "%d", &ttl)
	
	err := e.ContextManager.AddToBuffer(key, data, ttl)
	if err != nil {
		return fmt.Sprintf("❌ Ошибка записи в буфер: %v", err)
	}
	
	return fmt.Sprintf("✓ Данные записаны в буфер: %s (%d байт)", key, len(data))
}

func (e *ToolExecutor) addInference(conclusion, confidence, inferenceType string) string {
	if e.ContextManager == nil {
		return "❌ ContextManager не инициализирован"
	}
	
	var conf float64
	fmt.Sscanf(confidence, "%f", &conf)
	if conf == 0 {
		conf = 0.5
	}
	
	err := e.ContextManager.AddInference(conclusion, conf, inferenceType)
	if err != nil {
		return fmt.Sprintf("❌ Ошибка добавления вывода: %v", err)
	}
	
	return fmt.Sprintf("✓ Вывод добавлен: %s (уверенность: %.2f)", truncateString(conclusion, 50), conf)
}

// Вспомогательные функции

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}