package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Конфиг хранится в os.UserConfigDir() / "execai":
//   config.json      — публичные настройки (api_base, selected_model_id)
//   credentials.json — JWT и email (mode 0600)
//
// На Linux  — ~/.config/execai/
// На Windows — %APPDATA%\execai\
// На macOS  — ~/Library/Application Support/execai/

type Config struct {
	APIBase         string `json:"api_base"`
	SelectedModelID string `json:"selected_model_id,omitempty"`
	// ThinkingBudget — бюджет токенов на chain-of-thought (Anthropic-compat
	// провайдеры, GLM-5.2 thinking, Claude extended thinking). 0 = off.
	ThinkingBudget int `json:"thinking_budget,omitempty"`
	// MaxIterations — сколько tool-use итераций подряд агент может сделать
	// в одном ходе. Когда исчерпан — вставляется мягкий stop-маркер, юзер
	// может сказать 'продолжай' и loop поедет ещё столько же. 0 = дефолт (40).
	MaxIterations int `json:"max_iterations,omitempty"`
	// ClassicTUI — opt-in classic режим (alt-screen + mouse capture, прибитый
	// снизу статус-бар, Shift+drag для копирования). По умолчанию false =
	// Ink-style рендеринг: история пишется в терминальный scrollback через
	// tea.Println, native selection и scroll работают, только input+статус
	// в динамическом View().
	ClassicTUI bool `json:"classic_tui,omitempty"`
	// InlineMode — DEPRECATED alias для !ClassicTUI. Читается для JSON-совместимости.
	InlineMode bool `json:"inline_mode,omitempty"`
}

// GetMaxIterations возвращает эффективное значение — либо из конфига,
// либо дефолт если не задано.
func (c *Config) GetMaxIterations() int {
	if c == nil || c.MaxIterations <= 0 {
		return 40
	}
	return c.MaxIterations
}

type Credentials struct {
	Token   string `json:"token"`
	Email   string `json:"email"`
	SavedAt string `json:"saved_at"`

	// Для агентов (device-flow login). Browser-сессии этих полей не имеют.
	AgentID   string `json:"agent_id,omitempty"`
	Alias     string `json:"alias,omitempty"`     // "yz-laptop"
	AgentType string `json:"agent_type,omitempty"` // "execai-cli"
}

func Defaults() *Config {
	return &Config{
		APIBase: "https://api.execai.ru",
	}
}

// Dir возвращает путь до директории с конфигом. Если есть legacy-папка
// от старого бинаря (agent-vbai), переносит её содержимое в новую.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "execai")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	migrateLegacy(base, dir)
	return dir, nil
}

// migrateLegacy — одноразовая миграция из старой папки ~/.config/agent-vbai/
// в новую ~/.config/execai/. Ключевое: после первой успешной миграции
// legacy-папку УДАЛЯЕМ, иначе после auth.Logout() (который удаляет
// creds в новой папке) следующий вызов Dir() восстановит СТАРЫЕ,
// протухшие creds из legacy — device-flow login впустую (BUG-3, 2026-07-03).
func migrateLegacy(base, newDir string) {
	old := filepath.Join(base, "agent-vbai")
	st, err := os.Stat(old)
	if err != nil || !st.IsDir() {
		return
	}
	migrated := false
	for _, name := range []string{"config.json", "credentials.json"} {
		from := filepath.Join(old, name)
		to := filepath.Join(newDir, name)
		if _, err := os.Stat(to); err == nil {
			continue // в новой папке уже есть — не перезаписываем
		}
		if data, err := os.ReadFile(from); err == nil {
			if err := os.WriteFile(to, data, 0o600); err == nil {
				migrated = true
			}
		}
	}
	// После первой миграции сносим legacy — чтобы не восстанавливалось
	// после Logout. Если ничего не мигрировали (target файлы уже все были) —
	// тоже сносим, миграция не нужна.
	_ = os.RemoveAll(old)
	_ = migrated // явно игнорируем — снос делаем в любом случае
}

func Load() (*Config, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "config.json")

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		c := Defaults()
		if err := Save(c); err != nil {
			return nil, err
		}
		return c, nil
	}
	if err != nil {
		return nil, err
	}

	c := Defaults()
	if err := json.Unmarshal(data, c); err != nil {
		return nil, fmt.Errorf("config.json: %w", err)
	}
	return c, nil
}

func Save(c *Config) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "config.json"), data, 0o600)
}

func LoadCredentials() (*Credentials, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "credentials.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var cr Credentials
	if err := json.Unmarshal(data, &cr); err != nil {
		return nil, fmt.Errorf("credentials.json: %w", err)
	}
	return &cr, nil
}

func SaveCredentials(cr *Credentials) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(cr, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "credentials.json"), data, 0o600)
}

func DeleteCredentials() error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	err = os.Remove(filepath.Join(dir, "credentials.json"))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
