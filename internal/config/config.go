package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config is stored in os.UserConfigDir() / "execai":
//   config.json      — public settings (api_base, selected_model_id)
//   credentials.json — JWT and email (mode 0600)
//
// On Linux   — ~/.config/execai/
// On Windows — %APPDATA%\execai\
// On macOS   — ~/Library/Application Support/execai/

type Config struct {
	APIBase         string `json:"api_base"`
	SelectedModelID string `json:"selected_model_id,omitempty"`
	// ThinkingBudget is the token budget for chain-of-thought (Anthropic-compat
	// providers, GLM-5.2 thinking, Claude extended thinking). 0 = off.
	ThinkingBudget int `json:"thinking_budget,omitempty"`
	// MaxIterations is how many consecutive tool-use iterations the agent may
	// run in a single turn. When exhausted, a soft stop marker is inserted; the
	// user can say 'continue' and the loop runs the same amount again. 0 = default (50).
	MaxIterations int `json:"max_iterations,omitempty"`
	// ClassicTUI is the opt-in classic mode (alt-screen + mouse capture, status
	// bar pinned to the bottom, Shift+drag to copy). Default false =
	// Ink-style rendering: history is written to the terminal scrollback via
	// tea.Println, native selection and scroll work, only input+status live
	// in the dynamic View().
	ClassicTUI bool `json:"classic_tui,omitempty"`
	// InlineMode is a DEPRECATED alias for !ClassicTUI. Read for JSON compatibility.
	InlineMode bool `json:"inline_mode,omitempty"`
	// Locale is the UI language code ("en"/"ru"/"es"/"de"/"zh"). Empty = auto-detect from $LANG.
	// Set explicitly via the /lang <code> command.
	Locale string `json:"locale,omitempty"`
}

// GetMaxIterations returns the effective value — either from the config,
// or the default when unset.
func (c *Config) GetMaxIterations() int {
	if c == nil || c.MaxIterations <= 0 {
		return 50
	}
	return c.MaxIterations
}

type Credentials struct {
	Token   string `json:"token"`
	Email   string `json:"email"`
	SavedAt string `json:"saved_at"`

	// For agents (device-flow login). Browser sessions do not have these fields.
	AgentID   string `json:"agent_id,omitempty"`
	Alias     string `json:"alias,omitempty"`     // "yz-laptop"
	AgentType string `json:"agent_type,omitempty"` // "execai-cli"
}

func Defaults() *Config {
	return &Config{
		APIBase: "https://api.execai.ru",
	}
}

// Dir returns the path to the config directory. If a legacy directory from
// the old binary (agent-vbai) exists, its contents are moved into the new one.
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

// migrateLegacy is a one-time migration from the old ~/.config/agent-vbai/
// directory to the new ~/.config/execai/. Crucially: after the first
// successful migration we DELETE the legacy directory, otherwise after
// auth.Logout() (which removes creds in the new directory) the next Dir()
// call would restore the OLD, stale creds from legacy — making the
// device-flow login pointless (BUG-3, 2026-07-03).
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
			continue // already present in the new directory — do not overwrite
		}
		if data, err := os.ReadFile(from); err == nil {
			if err := os.WriteFile(to, data, 0o600); err == nil {
				migrated = true
			}
		}
	}
	// After the first migration remove the legacy directory — so nothing gets
	// restored after Logout. If nothing was migrated (all target files already
	// existed) — remove it too, migration is not needed.
	_ = os.RemoveAll(old)
	_ = migrated // explicitly ignored — we remove the directory either way
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
