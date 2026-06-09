// pkg/config/manager.go
package config

import (
    "fmt"
    "time"
 	"strings"
    "agent-db/pkg/executor"
)

type ConfigManager struct {
    exec *executor.Executor
}

type ModelConfig struct {
    ID          int       `json:"id"`
    Name        string    `json:"name"`        // "gpt-4", "claude-3", etc.
    DisplayName string    `json:"display_name"` // "GPT-4 Turbo"
    BaseURL     string    `json:"base_url"`     // API endpoint
    APIKey      string    `json:"api_key"`      // зашифрованный!
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
    ID                int    `json:"id"`
    ActiveModelID     int    `json:"active_model_id"`
    ActiveProjectID   int    `json:"active_project_id"`
    DefaultSessionID  int    `json:"default_session_id"`
    Theme             string `json:"theme"`
    StreamingEnabled  bool   `json:"streaming_enabled"`
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
        _, err := cm.exec.Execute(sql)
        if err != nil {
            fmt.Printf("[CONFIG] Create table error: %v\n", err)
        }
    }
    
    // Создаём запись настроек по умолчанию, если нет
    check, _ := cm.exec.Execute(`SELECT id FROM user_settings WHERE id = 1`)
    if check == "" || strings.Contains(check, "Строк: 0") {
        _, err := cm.exec.Execute(`
            INSERT INTO user_settings (id, default_session_id, streaming_enabled) 
            VALUES (1, 1, 1)
        `)
        if err != nil {
            fmt.Printf("[CONFIG] Insert settings error: %v\n", err)
        }
    }
    
    fmt.Println("✓ Config tables initialized")
}

// ========== Управление моделями ==========

func (cm *ConfigManager) AddModel(name, displayName, baseURL, apiKey string, isDefault bool) (*ModelConfig, error) {
    now := time.Now().Unix()
    
    if isDefault {
        // Сбрасываем флаг default у всех остальных
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
    
    if err != nil || result == "" {
        return nil, fmt.Errorf("модель не найдена: %s", name)
    }
    
    return cm.parseModelRow(result), nil
}

func (cm *ConfigManager) GetActiveModel() (*ModelConfig, error) {
    // Сначала из настроек пользователя
    settings, err := cm.GetSettings()
    if err == nil && settings.ActiveModelID > 0 {
        result, err := cm.exec.Execute(fmt.Sprintf(`
            SELECT id, name, display_name, base_url, api_key, is_default, created_at, updated_at
            FROM model_configs WHERE id = %d
        `, settings.ActiveModelID))
        if err == nil && result != "" {
            return cm.parseModelRow(result), nil
        }
    }
    
    // Иначе берём default
    result, err := cm.exec.Execute(`
        SELECT id, name, display_name, base_url, api_key, is_default, created_at, updated_at
        FROM model_configs WHERE is_default = 1 LIMIT 1
    `)
    
    if err != nil || result == "" {
        return nil, fmt.Errorf("нет активной модели")
    }
    
    return cm.parseModelRow(result), nil
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
    
    if err != nil || result == "" {
        return nil, fmt.Errorf("проект не найден: %s", name)
    }
    
    return cm.parseProjectRow(result), nil
}

func (cm *ConfigManager) GetActiveProject() (*ProjectConfig, error) {
    settings, err := cm.GetSettings()
    if err == nil && settings.ActiveProjectID > 0 {
        result, err := cm.exec.Execute(fmt.Sprintf(`
            SELECT id, name, root_path, description, is_active, last_used, created_at
            FROM project_configs WHERE id = %d
        `, settings.ActiveProjectID))
        if err == nil && result != "" {
            return cm.parseProjectRow(result), nil
        }
    }
    
    // Иначе берём первый попавшийся
    result, err := cm.exec.Execute(`
        SELECT id, name, root_path, description, is_active, last_used, created_at
        FROM project_configs LIMIT 1
    `)
    
    if err != nil || result == "" {
        return nil, fmt.Errorf("нет активного проекта")
    }
    
    return cm.parseProjectRow(result), nil
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
    
    // Обновляем last_used
    cm.exec.Execute(fmt.Sprintf(`
        UPDATE project_configs SET last_used = %d WHERE id = %d
    `, now, id))
    
    // Обновляем настройки пользователя
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
    
    if err != nil || result == "" {
        return &UserSettings{
            ActiveModelID:    0,
            ActiveProjectID:  0,
            DefaultSessionID: 1,
            Theme:           "dark",
            StreamingEnabled: true,
        }, nil
    }
    
    return cm.parseSettings(result), nil
}

func (cm *ConfigManager) SetActiveModel(id int) error {
    fmt.Printf("[SetActiveModel] Setting active_model_id to %d\n", id)
    result, err := cm.exec.Execute(fmt.Sprintf(`
        UPDATE user_settings SET active_model_id = %d WHERE id = 1
    `, id))
    fmt.Printf("[SetActiveModel] Result: %s, err: %v\n", result, err)
    return err
}

func (cm *ConfigManager) SetStreamingEnabled(enabled bool) error {
    val := 0
    if enabled {
        val = 1
    }
    fmt.Printf("[SetStreamingEnabled] Setting to %d\n", val)
    result, err := cm.exec.Execute(fmt.Sprintf(`
        UPDATE user_settings SET streaming_enabled = %d WHERE id = 1
    `, val))
    fmt.Printf("[SetStreamingEnabled] Result: %s\n", result)
    return err
}

// ========== Парсеры результатов ==========

func (cm *ConfigManager) parseModelRow(result string) *ModelConfig {
    // Простой парсинг (для реального проекта используйте нормальный SQL парсер)
    var cfg ModelConfig
    fmt.Sscanf(result, "%d|%s|%s|%s|%s|%d|%d|%d", 
        &cfg.ID, &cfg.Name, &cfg.DisplayName, &cfg.BaseURL, &cfg.APIKey, 
        &cfg.IsDefault, &cfg.CreatedAt, &cfg.UpdatedAt)
    return &cfg
}

func (cm *ConfigManager) parseModels(result string) []ModelConfig {
    var models []ModelConfig
    // Упрощённо — в реальности нужен нормальный парсер
    return models
}

func (cm *ConfigManager) parseProjectRow(result string) *ProjectConfig {
    var cfg ProjectConfig
    fmt.Sscanf(result, "%d|%s|%s|%s|%d|%d|%d",
        &cfg.ID, &cfg.Name, &cfg.RootPath, &cfg.Description,
        &cfg.IsActive, &cfg.LastUsed, &cfg.CreatedAt)
    return &cfg
}

func (cm *ConfigManager) parseProjects(result string) []ProjectConfig {
    var projects []ProjectConfig
    return projects
}

func (cm *ConfigManager) parseSettings(result string) *UserSettings {
    var settings UserSettings
    
    // Ищем первую строку данных (с числом)
    lines := strings.Split(result, "\n")
    for _, line := range lines {
        line = strings.TrimSpace(line)
        // Пропускаем рамки и заголовки
        if strings.HasPrefix(line, "│") || strings.HasPrefix(line, "├") || strings.HasPrefix(line, "┌") || strings.HasPrefix(line, "└") {
            // Это может быть строка данных - убираем рамку слева и справа
            line = strings.TrimPrefix(line, "│")
            line = strings.TrimSuffix(line, "│")
            line = strings.TrimSpace(line)
            
            // Убираем лишние пробелы вокруг |
            line = strings.Join(strings.Fields(line), " ")
            parts := strings.Split(line, "|")
            // Trim каждый элемент
            for i := range parts {
                parts[i] = strings.TrimSpace(parts[i])
            }
            fmt.Printf("[parseSettings] parts: %v\n", parts)
            if len(parts) >= 5 {
                fmt.Sscanf(parts[0], "%d", &settings.ActiveModelID)
                fmt.Sscanf(parts[1], "%d", &settings.ActiveProjectID)
                fmt.Sscanf(parts[2], "%d", &settings.DefaultSessionID)
                settings.Theme = parts[3]
                var streaming int
                fmt.Sscanf(parts[4], "%d", &streaming)
                settings.StreamingEnabled = streaming == 1
                fmt.Printf("[parseSettings] parsed: ActiveModelID=%d, ActiveProjectID=%d, Streaming=%v\n", settings.ActiveModelID, settings.ActiveProjectID, settings.StreamingEnabled)
                return &settings
            }
        }
    }
    return &settings
}

func escapeSQL(s string) string {
    return strings.ReplaceAll(strings.ReplaceAll(s, "'", "''"), "\n", " ")
}