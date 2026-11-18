package config

import (
	"strings"

	"encoding/json"
	"fmt"
	"github.com/velesbsdllc/agent-vbai/internal/notify"
	"github.com/velesbsdllc/agent-vbai/internal/security"
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
	// SecurityLevel — профиль доверия: light | deep | paranoid.
	// Пусто = deep. Определяет, что агент делает молча, а что спрашивает:
	// выход за каталог проекта, чтение секретов, сетевые запросы.
	// Дыры уровнем не регулируются — их нет ни на одном (см. internal/security).
	SecurityLevel string `json:"security_level,omitempty"`
	// AutoCompactPercent — порог автосжатия истории в процентах окна
	// контекста. 0 = дефолт (80), -1 = выключено. Команда /autocompact.
	AutoCompactPercent int `json:"auto_compact_percent,omitempty"`

	// MaxIterations is how many consecutive tool-use iterations the agent may
	// run in a single turn. When exhausted, a soft stop marker is inserted; the
	// user can say 'continue' and the loop runs the same amount again. 0 = default (50).
	MaxIterations int `json:"max_iterations,omitempty"`
	// SubagentIterations — steps a new subagent gets when the Task call names
	// none (/agents limit, the panel menu). 0 = tools.SubagentDefaultIterations.
	SubagentIterations int `json:"subagent_iterations,omitempty"`
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
	// Notifications — how the terminal chat tells the person that a turn
	// has finished or the agent waits for them while they are in another
	// window: system | bell | off; empty = system. Set with /notifications.
	// It belongs to this computer: sync leaves it out (internal/sync,
	// MachineOnlyKeys).
	Notifications string `json:"notifications,omitempty"`
	// ModelAuto — the model the agent picked itself (the catalog's default,
	// or the source's when the chosen one is gone), not the person: while
	// SelectedModelID equals it, the model is not a setting of this
	// computer, and joining sync does not keep a copy for it. This computer
	// only (internal/sync, MachineOnlyKeys).
	ModelAuto string `json:"model_auto,omitempty"`
	// UIBrowser — the browser execai ui opens its window with: a name
	// (chrome, chromium, edge, yandex, brave, vivaldi), a command or a path.
	// Empty = the default browser when it can show an app window, else the
	// first suitable one found. EXECAI_UI_BROWSER wins over it. It names a
	// program of this computer: sync leaves it out (internal/sync,
	// MachineOnlyKeys).
	UIBrowser string `json:"ui_browser,omitempty"`
}

// SetAutoModel records a model the agent picked itself; a model the person
// chooses is set with ChooseModel.
func (c *Config) SetAutoModel(id string) {
	c.SelectedModelID = id
	c.ModelAuto = id
}

// ChooseModel records a model the person chose: it is their setting, also
// when it is the one the agent had picked itself, and sync carries it to
// their other computers.
func (c *Config) ChooseModel(id string) {
	c.SelectedModelID = id
	c.ModelAuto = ""
}

// GetNotifications — the effective notifications mode.
func (c *Config) GetNotifications() notify.Mode {
	if c == nil {
		return notify.ModeSystem
	}
	m, _ := notify.ParseMode(c.Notifications)
	return m
}

// GetMaxIterations returns the effective value — either from the config,
// or the default when unset.
func (c *Config) GetMaxIterations() int {
	if c == nil || c.MaxIterations <= 0 {
		return 50
	}
	return c.MaxIterations
}

// GetAutoCompactPercent — действующий порог автосжатия (0 = выключено).
// Дефолт 80%: раньше — жалко живого контекста, позже — тяжёлый ход
// (вложения, длинный вывод инструмента) упрётся в потолок до сжатия.
func (c *Config) GetAutoCompactPercent() int {
	if c == nil || c.AutoCompactPercent == 0 {
		return 80
	}
	if c.AutoCompactPercent < 0 {
		return 0
	}
	if c.AutoCompactPercent < 50 {
		return 50 // ниже — сжатие зациклится: пересказ+хвост сами весят проценты
	}
	if c.AutoCompactPercent > 95 {
		return 95
	}
	return c.AutoCompactPercent
}

type Credentials struct {
	Token   string `json:"token"`
	Email   string `json:"email"`
	SavedAt string `json:"saved_at"`

	// For agents (device-flow login). Browser sessions do not have these fields.
	AgentID   string `json:"agent_id,omitempty"`
	Alias     string `json:"alias,omitempty"`      // "yz-laptop"
	AgentType string `json:"agent_type,omitempty"` // "execai-cli"
	// WebBase — the address of the web chat this login was confirmed on
	// (taken from the device-flow verify_uri). The links the agent prints for
	// the browser and the phone start with it; empty for older logins.
	WebBase string `json:"web_base,omitempty"`
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
// directory to the new ~/.config/execai/: config.json and credentials.json
// are copied where the new directory has none, and then the old directory
// is moved aside — renamed to agent-vbai.migrated (-2, -3… when taken) —
// so that it is never migrated again: after auth.Logout() (which removes
// creds in the new directory) the next Dir() call would otherwise restore
// the OLD, stale creds (BUG-3, 2026-07-03).
//
// Nothing is deleted. Each copy is written to a temporary file next to its
// target and renamed over it, so a target is either whole or absent; the
// old directory is moved only after every copy it needs went through. An
// interruption anywhere — a crash, a full disk, an unreadable file — leaves
// the old directory where it was, and the next call goes on from there:
// targets already copied are left alone. Whatever else the old directory
// holds stays in it.
func migrateLegacy(base, newDir string) {
	old := filepath.Join(base, "agent-vbai")
	st, err := os.Stat(old) // a link to a folder is moved as a link
	if err != nil || !st.IsDir() {
		return
	}
	for _, name := range []string{"config.json", "credentials.json"} {
		from := filepath.Join(old, name)
		to := filepath.Join(newDir, name)
		if _, err := os.Lstat(to); err == nil {
			continue // already present in the new directory — not overwritten
		}
		data, err := os.ReadFile(from)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil || writeAtomic(to, data, 0o600) != nil {
			return // the old directory stays; the next call tries again
		}
	}
	aside := old + ".migrated"
	for i := 2; ; i++ {
		if _, err := os.Lstat(aside); os.IsNotExist(err) {
			break
		}
		aside = fmt.Sprintf("%s.migrated-%d", old, i)
	}
	_ = os.Rename(old, aside)
}

// writeAtomic writes data to path through a temporary file in the same
// directory renamed over it: path is either whole or as it was.
func writeAtomic(path string, data []byte, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
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
		applySecurity(c)
		return c, nil
	}
	if err != nil {
		return nil, err
	}

	c := Defaults()
	if err := json.Unmarshal(data, c); err != nil {
		return nil, fmt.Errorf("config.json: %w", err)
	}
	applySecurity(c)
	return c, nil
}

// applySecurity ставит уровень доверия процессу.
//
// Делается здесь, а не в каждой команде: config.Load() вызывают из десятка
// мест (chat, serve, ide, run, cron…), и уровень, забытый в одном из них,
// означал бы режим без защиты — ровно там, где о нём никто не подумал.
//
// Переменная окружения перекрывает файл: так уровень задаётся для одного
// запуска (в том числе автотестами), не трогая конфиг человека.
func applySecurity(c *Config) {
	level := c.SecurityLevel
	if env := strings.TrimSpace(os.Getenv("EXECAI_SECURITY")); env != "" {
		level = env
	}
	security.Set(security.Parse(level))
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
