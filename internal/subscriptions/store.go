// Store of user subscriptions to external providers (Z.ai/Anthropic/OpenAI).
// The user can connect several and switch between them. The base ExecAI
// plan remains a separate option (active="" or active="execai").
//
// File: ~/.config/execai/subscriptions.json. Mode 0600.
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

// Subscription is the account for a single provider.
type Subscription struct {
	Provider string `json:"provider"`           // "zai" | "anthropic" | "openai"
	APIKey   string `json:"api_key"`            // bearer token for the provider API
	BaseURL  string `json:"base_url,omitempty"` // override (e.g. open.bigmodel.cn for CN)
	Plan     string `json:"plan,omitempty"`     // e.g. "coding" for Z.ai
	// Model IDs available on the subscription — filled during /connect by
	// querying the provider's /models endpoint. Empty if the endpoint is unsupported.
	AvailableModels []string `json:"available_models,omitempty"`
	// When that list was fetched — an empty value means "unknown, refetch".
	// Without it a plan upgrade stayed invisible: the list was only ever
	// fetched while it was empty.
	ModelsFetchedAt time.Time `json:"models_fetched_at,omitempty"`
	ConnectedAt     time.Time `json:"connected_at"`
}

// Source — where to get the model from: the ExecAI gateway or an external subscription.
const (
	SourceExecAI = "execai" // default path via api.execai.ru → aicore-vbai → billing
	// SourceZAI = Z.ai GLM Coding Plan subscription (z.ai/subscribe).
	// Endpoint: api.z.ai/api/anthropic — Anthropic-compat only.
	//
	// ⚠️ Their usage policy limits the Coding Plan to a closed list of tools
	// (Claude Code, Cursor, Cline, Roo Code, OpenCode, Pi, Crush, Goose…) and
	// execai is not on it: "users may not use their subscription benefits for
	// tools or scenarios outside of this scope", enforcement up to a ban after
	// three violations. Hence the warning on /connect zai and SourceZAIAPI below
	// as the unrestricted path.
	SourceZAI = "zai"
	// SourceZAIAPI = Z.ai open platform, pay-per-token (api.z.ai/api/paas/v4).
	// OpenAI-compatible, NOT limited by the Coding Plan tool list — this is the
	// legitimate way to use GLM models from a self-built agent.
	SourceZAIAPI = "zai-api"
	// SourceOpenAI = OpenAI Platform pay-per-token API key (platform.openai.com).
	// Endpoint: api.openai.com/v1. Models: gpt-4o, o3, o4-mini and others.
	SourceOpenAI = "openai"
	// SourceCodexCLI = delegation to the local OpenAI Codex CLI (`codex` binary).
	// ChatGPT Plus/Pro OAuth subscription, no separate API key. Analogous to claude-cli.
	SourceCodexCLI = "codex-cli"
	// SourceKimi = Kimi Code (kimi.com/code) — Coding Plan subscription.
	// Endpoint: api.kimi.com/coding. Models: k3, kimi-for-coding.
	SourceKimi = "kimi"
	// SourceKimiAPI = Moonshot Platform (platform.moonshot.ai) — pay-per-token.
	// Endpoint: api.moonshot.ai/v1. Models: kimi-latest, kimi-k2-turbo-preview, moonshot-v1-*.
	SourceKimiAPI = "kimi-api"
	// SourceOpenRouter = openrouter.ai — one pay-per-token key for every
	// vendor (Anthropic, OpenAI, Google, DeepSeek, Qwen, Moonshot, Z.ai…).
	// Endpoint: openrouter.ai/api/v1, OpenAI-compatible. Their catalog is
	// several hundred models, so the picker order matters — see internal/catalog.
	SourceOpenRouter = "openrouter"
	SourceAnthropic  = "anthropic"
	// SourceClaudeCLI — delegation to the local `claude` CLI (Claude Code).
	// Uses the OAuth session from the user's Pro/Max subscription, no separate key.
	SourceClaudeCLI = "claude-cli"
	// SourceOllama — local ollama.com runner. No API key needed, base_url
	// defaults to http://localhost:11434, the model catalog is dynamic
	// (via GET /api/tags), billed at 0 ₽.
	SourceOllama = "ollama"
	// SourceOpenAI = "openai"  // TODO once they open an official OAuth/API for Plus
)

// Store — all subscriptions plus which one is active.
type Store struct {
	Subscriptions map[string]Subscription `json:"subscriptions"` // key = provider
	Active        string                  `json:"active"`        // "" or "execai" = base ExecAI; otherwise a subscription key
}

func filePath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "subscriptions.json"), nil
}

// Load reads the store from disk. Returns an empty Store if the file does not exist.
func Load() (*Store, error) {
	path, err := filePath()
	if err != nil {
		return nil, err
	}
	// Ниже важное правило: при ЛЮБОЙ ошибке возвращается пустое, но рабочее
	// хранилище — не nil.
	//
	// Десяток мест в коде пишет `subs, _ := Load()` и сразу зовёт методы. С
	// nil-хранилищем битый или недоступный subscriptions.json превращался в
	// панику на старте — агент не запускался вовсе, вместо того чтобы поднять
	// базовый источник и сказать человеку, что файл сломан. Поймано
	// самопрогоном 15.08 (этап «ход агента»).
	empty := &Store{Subscriptions: map[string]Subscription{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return empty, nil
		}
		return empty, err
	}
	var s Store
	if err := json.Unmarshal(data, &s); err != nil {
		return empty, fmt.Errorf("parse subscriptions.json: %w", err)
	}
	if s.Subscriptions == nil {
		s.Subscriptions = map[string]Subscription{}
	}
	return &s, nil
}

// Save writes atomically (via temp + rename), mode 0600.
func (s *Store) Save() error {
	if s == nil {
		return errors.New("подписки не загружены")
	}
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

// Add inserts/updates a subscription. ConnectedAt is set automatically.
func (s *Store) Add(sub Subscription) {
	if s == nil {
		return
	}
	if s.Subscriptions == nil {
		s.Subscriptions = map[string]Subscription{}
	}
	sub.ConnectedAt = time.Now()
	s.Subscriptions[sub.Provider] = sub
}

// Remove deletes a subscription. If it was active, switch to execai.
func (s *Store) Remove(provider string) {
	if s == nil {
		return
	}
	delete(s.Subscriptions, provider)
	if s.Active == provider {
		s.Active = SourceExecAI
	}
}

// Activate switches the source to the given provider. "" or "execai" = base ExecAI.
func (s *Store) Activate(provider string) error {
	if s == nil {
		return errors.New("подписки не загружены")
	}
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

// ActiveSubscription returns the active subscription, or nil if active is execai/empty.
func (s *Store) ActiveSubscription() *Subscription {
	if s == nil {
		return nil
	}
	if s.Active == "" || s.Active == SourceExecAI {
		return nil
	}
	if sub, ok := s.Subscriptions[s.Active]; ok {
		return &sub
	}
	return nil
}

// ProviderName — how a source is called on screen. The IDs stay as they are
// (they live in config files and in every /connect anyone has typed), but a
// bare "kimi" told nobody which of the two Kimi paths it is: the Coding Plan
// subscription or the pay-per-token platform. Names say it.
func ProviderName(provider string) string {
	switch provider {
	case SourceKimi:
		return "Kimi Code"
	case SourceKimiAPI:
		return "Moonshot"
	case SourceZAI:
		return "Z.ai Coding Plan"
	case SourceZAIAPI:
		return "Z.ai API"
	case SourceOpenAI:
		return "OpenAI"
	case SourceOpenRouter:
		return "OpenRouter"
	case SourceAnthropic:
		return "Anthropic"
	case SourceClaudeCLI:
		return "Claude Code CLI"
	case SourceCodexCLI:
		return "Codex CLI"
	case SourceOllama:
		return "Ollama"
	case SourceExecAI, "":
		return "ExecAI"
	}
	return provider
}

// SourceLabel is a human-friendly name of the current source for the status bar.
func (s *Store) SourceLabel() string {
	if s == nil {
		return "ExecAI"
	}
	if s.Active == "" || s.Active == SourceExecAI {
		return "ExecAI"
	}
	if sub, ok := s.Subscriptions[s.Active]; ok {
		name := ProviderName(sub.Provider)
		// Derive the subscription tier from the available models — /connect
		// automatically detected what is actually accessible. More accurate than the plan name.
		if tier := deriveKimiTier(sub); tier != "" {
			return fmt.Sprintf("%s (%s)", name, tier)
		}
		if sub.Plan != "" {
			return fmt.Sprintf("%s (%s)", name, sub.Plan)
		}
		return name
	}
	return s.Active
}

// deriveKimiTier computes a nominal Kimi Code tier name from the list of
// models available on the subscription. It reflects WHAT IS ACTUALLY
// AVAILABLE, not the plan name.
// Access order (per docs at www.kimi.com/code/docs/en/kimi-code/models.html):
//   - kimi-for-coding                          → minimum (base plan)
//   - + k3                                     → Moderato+
//   - + kimi-for-coding-highspeed              → Allegretto+
//
// Top tiers (Allegro, Presto, Vivace) usually expose all three.
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

// List returns a sorted list of connected providers for the UI.
func (s *Store) List() []Subscription {
	if s == nil {
		return nil
	}
	out := make([]Subscription, 0, len(s.Subscriptions))
	for _, sub := range s.Subscriptions {
		out = append(out, sub)
	}
	return out
}
