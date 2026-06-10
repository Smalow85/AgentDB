// pkg/config/manager.go
package config

import (
	"agent-db/pkg/executor"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type ConfigManager struct {
	exec *executor.Executor
}

type ModelConfig struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	DisplayName string    `json:"display_name"`
	BaseURL     string    `json:"base_url"`
	APIKey      string    `json:"api_key"`
	IsDefault   bool      `json:"is_default"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type ProjectConfig struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	RootPath    string    `json:"root_path"`
	Description string    `json:"description"`
	IsActive    bool      `json:"is_active"`
	LastUsed    time.Time `json:"last_used"`
	CreatedAt   time.Time `json:"created_at"`
}

type UserSettings struct {
	ID               int    `json:"id"`
	ActiveModelID    int    `json:"active_model_id"`
	ActiveProjectID  int    `json:"active_project_id"`
	DefaultSessionID int    `json:"default_session_id"`
	Theme            string `json:"theme"`
	StreamingEnabled bool   `json:"streaming_enabled"`
}

func NewConfigManager(exec *executor.Executor) *ConfigManager {
	cm := &ConfigManager{exec: exec}
	cm.initTables()
	return cm
}

func (cm *ConfigManager) initTables() {
	tables := []string{
		`CREATE TABLE IF NOT EXISTS model_configs (
            id INT PRIMARY KEY AUTOINCREMENT,
            name TEXT NOT NULL UNIQUE,
            display_name TEXT NOT NULL,
            base_url TEXT NOT NULL,
            api_key TEXT NOT NULL,
            is_default INT DEFAULT 0,
            created_at INT NOT NULL,
            updated_at INT NOT NULL
        )`,
		`CREATE TABLE IF NOT EXISTS project_configs (
            id INT PRIMARY KEY AUTOINCREMENT,
            name TEXT NOT NULL UNIQUE,
            root_path TEXT NOT NULL,
            description TEXT,
            is_active INT DEFAULT 0,
            last_used INT NOT NULL,
            created_at INT NOT NULL
        )`,
		`CREATE TABLE IF NOT EXISTS user_settings (
            id INT PRIMARY KEY DEFAULT 1,
            active_model_id INT,
            active_project_id INT,
            default_session_id INT DEFAULT 1,
            theme TEXT DEFAULT 'dark',
            streaming_enabled INT DEFAULT 1
        )`,
	}

	for _, sql := range tables {
		cm.exec.Execute(sql)
	}

	// Создаём запись настроек по умолчанию, если нет
	result, _ := cm.exec.Execute(`SELECT id FROM user_settings WHERE id = 1`)
	if result == nil || result.Type == "ERROR" || len(result.Rows) == 0 {
		cm.exec.Execute(`
            INSERT INTO user_settings (id, default_session_id, streaming_enabled) 
            VALUES (1, 1, 1)
        `)
	}

	fmt.Println("✓ Config tables initialized")
}

// ========== Вспомогательные функции ==========

func escapeSQL(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "'", "''"), "\n", " ")
}

// getFirstValue извлекает первое значение из первой строки QueryResult
func getFirstValue(qr *executor.QueryResult) string {
	if qr == nil || qr.Type == "ERROR" || len(qr.Rows) == 0 || len(qr.Rows[0]) == 0 {
		return ""
	}
	if qr.Rows[0][0] == nil {
		return ""
	}
	return fmt.Sprintf("%v", qr.Rows[0][0])
}

// getAllRows возвращает все строки как массив строк
func getAllRows(qr *executor.QueryResult) []map[string]interface{} {
	if qr == nil || qr.Type == "ERROR" || len(qr.Rows) == 0 {
		return nil
	}

	var result []map[string]interface{}
	for _, row := range qr.Rows {
		item := make(map[string]interface{})
		for i, col := range qr.Columns {
			if i < len(row) {
				item[col] = row[i]
			}
		}
		result = append(result, item)
	}
	return result
}

// ========== Управление моделями ==========

func (cm *ConfigManager) AddModel(name, displayName, baseURL, apiKey string, isDefault bool) (*ModelConfig, error) {
	now := time.Now().Unix()

	if isDefault {
		cm.exec.Execute("UPDATE model_configs SET is_default = 0")
	}

	defaultVal := 0
	if isDefault {
		defaultVal = 1
	}

	_, err := cm.exec.Execute(fmt.Sprintf(`
        INSERT INTO model_configs (name, display_name, base_url, api_key, is_default, created_at, updated_at)
        VALUES ('%s', '%s', '%s', '%s', %d, %d, %d)
    `, escapeSQL(name), escapeSQL(displayName), escapeSQL(baseURL), escapeSQL(apiKey), defaultVal, now, now))

	if err != nil {
		return nil, err
	}

	return cm.GetModelByName(name)
}

func (cm *ConfigManager) GetModelByName(name string) (*ModelConfig, error) {
	result, err := cm.exec.Execute(fmt.Sprintf(`
        SELECT id, name, display_name, base_url, api_key, is_default, created_at, updated_at
        FROM model_configs WHERE name = '%s'
    `, escapeSQL(name)))

	if err != nil {
		return nil, err
	}

	models := cm.parseModels(result)
	if len(models) == 0 {
		return nil, fmt.Errorf("модель не найдена: %s", name)
	}

	return &models[0], nil
}

func (cm *ConfigManager) GetActiveModel() (*ModelConfig, error) {
	settings, err := cm.GetSettings()
	if err == nil && settings.ActiveModelID > 0 {
		result, err := cm.exec.Execute(fmt.Sprintf(`
            SELECT id, name, display_name, base_url, api_key, is_default, created_at, updated_at
            FROM model_configs WHERE id = %d
        `, settings.ActiveModelID))
		if err == nil {
			models := cm.parseModels(result)
			if len(models) > 0 {
				return &models[0], nil
			}
		}
	}

	result, err := cm.exec.Execute(`
        SELECT id, name, display_name, base_url, api_key, is_default, created_at, updated_at
        FROM model_configs WHERE is_default = 1 LIMIT 1
    `)

	if err != nil {
		return nil, err
	}

	models := cm.parseModels(result)
	if len(models) == 0 {
		return nil, fmt.Errorf("нет активной модели")
	}

	return &models[0], nil
}

func (cm *ConfigManager) ListModels() ([]ModelConfig, error) {
	result, err := cm.exec.Execute(`
        SELECT id, name, display_name, base_url, api_key, is_default, created_at, updated_at
        FROM model_configs ORDER BY is_default DESC, name ASC
    `)

	if err != nil {
		return nil, err
	}

	return cm.parseModels(result), nil
}

func (cm *ConfigManager) DeleteModel(id int) error {
	_, err := cm.exec.Execute(fmt.Sprintf("DELETE FROM model_configs WHERE id = %d", id))
	return err
}

// ========== Управление проектами ==========

func (cm *ConfigManager) AddProject(name, rootPath, description string) (*ProjectConfig, error) {
	now := time.Now().Unix()

	_, err := cm.exec.Execute(fmt.Sprintf(`
        INSERT INTO project_configs (name, root_path, description, is_active, last_used, created_at)
        VALUES ('%s', '%s', '%s', 0, %d, %d)
    `, escapeSQL(name), escapeSQL(rootPath), escapeSQL(description), now, now))

	if err != nil {
		return nil, err
	}

	return cm.GetProjectByName(name)
}

func (cm *ConfigManager) GetProjectByName(name string) (*ProjectConfig, error) {
	result, err := cm.exec.Execute(fmt.Sprintf(`
        SELECT id, name, root_path, description, is_active, last_used, created_at
        FROM project_configs WHERE name = '%s'
    `, escapeSQL(name)))

	if err != nil {
		return nil, err
	}

	projects := cm.parseProjects(result)
	if len(projects) == 0 {
		return nil, fmt.Errorf("проект не найден: %s", name)
	}

	return &projects[0], nil
}

func (cm *ConfigManager) GetActiveProject() (*ProjectConfig, error) {
	settings, err := cm.GetSettings()
	if err == nil && settings.ActiveProjectID > 0 {
		result, err := cm.exec.Execute(fmt.Sprintf(`
            SELECT id, name, root_path, description, is_active, last_used, created_at
            FROM project_configs WHERE id = %d
        `, settings.ActiveProjectID))
		if err == nil {
			projects := cm.parseProjects(result)
			if len(projects) > 0 {
				return &projects[0], nil
			}
		}
	}

	result, err := cm.exec.Execute(`
        SELECT id, name, root_path, description, is_active, last_used, created_at
        FROM project_configs LIMIT 1
    `)

	if err != nil {
		return nil, err
	}

	projects := cm.parseProjects(result)
	if len(projects) == 0 {
		return nil, fmt.Errorf("нет активного проекта")
	}

	return &projects[0], nil
}

func (cm *ConfigManager) ListProjects() ([]ProjectConfig, error) {
	result, err := cm.exec.Execute(`
        SELECT id, name, root_path, description, is_active, last_used, created_at
        FROM project_configs ORDER BY last_used DESC, name ASC
    `)

	if err != nil {
		return nil, err
	}

	return cm.parseProjects(result), nil
}

func (cm *ConfigManager) SetActiveProject(id int) error {
	now := time.Now().Unix()

	cm.exec.Execute(fmt.Sprintf(`
        UPDATE project_configs SET last_used = %d WHERE id = %d
    `, now, id))

	_, err := cm.exec.Execute(fmt.Sprintf(`
        UPDATE user_settings SET active_project_id = %d WHERE id = 1
    `, id))

	return err
}

func (cm *ConfigManager) DeleteProject(id int) error {
	_, err := cm.exec.Execute(fmt.Sprintf("DELETE FROM project_configs WHERE id = %d", id))
	return err
}

// ========== Настройки пользователя ==========

func (cm *ConfigManager) GetSettings() (*UserSettings, error) {
	result, err := cm.exec.Execute(`
        SELECT active_model_id, active_project_id, default_session_id, theme, streaming_enabled
        FROM user_settings WHERE id = 1
    `)

	if err != nil {
		return &UserSettings{
			ActiveModelID:    0,
			ActiveProjectID:  0,
			DefaultSessionID: 1,
			Theme:            "dark",
			StreamingEnabled: true,
		}, nil
	}

	return cm.parseSettings(result), nil
}

func (cm *ConfigManager) SetActiveModel(id int) error {
	fmt.Printf("[SetActiveModel] Setting active_model_id to %d\n", id)
	_, err := cm.exec.Execute(fmt.Sprintf(`
        UPDATE user_settings SET active_model_id = %d WHERE id = 1
    `, id))
	return err
}

func (cm *ConfigManager) SetStreamingEnabled(enabled bool) error {
	val := 0
	if enabled {
		val = 1
	}
	fmt.Printf("[SetStreamingEnabled] Setting to %d\n", val)
	_, err := cm.exec.Execute(fmt.Sprintf(`
        UPDATE user_settings SET streaming_enabled = %d WHERE id = 1
    `, val))
	return err
}

func toInt(value interface{}) int {
	if value == nil {
		return 0
	}

	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
		return 0
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return int(i)
		}
		return 0
	default:
		fmt.Printf("[toInt] Unknown type: %T, value: %v\n", v, v)
		return 0
	}
}

// ========== Парсеры результатов ==========

func (cm *ConfigManager) parseModels(qr *executor.QueryResult) []ModelConfig {
	var models []ModelConfig

	if qr == nil || qr.Type == "ERROR" || len(qr.Rows) == 0 {
		return models
	}

	fmt.Printf("[parseModels] Processing %d rows\n", len(qr.Rows))
	fmt.Printf("[parseModels] Columns: %v\n", qr.Columns)

	for i, row := range qr.Rows {
		cfg := ModelConfig{}

		// Индексы колонок (порядок в SELECT)
		// SELECT id, name, display_name, base_url, api_key, is_default, created_at, updated_at
		//       0    1    2            3        4        5          6          7

		if len(row) >= 8 {
			// ID
			cfg.ID = toInt(row[0])

			// Name
			if v, ok := row[1].(string); ok {
				cfg.Name = v
			} else if row[1] != nil {
				cfg.Name = fmt.Sprintf("%v", row[1])
			}

			// DisplayName
			if v, ok := row[2].(string); ok {
				cfg.DisplayName = v
			} else if row[2] != nil {
				cfg.DisplayName = fmt.Sprintf("%v", row[2])
			}

			// BaseURL
			if v, ok := row[3].(string); ok {
				cfg.BaseURL = v
			} else if row[3] != nil {
				cfg.BaseURL = fmt.Sprintf("%v", row[3])
			}

			// APIKey
			if v, ok := row[4].(string); ok {
				cfg.APIKey = v
			} else if row[4] != nil {
				cfg.APIKey = fmt.Sprintf("%v", row[4])
			}

			// IsDefault
			switch v := row[5].(type) {
			case int64:
				cfg.IsDefault = v == 1
			case int:
				cfg.IsDefault = v == 1
			case float64:
				cfg.IsDefault = v == 1
			case bool:
				cfg.IsDefault = v
			}

			fmt.Printf("[parseModels] Row %d: ID=%d, Name=%s, IsDefault=%v\n", i, cfg.ID, cfg.Name, cfg.IsDefault)
		}

		models = append(models, cfg)
	}
	return models
}

// parseProjects парсит результат SELECT из project_configs
func (cm *ConfigManager) parseProjects(qr *executor.QueryResult) []ProjectConfig {
	var projects []ProjectConfig

	if qr == nil || qr.Type == "ERROR" || len(qr.Rows) == 0 {
		return projects
	}

	fmt.Printf("[parseProjects] Processing %d rows\n", len(qr.Rows))

	for i, row := range qr.Rows {
		cfg := ProjectConfig{}

		// SELECT id, name, root_path, description, is_active, last_used, created_at
		//        0    1     2          3           4         5          6

		if len(row) >= 7 {
			// ID
			switch v := row[0].(type) {
			case int64:
				cfg.ID = int(v)
			case int:
				cfg.ID = v
			case float64:
				cfg.ID = int(v)
			case string:
				cfg.ID, _ = strconv.Atoi(v)
			}

			// Name
			if v, ok := row[1].(string); ok {
				cfg.Name = v
			} else if row[1] != nil {
				cfg.Name = fmt.Sprintf("%v", row[1])
			}

			// RootPath
			if v, ok := row[2].(string); ok {
				cfg.RootPath = v
			} else if row[2] != nil {
				cfg.RootPath = fmt.Sprintf("%v", row[2])
			}

			// Description
			if v, ok := row[3].(string); ok {
				cfg.Description = v
			} else if row[3] != nil {
				cfg.Description = fmt.Sprintf("%v", row[3])
			}

			// IsActive
			switch v := row[4].(type) {
			case int64:
				cfg.IsActive = v == 1
			case int:
				cfg.IsActive = v == 1
			case float64:
				cfg.IsActive = v == 1
			case bool:
				cfg.IsActive = v
			}

			fmt.Printf("[parseProjects] Row %d: ID=%d, Name=%s, IsActive=%v\n", i, cfg.ID, cfg.Name, cfg.IsActive)
		}

		projects = append(projects, cfg)
	}

	return projects
}

// parseSettings парсит результат SELECT из user_settings
func (cm *ConfigManager) parseSettings(qr *executor.QueryResult) *UserSettings {
	settings := &UserSettings{
		ActiveModelID:    0,
		ActiveProjectID:  0,
		DefaultSessionID: 1,
		Theme:            "dark",
		StreamingEnabled: true,
	}

	if qr == nil || qr.Type == "ERROR" || len(qr.Rows) == 0 {
		fmt.Printf("[parseSettings] No settings found, using defaults\n")
		return settings
	}

	row := qr.Rows[0]
	// SELECT active_model_id, active_project_id, default_session_id, theme, streaming_enabled
	//        0                1                 2                3      4

	fmt.Printf("[parseSettings] Row length: %d\n", len(row))

	if len(row) >= 5 {
		// ActiveModelID
		switch v := row[0].(type) {
		case int64:
			settings.ActiveModelID = int(v)
		case int:
			settings.ActiveModelID = v
		case float64:
			settings.ActiveModelID = int(v)
		case string:
			settings.ActiveModelID, _ = strconv.Atoi(v)
		}

		// ActiveProjectID
		switch v := row[1].(type) {
		case int64:
			settings.ActiveProjectID = int(v)
		case int:
			settings.ActiveProjectID = v
		case float64:
			settings.ActiveProjectID = int(v)
		case string:
			settings.ActiveProjectID, _ = strconv.Atoi(v)
		}

		// DefaultSessionID
		switch v := row[2].(type) {
		case int64:
			settings.DefaultSessionID = int(v)
		case int:
			settings.DefaultSessionID = v
		case float64:
			settings.DefaultSessionID = int(v)
		}

		// Theme
		if v, ok := row[3].(string); ok {
			settings.Theme = v
		}

		// StreamingEnabled
		switch v := row[4].(type) {
		case int64:
			settings.StreamingEnabled = v == 1
		case int:
			settings.StreamingEnabled = v == 1
		case float64:
			settings.StreamingEnabled = v == 1
		case bool:
			settings.StreamingEnabled = v
		}
	}

	fmt.Printf("[parseSettings] ActiveModelID=%d, ActiveProjectID=%d, StreamingEnabled=%v\n",
		settings.ActiveModelID, settings.ActiveProjectID, settings.StreamingEnabled)

	return settings
}
