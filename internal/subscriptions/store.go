// Хранилище пользовательских подписок на внешние провайдеры (Z.ai/Anthropic/OpenAI).
// Юзер может подключить несколько и переключаться между ними. Базовый
// тариф ExecAI остаётся отдельной опцией (active="" или active="execai").
//
// Файл: ~/.config/execai/subscriptions.json. Права 0600.
package subscriptions

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/config"
)

// Subscription — учётка одного провайдера.
type Subscription struct {
	Provider    string    `json:"provider"`     // "zai" | "anthropic" | "openai"
	APIKey      string    `json:"api_key"`      // bearer-токен для провайдерского API
	BaseURL     string    `json:"base_url,omitempty"` // override (напр. open.bigmodel.cn для CN)
	Plan        string    `json:"plan,omitempty"`     // напр. "coding" для Z.ai
	// Доступные ID моделей на подписке — заполняется при /connect опросом
	// провайдерского /models endpoint'а. Пусто если endpoint не поддерживается.
	AvailableModels []string  `json:"available_models,omitempty"`
	ConnectedAt time.Time `json:"connected_at"`
}

// Source — где взять модель: ExecAI gateway или внешняя подписка.
const (
	SourceExecAI    = "execai" // дефолтный путь через api.execai.ru → aicore-vbai → биллинг
	SourceZAI       = "zai"
	// SourceOpenAI = OpenAI Platform pay-per-token API-key (platform.openai.com).
	// Endpoint: api.openai.com/v1. Модели: gpt-4o, o3, o4-mini и др.
	SourceOpenAI    = "openai"
	// SourceCodexCLI = делегирование в локальный OpenAI Codex CLI (`codex` binary).
	// ChatGPT Plus/Pro OAuth-подписка, без отдельного API-key. Аналог claude-cli.
	SourceCodexCLI  = "codex-cli"
	// SourceKimi = Kimi Code (kimi.com/code) — подписка Coding Plan.
	// Endpoint: api.kimi.com/coding. Модели: k3, kimi-for-coding.
	SourceKimi      = "kimi"
	// SourceKimiAPI = Moonshot Platform (platform.moonshot.ai) — pay-per-token.
	// Endpoint: api.moonshot.ai/v1. Модели: kimi-latest, kimi-k2-turbo-preview, moonshot-v1-*.
	SourceKimiAPI   = "kimi-api"
	SourceAnthropic = "anthropic"
	// SourceClaudeCLI — делегирование в локальный `claude` CLI (Claude Code).
	// Использует OAuth-сессию из Pro/Max-подписки юзера, без отдельного ключа.
	SourceClaudeCLI = "claude-cli"
	// SourceOllama — локальный runner ollama.com. API-key не нужен, base_url
	// по умолчанию http://localhost:11434, каталог моделей динамический
	// (через GET /api/tags), билинг 0 ₽.
	SourceOllama = "ollama"
	// SourceOpenAI = "openai"  // TODO когда они откроют официальный OAuth/API для Plus
)

// Store — все подписки + какая активна.
type Store struct {
	Subscriptions map[string]Subscription `json:"subscriptions"` // ключ = provider
	Active        string                  `json:"active"`        // "" или "execai" = базовый ExecAI; иначе ключ подписки
}

func filePath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "subscriptions.json"), nil
}

// Load читает store из диска. Возвращает пустой Store если файла нет.
func Load() (*Store, error) {
	path, err := filePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Store{Subscriptions: map[string]Subscription{}}, nil
		}
		return nil, err
	}
	var s Store
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse subscriptions.json: %w", err)
	}
	if s.Subscriptions == nil {
		s.Subscriptions = map[string]Subscription{}
	}
	return &s, nil
}

// Save пишет атомарно (через temp + rename), права 0600.
func (s *Store) Save() error {
	path, err := filePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Add вставляет/обновляет подписку. ConnectedAt автоматически.
func (s *Store) Add(sub Subscription) {
	if s.Subscriptions == nil {
		s.Subscriptions = map[string]Subscription{}
	}
	sub.ConnectedAt = time.Now()
	s.Subscriptions[sub.Provider] = sub
}

// Remove удаляет подписку. Если она была active — переключаемся на execai.
func (s *Store) Remove(provider string) {
	delete(s.Subscriptions, provider)
	if s.Active == provider {
		s.Active = SourceExecAI
	}
}

// Activate переключает источник на указанный provider. "" или "execai" = базовый ExecAI.
func (s *Store) Activate(provider string) error {
	if provider == "" || provider == SourceExecAI {
		s.Active = SourceExecAI
		return nil
	}
	if _, ok := s.Subscriptions[provider]; !ok {
		return fmt.Errorf("подписка %q не подключена — сначала /connect %s", provider, provider)
	}
	s.Active = provider
	return nil
}

// ActiveSubscription возвращает активную подписку, или nil если active=execai/пусто.
func (s *Store) ActiveSubscription() *Subscription {
	if s.Active == "" || s.Active == SourceExecAI {
		return nil
	}
	if sub, ok := s.Subscriptions[s.Active]; ok {
		return &sub
	}
	return nil
}

// SourceLabel — человеко-понятное имя текущего источника для status bar.
func (s *Store) SourceLabel() string {
	if s.Active == "" || s.Active == SourceExecAI {
		return "ExecAI"
	}
	if sub, ok := s.Subscriptions[s.Active]; ok {
		// Тариф подписки выводим через анализ доступных моделей — /connect
		// автоматически определил что реально доступно. Точнее чем название плана.
		if tier := deriveKimiTier(sub); tier != "" {
			return fmt.Sprintf("%s (%s)", sub.Provider, tier)
		}
		if sub.Plan != "" {
			return fmt.Sprintf("%s (%s)", sub.Provider, sub.Plan)
		}
		return sub.Provider
	}
	return s.Active
}

// deriveKimiTier — по списку доступных на подписке моделей вычисляет условное
// название тарифа Kimi Code. Отражает то ЧТО РЕАЛЬНО ДОСТУПНО, а не название плана.
// Порядок доступов (по документации www.kimi.com/code/docs/en/kimi-code/models.html):
//   * kimi-for-coding                          → минимум (базовый план)
//   * + k3                                     → Moderato+
//   * + kimi-for-coding-highspeed              → Allegretto+
// Обычно верхние тарифы (Allegro, Presto, Vivace) отдают все три.
func deriveKimiTier(sub Subscription) string {
	if sub.Provider != SourceKimi || len(sub.AvailableModels) == 0 {
		return ""
	}
	has := map[string]bool{}
	for _, id := range sub.AvailableModels {
		has[id] = true
	}
	switch {
	case has["k3"] && has["kimi-for-coding-highspeed"]:
		return "K3 + HighSpeed"
	case has["k3"]:
		return "K3"
	case has["kimi-for-coding"]:
		return "K2.7 Code"
	}
	return ""
}

// List возвращает отсортированный список подключенных провайдеров для UI.
func (s *Store) List() []Subscription {
	out := make([]Subscription, 0, len(s.Subscriptions))
	for _, sub := range s.Subscriptions {
		out = append(out, sub)
	}
	return out
}
