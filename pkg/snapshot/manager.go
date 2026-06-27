// pkg/snapshot/manager.go
package snapshot

import (
	"agent-db/pkg/context"
	"agent-db/pkg/executor"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type SnapshotManager struct {
	exec        *executor.Executor
	projectPath string
	memoryMgr   *context.MemoryManager
}

type Version struct {
	ID          int
	VersionNum  int
	ParentID    int
	Description string
	CreatedAt   int64
	CreatedBy   string
}

type FileDiff struct {
	Path        string
	OldContent  string
	NewContent  string
	UnifiedDiff string
}

type Diff struct {
	Added    []string
	Removed  []string
	Modified []FileDiff
}

func NewSnapshotManager(exec *executor.Executor, projectPath string, memoryMgr *context.MemoryManager) *SnapshotManager {
	sm := &SnapshotManager{
		exec:        exec,
		projectPath: projectPath,
		memoryMgr:   memoryMgr,
	}
	sm.initTables()
	return sm
}

func (sm *SnapshotManager) initTables() {
	tables := []string{
		`CREATE TABLE IF NOT EXISTS versions (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            version_num INTEGER NOT NULL UNIQUE,
            parent_version INTEGER,
            description TEXT,
            created_at INTEGER NOT NULL,
            created_by TEXT,
            metadata TEXT
        )`,
		`CREATE TABLE IF NOT EXISTS objects (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            hash TEXT UNIQUE NOT NULL,
            content TEXT,
            size INTEGER,
            created_at INTEGER
        )`,
		`CREATE TABLE IF NOT EXISTS version_files (
            version_id INTEGER NOT NULL,
            file_path TEXT NOT NULL,
            object_hash TEXT NOT NULL,
            operation TEXT,
            PRIMARY KEY (version_id, file_path)
        )`,
		`CREATE TABLE IF NOT EXISTS version_memory (
            version_id INTEGER PRIMARY KEY,
            instructions TEXT,
            thoughts TEXT,
            inferences TEXT,
            buffer TEXT,
            created_at INTEGER
        )`,
	}
	for _, sql := range tables {
		sm.exec.Execute(sql)
	}
}

// ========== ОСНОВНЫЕ МЕТОДЫ ==========

func (sm *SnapshotManager) CreateSnapshot(description, createdBy string) (int, error) {
	currentVersion := sm.getCurrentVersion()
	newVersion := currentVersion + 1
	now := time.Now().Unix()

	// 1. Создаём запись о версии
	_, err := sm.exec.Execute(fmt.Sprintf(`
        INSERT INTO versions (version_num, parent_version, description, created_at, created_by)
        VALUES (%d, %d, '%s', %d, '%s')
    `, newVersion, currentVersion, escapeSQL(description), now, createdBy))
	if err != nil {
		return 0, err
	}

	versionID := sm.getVersionID(newVersion)

	// 2. Сохраняем изменённые файлы
	if err := sm.snapshotFiles(versionID); err != nil {
		return 0, err
	}

	// 3. Сохраняем память агента
	if err := sm.snapshotMemory(versionID); err != nil {
		return 0, err
	}

	return versionID, nil
}

func (sm *SnapshotManager) RollbackToVersion(versionID int) error {
	// 1. Восстанавливаем файлы
	if err := sm.restoreFiles(versionID); err != nil {
		return err
	}

	// 2. Восстанавливаем память агента
	if err := sm.restoreMemory(versionID); err != nil {
		return err
	}

	return nil
}

func (sm *SnapshotManager) Diff(versionA, versionB int) (*Diff, error) {
	filesA := sm.getFilesAtVersion(versionA)
	filesB := sm.getFilesAtVersion(versionB)

	diff := &Diff{
		Added:    []string{},
		Removed:  []string{},
		Modified: []FileDiff{},
	}

	for path, hashA := range filesA {
		if hashB, ok := filesB[path]; ok {
			if hashA != hashB {
				contentA := sm.getContent(hashA)
				contentB := sm.getContent(hashB)
				diff.Modified = append(diff.Modified, FileDiff{
					Path:        path,
					OldContent:  contentA,
					NewContent:  contentB,
					UnifiedDiff: unifiedDiff(contentA, contentB),
				})
			}
		} else {
			diff.Removed = append(diff.Removed, path)
		}
	}

	for path := range filesB {
		if _, ok := filesA[path]; !ok {
			diff.Added = append(diff.Added, path)
		}
	}

	return diff, nil
}

// ========== ВНУТРЕННИЕ МЕТОДЫ ==========

func (sm *SnapshotManager) snapshotFiles(versionID int) error {
	changedFiles, err := sm.getChangedFiles()
	if err != nil {
		return err
	}

	for _, file := range changedFiles {
		content, err := os.ReadFile(file)
		if err != nil {
			continue
		}

		hash := sha256sum(string(content))

		// Сохраняем объект, если новый
		sm.exec.Execute(fmt.Sprintf(`
            INSERT OR IGNORE INTO objects (hash, content, size, created_at)
            VALUES ('%s', '%s', %d, %d)
        `, hash, escapeSQL(string(content)), len(content), time.Now().Unix()))

		// Сохраняем ссылку в версии
		sm.exec.Execute(fmt.Sprintf(`
            INSERT OR REPLACE INTO version_files (version_id, file_path, object_hash, operation)
            VALUES (%d, '%s', '%s', 'modify')
        `, versionID, escapeSQL(file), hash))
	}

	return nil
}

func (sm *SnapshotManager) snapshotMemory(versionID int) error {
	// Получаем текущую память
	instructions := sm.getCurrentInstructions()
	thoughts := sm.getCurrentThoughts()
	inferences := sm.getCurrentInferences()
	buffer := sm.getCurrentBuffer()

	instructionsJSON, _ := json.Marshal(instructions)
	thoughtsJSON, _ := json.Marshal(thoughts)
	inferencesJSON, _ := json.Marshal(inferences)
	bufferJSON, _ := json.Marshal(buffer)

	_, err := sm.exec.Execute(fmt.Sprintf(`
        INSERT OR REPLACE INTO version_memory (version_id, instructions, thoughts, inferences, buffer, created_at)
        VALUES (%d, '%s', '%s', '%s', '%s', %d)
    `, versionID, escapeSQL(string(instructionsJSON)), escapeSQL(string(thoughtsJSON)),
		escapeSQL(string(inferencesJSON)), escapeSQL(string(bufferJSON)), time.Now().Unix()))

	return err
}

func (sm *SnapshotManager) restoreFiles(versionID int) error {
	// Получаем все файлы из версии
	files := sm.getFilesAtVersion(versionID)

	for path, hash := range files {
		content := sm.getContent(hash)
		fullPath := filepath.Join(sm.projectPath, path)

		// Создаём директорию если нужно
		dir := filepath.Dir(fullPath)
		os.MkdirAll(dir, 0755)

		// Записываем файл
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			return err
		}
	}

	return nil
}

func (sm *SnapshotManager) restoreMemory(versionID int) error {
	result, _ := sm.exec.Execute(fmt.Sprintf(`
        SELECT instructions, thoughts, inferences, buffer
        FROM version_memory WHERE version_id = %d
    `, versionID))

	if result == nil || result.Type == "ERROR" || len(result.Rows) == 0 {
		return nil
	}

	row := result.Rows[0]
	if len(row) >= 4 {
		// Восстанавливаем инструкции
		if row[0] != nil {
			var instructions []Instruction
			json.Unmarshal([]byte(fmt.Sprintf("%v", row[0])), &instructions)
			sm.restoreInstructions(instructions)
		}

		// Восстанавливаем мысли
		if row[1] != nil {
			var thoughts []Thought
			json.Unmarshal([]byte(fmt.Sprintf("%v", row[1])), &thoughts)
			sm.restoreThoughts(thoughts)
		}

		// Восстанавливаем выводы
		if row[2] != nil {
			var inferences []Inference
			json.Unmarshal([]byte(fmt.Sprintf("%v", row[2])), &inferences)
			sm.restoreInferences(inferences)
		}

		// Восстанавливаем буфер
		if row[3] != nil {
			var buffer map[string]BufferItem
			json.Unmarshal([]byte(fmt.Sprintf("%v", row[3])), &buffer)
			sm.restoreBuffer(buffer)
		}
	}

	return nil
}

// ========== ВСПОМОГАТЕЛЬНЫЕ МЕТОДЫ ДЛЯ РАБОТЫ С ВЕРСИЯМИ ==========

func (sm *SnapshotManager) getCurrentVersion() int {
	result, _ := sm.exec.Execute(`SELECT COALESCE(MAX(version_num), 0) FROM versions`)
	if result == nil || result.Type == "ERROR" || len(result.Rows) == 0 {
		return 0
	}
	if len(result.Rows[0]) > 0 && result.Rows[0][0] != nil {
		var version int
		fmt.Sscanf(fmt.Sprintf("%v", result.Rows[0][0]), "%d", &version)
		return version
	}
	return 0
}

func (sm *SnapshotManager) getVersionID(versionNum int) int {
	result, _ := sm.exec.Execute(fmt.Sprintf(`SELECT id FROM versions WHERE version_num = %d`, versionNum))
	if result == nil || result.Type == "ERROR" || len(result.Rows) == 0 {
		return 0
	}
	if len(result.Rows[0]) > 0 && result.Rows[0][0] != nil {
		var id int
		fmt.Sscanf(fmt.Sprintf("%v", result.Rows[0][0]), "%d", &id)
		return id
	}
	return 0
}

func (sm *SnapshotManager) getChangedFiles() ([]string, error) {
	var changedFiles []string

	// Рекурсивно обходим директорию проекта
	err := filepath.Walk(sm.projectPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			// Пропускаем скрытые директории и .git
			if strings.HasPrefix(info.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}

		// Пропускаем бинарные файлы и большие файлы (> 1MB)
		if info.Size() > 1024*1024 {
			return nil
		}

		// Получаем относительный путь
		relPath, err := filepath.Rel(sm.projectPath, path)
		if err != nil {
			relPath = path
		}
		changedFiles = append(changedFiles, relPath)
		return nil
	})

	return changedFiles, err
}

func (sm *SnapshotManager) getFilesAtVersion(versionID int) map[string]string {
	files := make(map[string]string)

	result, _ := sm.exec.Execute(fmt.Sprintf(`
		SELECT file_path, object_hash FROM version_files WHERE version_id = %d
	`, versionID))

	if result == nil || result.Type == "ERROR" {
		return files
	}

	for _, row := range result.Rows {
		if len(row) >= 2 && row[0] != nil && row[1] != nil {
			files[fmt.Sprintf("%v", row[0])] = fmt.Sprintf("%v", row[1])
		}
	}

	return files
}

func (sm *SnapshotManager) getContent(hash string) string {
	result, _ := sm.exec.Execute(fmt.Sprintf(`SELECT content FROM objects WHERE hash = '%s'`, hash))
	if result == nil || result.Type == "ERROR" || len(result.Rows) == 0 {
		return ""
	}
	if len(result.Rows[0]) > 0 && result.Rows[0][0] != nil {
		return fmt.Sprintf("%v", result.Rows[0][0])
	}
	return ""
}

// ========== МЕТОДЫ ДЛЯ РАБОТЫ С ПАМЯТЬЮ АГЕНТА ==========

type Instruction struct {
	ID        int    `json:"id"`
	SessionID int    `json:"session_id"`
	Content   string `json:"content"`
	Depth     int    `json:"depth"`
	Status    string `json:"status"`
	CreatedAt int64  `json:"created_at"`
}

type Thought struct {
	ID         int     `json:"id"`
	SessionID  int     `json:"session_id"`
	Type       string  `json:"type"`
	Content    string  `json:"content"`
	Confidence float64 `json:"confidence"`
	CreatedAt  int64   `json:"created_at"`
}

type Inference struct {
	ID         int     `json:"id"`
	SessionID  int     `json:"session_id"`
	Conclusion string  `json:"conclusion"`
	Confidence float64 `json:"confidence"`
	Type       string  `json:"type"`
	Source     string  `json:"source"`
	CreatedAt  int64   `json:"created_at"`
}

type BufferItem struct {
	Key       string `json:"key"`
	Value     string `json:"value"`
	TTL       int    `json:"ttl"`
	CreatedAt int64  `json:"created_at"`
}

func (sm *SnapshotManager) getCurrentInstructions() []Instruction {
	var instructions []Instruction

	// Получаем инструкции из memoryMgr
	if sm.memoryMgr != nil {
		// Здесь нужно получить текущие инструкции из MemoryManager
		// Пока возвращаем пустой слайс
	}

	return instructions
}

func (sm *SnapshotManager) getCurrentThoughts() []Thought {
	var thoughts []Thought

	if sm.memoryMgr != nil {
		// Здесь нужно получить текущие мысли из MemoryManager
	}

	return thoughts
}

func (sm *SnapshotManager) getCurrentInferences() []Inference {
	var inferences []Inference

	if sm.memoryMgr != nil {
		// Здесь нужно получить текущие выводы из MemoryManager
	}

	return inferences
}

func (sm *SnapshotManager) getCurrentBuffer() map[string]BufferItem {
	buffer := make(map[string]BufferItem)

	if sm.memoryMgr != nil {
		// Здесь нужно получить текущий буфер из MemoryManager
	}

	return buffer
}

func (sm *SnapshotManager) restoreInstructions(instructions []Instruction) {
	if sm.memoryMgr == nil {
		return
	}

	// Очищаем текущие инструкции
	sm.exec.Execute("DELETE FROM instruction_stack WHERE session_id > 0")

	// Восстанавливаем инструкции
	for _, inst := range instructions {
		sm.exec.Execute(fmt.Sprintf(`
			INSERT INTO instruction_stack (session_id, content, depth, status, created_at)
			VALUES (%d, '%s', %d, '%s', %d)
		`, inst.SessionID, escapeSQL(inst.Content), inst.Depth, inst.Status, inst.CreatedAt))
	}
}

func (sm *SnapshotManager) restoreThoughts(thoughts []Thought) {
	if sm.memoryMgr == nil {
		return
	}

	// Очищаем текущие мысли
	sm.exec.Execute("DELETE FROM reasoning_space WHERE session_id > 0")

	// Восстанавливаем мысли
	for _, thought := range thoughts {
		sm.exec.Execute(fmt.Sprintf(`
			INSERT INTO reasoning_space (session_id, thought_type, content, confidence, created_at)
			VALUES (%d, '%s', '%s', %f, %d)
		`, thought.SessionID, thought.Type, escapeSQL(thought.Content), thought.Confidence, thought.CreatedAt))
	}
}

func (sm *SnapshotManager) restoreInferences(inferences []Inference) {
	if sm.memoryMgr == nil {
		return
	}

	// Очищаем текущие выводы
	sm.exec.Execute("DELETE FROM inference_space WHERE session_id > 0")

	// Восстанавливаем выводы
	for _, inference := range inferences {
		sm.exec.Execute(fmt.Sprintf(`
			INSERT INTO inference_space (session_id, conclusion, confidence, inference_type, source, created_at)
			VALUES (%d, '%s', %f, '%s', '%s', %d)
		`, inference.SessionID, escapeSQL(inference.Conclusion), inference.Confidence, inference.Type, inference.Source, inference.CreatedAt))
	}
}

func (sm *SnapshotManager) restoreBuffer(buffer map[string]BufferItem) {
	if sm.memoryMgr == nil {
		return
	}

	// Очищаем текущий буфер
	sm.exec.Execute("DELETE FROM buffer_space WHERE session_id > 0")

	// Восстанавливаем буфер
	for key, item := range buffer {
		sm.exec.Execute(fmt.Sprintf(`
			INSERT INTO buffer_space (session_id, key, value, ttl, created_at)
			VALUES (1, '%s', '%s', %d, %d)
		`, escapeSQL(key), escapeSQL(item.Value), item.TTL, item.CreatedAt))
	}
}

// ========== МЕТОДЫ ДЛЯ РАБОТЫ СО СНАПШОТАМИ ==========

func (sm *SnapshotManager) ListVersions() ([]Version, error) {
	var versions []Version

	result, err := sm.exec.Execute(`
		SELECT id, version_num, parent_version, description, created_at, created_by
		FROM versions ORDER BY version_num DESC
	`)

	if err != nil {
		return nil, err
	}

	if result == nil || result.Type == "ERROR" {
		return versions, nil
	}

	for _, row := range result.Rows {
		if len(row) >= 6 {
			v := Version{}
			fmt.Sscanf(fmt.Sprintf("%v", row[0]), "%d", &v.ID)
			fmt.Sscanf(fmt.Sprintf("%v", row[1]), "%d", &v.VersionNum)
			fmt.Sscanf(fmt.Sprintf("%v", row[2]), "%d", &v.ParentID)
			v.Description = fmt.Sprintf("%v", row[3])
			fmt.Sscanf(fmt.Sprintf("%v", row[4]), "%d", &v.CreatedAt)
			v.CreatedBy = fmt.Sprintf("%v", row[5])
			versions = append(versions, v)
		}
	}

	return versions, nil
}

func (sm *SnapshotManager) GetVersion(id int) (*Version, error) {
	result, err := sm.exec.Execute(fmt.Sprintf(`
		SELECT id, version_num, parent_version, description, created_at, created_by
		FROM versions WHERE id = %d
	`, id))

	if err != nil {
		return nil, err
	}

	if result == nil || result.Type == "ERROR" || len(result.Rows) == 0 {
		return nil, fmt.Errorf("version %d not found", id)
	}

	row := result.Rows[0]
	if len(row) >= 6 {
		v := &Version{}
		fmt.Sscanf(fmt.Sprintf("%v", row[0]), "%d", &v.ID)
		fmt.Sscanf(fmt.Sprintf("%v", row[1]), "%d", &v.VersionNum)
		fmt.Sscanf(fmt.Sprintf("%v", row[2]), "%d", &v.ParentID)
		v.Description = fmt.Sprintf("%v", row[3])
		fmt.Sscanf(fmt.Sprintf("%v", row[4]), "%d", &v.CreatedAt)
		v.CreatedBy = fmt.Sprintf("%v", row[5])
		return v, nil
	}

	return nil, fmt.Errorf("version %d not found", id)
}

func (sm *SnapshotManager) DeleteVersion(versionID int) error {
	// Удаляем записи о файлах версии
	_, err := sm.exec.Execute(fmt.Sprintf(`DELETE FROM version_files WHERE version_id = %d`, versionID))
	if err != nil {
		return err
	}

	// Удаляем запись о памяти версии
	_, err = sm.exec.Execute(fmt.Sprintf(`DELETE FROM version_memory WHERE version_id = %d`, versionID))
	if err != nil {
		return err
	}

	// Удаляем саму версию
	_, err = sm.exec.Execute(fmt.Sprintf(`DELETE FROM versions WHERE id = %d`, versionID))

	return err
}

func (sm *SnapshotManager) GetCurrentVersion() *Version {
	currentNum := sm.getCurrentVersion()
	if currentNum == 0 {
		return nil
	}

	id := sm.getVersionID(currentNum)
	version, _ := sm.GetVersion(id)
	return version
}

// ========== ВСПОМОГАТЕЛЬНЫЕ ФУНКЦИИ ==========

func sha256sum(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}

func unifiedDiff(oldContent, newContent string) string {
	// Упрощённая версия — можно использовать github.com/sergi/go-diff
	if oldContent == newContent {
		return ""
	}
	return fmt.Sprintf("--- old\n+++ new\n%s\n%s", oldContent[:100], newContent[:100])
}

func escapeSQL(s string) string {
	s = strings.ReplaceAll(s, "'", "''")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}
