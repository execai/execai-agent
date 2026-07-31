// Commands and helpers for managing subscriptions to external providers
// (/connect zai, /use, /subscriptions). Local model cache per source.
package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/velesbsdllc/agent-vbai/internal/version"
	"net/http"
	"strings"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

// makeLLMClient builds an LLM client based on the active subscription.
// If active="" or "execai" → AICoreClient (our gateway).
// Otherwise → the client of the corresponding provider.
func (m *tuiModel) makeLLMClient() llm.StreamingLLM {
	if m.subs != nil {
		if active := m.subs.ActiveSubscription(); active != nil {
			switch active.Provider {
			case subscriptions.SourceZAI:
				// A Z.ai Coding Plan key works ONLY through the Anthropic-compatible
				// endpoint (like Claude Code). Default base = https://api.z.ai/api/anthropic.
				// OpenAI /chat/completions with a Coding Plan key returns 429 "Insufficient balance"
				// because it is billed separately from the subscription.
				base := active.BaseURL
				if base == "" {
					base = "https://api.z.ai/api/anthropic"
				}
				return llm.NewAnthropicClient(base, active.APIKey, m.current.ID, m.cfg.ThinkingBudget)
			case subscriptions.SourceAnthropic:
				// The real Anthropic API — sk-ant-... key from console.anthropic.com.
				// Pay-per-token. Endpoint api.anthropic.com.
				base := active.BaseURL
				if base == "" {
					base = "https://api.anthropic.com"
				}
				return llm.NewAnthropicClient(base, active.APIKey, m.current.ID, m.cfg.ThinkingBudget)
			case subscriptions.SourceKimi:
				// Kimi Code Coding Plan subscription (kimi.com/code).
				// Endpoint: api.kimi.com/coding — Anthropic-compat + thinking.
				base := active.BaseURL
				if base == "" {
					base = "https://api.kimi.com/coding"
				}
				return llm.NewAnthropicClient(base, active.APIKey, m.current.ID, m.cfg.ThinkingBudget)
			case subscriptions.SourceKimiAPI:
				// Moonshot Platform pay-per-token API key (platform.moonshot.ai).
				// Endpoint: api.moonshot.ai/v1 — OpenAI-compat via GLMClient.
				base := active.BaseURL
				if base == "" {
					base = "https://api.moonshot.ai/v1"
				}
				return llm.NewGLMClient(base, active.APIKey, m.current.ID)
			case subscriptions.SourceOpenAI:
				// OpenAI Platform pay-per-token API key.
				// Endpoint: api.openai.com/v1 — OpenAI-compat via GLMClient
				// (ProviderLabel is detected as "OpenAI" from BaseURL).
				base := active.BaseURL
				if base == "" {
					base = "https://api.openai.com/v1"
				}
				return llm.NewGLMClient(base, active.APIKey, m.current.ID)
			case subscriptions.SourceCodexCLI:
				// Delegation to the local `codex` CLI.
				cli, err := llm.NewCodexCLIClient(m.current.ID)
				if err != nil {
					return llm.NewAICoreClient(m.cfg.APIBase, m.credsToken(), m.current.ID, m.current.Provider)
				}
				return cli
			case subscriptions.SourceClaudeCLI:
				// Delegate to the local `claude` CLI. Claude Code OAuth session,
				// quota from the Pro/Max subscription.
				cli, err := llm.NewClaudeCLIClient(m.current.ID)
				if err != nil {
					// Silent fallback — the UI should show that it is unavailable.
					return llm.NewAICoreClient(m.cfg.APIBase, m.credsToken(), m.current.ID, m.current.Provider)
				}
				return cli
			case subscriptions.SourceOllama:
				base := active.BaseURL
				// Cloud (ollama.com) — Anthropic-compatible endpoint, API key required.
				// Local (localhost:11434 or custom) — OpenAI-compat, no key.
				if isOllamaCloud(base, active.Plan) {
					if base == "" {
						base = "https://ollama.com"
					}
					return llm.NewAnthropicClient(base, active.APIKey, m.current.ID, m.cfg.ThinkingBudget)
				}
				if base == "" {
					base = "http://localhost:11434"
				}
				return llm.NewOllamaClient(base, m.current.ID)
			}
		}
	}
	return llm.NewAICoreClient(m.cfg.APIBase, m.credsToken(), m.current.ID, m.current.Provider)
}

// applySubscriptionSource syncs m.models/m.current with the active source
// and recreates m.cli. Called after /use and at boot.
func (m *tuiModel) applySubscriptionSource() {
	if m.subs == nil {
		m.cli = m.makeLLMClient()
		return
	}
	active := m.subs.ActiveSubscription()
	if active == nil {
		// ExecAI — restore the original catalog from the snapshot, otherwise after
		// /source zai → /source execai m.models would keep GLMModels and
		// m.current.Provider would be "zai" → the request would go to /aicore-vbai/agent-stream
		// with a foreign model and provider → 401.
		if len(m.execAIModels) > 0 {
			m.models = m.execAIModels
		}
		m.current = pickForNewCatalog(m.models, m.current)
		m.cfg.SelectedModelID = m.current.ID
		m.cli = m.makeLLMClient()
		return
	}
	// External subscription — replace the model catalog with its own.
	switch active.Provider {
	case subscriptions.SourceZAI:
		m.models = llm.GLMModels()
	case subscriptions.SourceAnthropic:
		m.models = llm.AnthropicModels()
	case subscriptions.SourceKimi:
		m.models = llm.KimiModels()
		// Lazy fetch of the available-models list — for users who connected
		// before this feature existed (or to refresh after a plan upgrade).
		// Once per source, in the background; the UI picks it up after save.
		if len(active.AvailableModels) == 0 {
			go m.refreshKimiAvailability(*active)
		}
	case subscriptions.SourceKimiAPI:
		// Priority: the real model list from the server (filled in /connect).
		// Fallback: hardcoded MoonshotModels() — if the server did not respond or
		// the connection is old (predates fetchKimiAvailableModels).
		if len(active.AvailableModels) > 0 {
			m.models = buildKimiAPIDynamicCatalog(active.AvailableModels)
		} else {
			m.models = llm.MoonshotModels()
			// Try to fetch in the background — the next switch will have fresh data.
			go m.refreshKimiAvailability(*active)
		}
	case subscriptions.SourceOpenAI:
		if len(active.AvailableModels) > 0 {
			m.models = buildOpenAIDynamicCatalog(active.AvailableModels)
		} else {
			m.models = llm.OpenAIModels()
			go m.refreshOpenAIAvailability(*active)
		}
	case subscriptions.SourceCodexCLI:
		m.models = llm.CodexCLIModels()
	case subscriptions.SourceClaudeCLI:
		m.models = llm.ClaudeCLIModels()
	case subscriptions.SourceOllama:
		if isOllamaCloud(active.BaseURL, active.Plan) {
			// Cloud — dynamic catalog: ollama.com serves the same /api/tags
			// behind Bearer auth with the FULL current list (kimi-k3,
			// deepseek-v4-flash, glm-5.2, …). Session cache; hardcoded
			// OllamaCloudModels() only as a network-failure fallback.
			if len(m.ollamaCloudModels) == 0 {
				base := active.BaseURL
				if base == "" {
					base = "https://ollama.com"
				}
				ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
				if ms, err := llm.FetchOllamaModelsAuth(ctx, base, active.APIKey); err == nil && len(ms) > 0 {
					m.ollamaCloudModels = pickOllamaCloudPrimary(ms)
				}
				cancel()
			}
			if len(m.ollamaCloudModels) > 0 {
				m.models = m.ollamaCloudModels
			} else {
				m.models = llm.OllamaCloudModels()
			}
		} else {
			// Local — dynamic via /api/tags. Cache kept for the session.
			if len(m.ollamaModels) == 0 {
				base := active.BaseURL
				if base == "" {
					base = "http://localhost:11434"
				}
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				if ms, err := llm.FetchOllamaModels(ctx, base); err == nil {
					m.ollamaModels = ms
				}
				cancel()
			}
			m.models = m.ollamaModels
		}
	default:
		// Unknown provider — fallback to the ExecAI catalog, but the client may not have worked.
	}
	m.current = pickForNewCatalog(m.models, m.current)
	m.cfg.SelectedModelID = m.current.ID
	m.cli = m.makeLLMClient()
}

// pickForNewCatalog picks a Model from the new catalog:
//   - if there is an entry with the same ID — take THAT one (important: identical IDs
//     have different Providers, e.g. glm-5.2 from zai vs ollama-cloud);
//   - otherwise — primary.
//
// If the catalog is empty — leave current as is.
func pickForNewCatalog(catalog []llm.Model, current llm.Model) llm.Model {
	if len(catalog) == 0 {
		return current
	}
	for _, mm := range catalog {
		if mm.ID == current.ID {
			return mm
		}
	}
	return pickPrimary(catalog)
}

// sourceSupportsThinking — whether effort/thinking can be tuned for the current source.
// Anthropic-compat clients (zai, kimi, anthropic, ollama-cloud, claude-cli) — yes.
// OpenAI-compat local ollama and our ExecAI gateway — no (for now).
func sourceSupportsThinking(s *subscriptions.Store) bool {
	if s == nil {
		return false
	}
	switch s.Active {
	case subscriptions.SourceZAI, subscriptions.SourceAnthropic, subscriptions.SourceKimi, subscriptions.SourceClaudeCLI:
		return true
	case subscriptions.SourceOllama:
		if sub, ok := s.Subscriptions[subscriptions.SourceOllama]; ok {
			return isOllamaCloud(sub.BaseURL, sub.Plan)
		}
	}
	return false
}

// isOllamaCloud — heuristic: ollama.com/api.ollama.com or plan="cloud".
// Everything else is treated as local (localhost, LAN IP, custom domain).
func isOllamaCloud(baseURL, plan string) bool {
	if plan == "cloud" {
		return true
	}
	low := strings.ToLower(baseURL)
	return strings.Contains(low, "ollama.com")
}

func modelInCatalog(models []llm.Model, id string) bool {
	for _, mm := range models {
		if mm.ID == id {
			return true
		}
	}
	return false
}

func pickPrimary(models []llm.Model) llm.Model {
	for _, mm := range models {
		if mm.IsPrimary {
			return mm
		}
	}
	return models[0]
}

// handleSubsCommand handles /connect, /disconnect, /use, /subscriptions.
// Returns (handled, statusMessage). If handled=false — the command is not ours.
func (m *tuiModel) handleSubsCommand(cmd string) (bool, string) {
	switch {
	case cmd == "/subscriptions" || cmd == "/subs":
		return true, m.formatSubsList()

	case strings.HasPrefix(cmd, "/connect "):
		provider := strings.TrimSpace(strings.TrimPrefix(cmd, "/connect"))
		return true, m.startConnectFlow(provider)

	case strings.HasPrefix(cmd, "/disconnect "):
		provider := strings.TrimSpace(strings.TrimPrefix(cmd, "/disconnect"))
		if m.subs == nil {
			return true, i18n.T("subs.noSubs")
		}
		if _, ok := m.subs.Subscriptions[provider]; !ok {
			return true, i18n.Tf("subs.disconnect.notFound", provider)
		}
		m.subs.Remove(provider)
		if err := m.subs.Save(); err != nil {
			return true, i18n.Tf("subs.saveError", err)
		}
		m.applySubscriptionSource()
		return true, i18n.Tf("subs.disconnected", provider)

	case cmd == "/source" || cmd == "/use":
		// Bare /source — list of options. /use is kept as a deprecated alias.
		var b strings.Builder
		if cmd == "/use" {
			b.WriteString(i18n.T("subs.useDeprecated"))
		}
		b.WriteString(i18n.T("subs.source.listHeader"))
		b.WriteString(i18n.T("subs.source.listExecai"))
		if m.subs != nil {
			for _, sub := range m.subs.List() {
				b.WriteString(i18n.Tf("subs.source.listItem", sub.Provider, sub.Plan))
			}
		}
		b.WriteString(i18n.T("subs.source.listFooter"))
		return true, b.String()

	case strings.HasPrefix(cmd, "/source ") || strings.HasPrefix(cmd, "/use "):
		// Both prefixes are supported. /use is a deprecated alias.
		var target string
		if strings.HasPrefix(cmd, "/source ") {
			target = strings.TrimSpace(strings.TrimPrefix(cmd, "/source"))
		} else {
			target = strings.TrimSpace(strings.TrimPrefix(cmd, "/use"))
		}
		if m.subs == nil {
			m.subs, _ = subscriptions.Load()
		}
		if err := m.subs.Activate(target); err != nil {
			// Convenient how-to for specific providers.
			extra := ""
			switch target {
			case "zai":
				extra = i18n.T("subs.howto.zai")
			case "anthropic":
				extra = i18n.T("subs.howto.anthropic")
			}
			return true, "✗ " + err.Error() + extra
		}
		if err := m.subs.Save(); err != nil {
			return true, i18n.Tf("subs.saveError", err)
		}
		m.applySubscriptionSource()
		// No-login mode: switching to an external subscription while not logged
		// in to ExecAI exits the login screen — an ExecAI account is optional.
		if m.loginMode && m.subs.ActiveSubscription() != nil {
			m.loginMode = false
			m.textarea.Placeholder = i18n.T("placeholder.chat")
		}
		return true, i18n.Tf("subs.sourceSwitched", m.subs.SourceLabel(), m.current.ID)

	case cmd == "/connect":
		return true, i18n.T("subs.connect.usageShort")

	case cmd == "/disconnect":
		if m.subs == nil || len(m.subs.Subscriptions) == 0 {
			return true, i18n.T("subs.disconnect.none")
		}
		var b strings.Builder
		b.WriteString(i18n.T("subs.disconnect.header"))
		for _, sub := range m.subs.List() {
			fmt.Fprintf(&b, "  • %s\n", sub.Provider)
		}
		return true, b.String()
	}
	return false, ""
}

// formatSubsList — compact output for /subscriptions.
func (m *tuiModel) formatSubsList() string {
	if m.subs == nil || len(m.subs.Subscriptions) == 0 {
		return i18n.T("subs.list.empty")
	}
	var b strings.Builder
	b.WriteString(i18n.Tf("subs.list.header", m.subs.SourceLabel()))
	for _, sub := range m.subs.List() {
		mark := "  "
		if sub.Provider == m.subs.Active {
			mark = "● "
		}
		fmt.Fprintf(&b, "%s%-12s  plan=%s  connected=%s\n",
			mark, sub.Provider, sub.Plan, sub.ConnectedAt.Format("2006-01-02"))
	}
	b.WriteString(i18n.T("subs.list.switchHint"))
	b.WriteString(i18n.T("subs.list.disconnectHint"))
	return b.String()
}

// filterUseOptions — what to suggest for /source <arg> (and deprecated /use).
// Base ExecAI is always present. Connected subscriptions too.
// cmd — the specific slash command the user typed (to preserve the prefix).
func filterUseOptions(s *subscriptions.Store, prefix string) []suggestItem {
	return filterUseOptionsFor(s, prefix, "/source")
}

func filterUseOptionsFor(s *subscriptions.Store, prefix, cmdPrefix string) []suggestItem {
	low := strings.ToLower(prefix)
	// Known providers (order = how they appear in the menu).
	// desc — an i18n key; renderSuggestPanel translates hints via i18n.T().
	knownProviders := []struct {
		name string
		desc string
	}{
		{"execai", "subs.provider.execai"},
		{"zai", "subs.provider.zai"},
		{"kimi", "subs.provider.kimi"},
		{"kimi-api", "subs.provider.kimiapi"},
		{"anthropic", "subs.provider.anthropic"},
		{"openai", "subs.provider.openai"},
		{"codex-cli", "subs.provider.codexcli"},
		{"claude-cli", "subs.provider.claudecli"},
		{"ollama", "subs.provider.ollama"},
	}
	all := []suggestItem{}
	connected := map[string]subscriptions.Subscription{}
	if s != nil {
		for _, sub := range s.List() {
			connected[sub.Provider] = sub
		}
	}
	for _, p := range knownProviders {
		if p.name == "execai" {
			mark := ""
			if s == nil || s.Active == "" || s.Active == "execai" {
				mark = "● "
			}
			all = append(all, suggestItem{
				insert: cmdPrefix + " execai",
				label:  mark + "execai",
				hint:   p.desc,
			})
			continue
		}
		if sub, ok := connected[p.name]; ok {
			// Pre-translated (dynamic plan value) — i18n.T at render is a no-op.
			h := i18n.T("subs.hint.connected")
			if sub.Plan != "" {
				h = i18n.Tf("subs.hint.connectedPlan", sub.Plan)
			}
			mark := ""
			if s != nil && s.Active == p.name {
				mark = "● "
			}
			all = append(all, suggestItem{
				insert: cmdPrefix + " " + p.name,
				label:  mark + p.name,
				hint:   h,
			})
		} else {
			// Not connected — show as a hint; selecting it will suggest /connect.
			all = append(all, suggestItem{
				insert: cmdPrefix + " " + p.name,
				label:  p.name,
				hint:   i18n.Tf("subs.hint.notConnected", p.name),
			})
		}
	}
	out := make([]suggestItem, 0, len(all))
	for _, it := range all {
		if low == "" || strings.Contains(strings.ToLower(it.label), low) {
			out = append(out, it)
		}
	}
	return out
}

// filterConnectOptions — what to suggest for /connect <arg>.
// Only zai for now. anthropic/openai will be added when implemented.
func filterConnectOptions(prefix string) []suggestItem {
	low := strings.ToLower(prefix)
	// hint — an i18n key; renderSuggestPanel translates via i18n.T().
	all := []suggestItem{
		{insert: "/connect zai ", label: "zai", hint: "subs.connectHint.zai"},
		{insert: "/connect kimi ", label: "kimi", hint: "subs.connectHint.kimi"},
		{insert: "/connect kimi-api ", label: "kimi-api", hint: "subs.connectHint.kimiapi"},
		{insert: "/connect anthropic ", label: "anthropic", hint: "subs.connectHint.anthropic"},
		{insert: "/connect openai ", label: "openai", hint: "subs.connectHint.openai"},
		{insert: "/connect codex-cli", label: "codex-cli", hint: "subs.connectHint.codexcli"},
		{insert: "/connect claude-cli", label: "claude-cli", hint: "subs.connectHint.claudecli"},
		{insert: "/connect ollama", label: "ollama", hint: "subs.connectHint.ollama"},
	}
	out := make([]suggestItem, 0, len(all))
	for _, it := range all {
		if low == "" || strings.Contains(strings.ToLower(it.label), low) {
			out = append(out, it)
		}
	}
	return out
}

// filterDisconnectOptions — connected ones only.
func filterDisconnectOptions(s *subscriptions.Store, prefix string) []suggestItem {
	if s == nil {
		return nil
	}
	low := strings.ToLower(prefix)
	out := []suggestItem{}
	for _, sub := range s.List() {
		if low == "" || strings.Contains(strings.ToLower(sub.Provider), low) {
			out = append(out, suggestItem{
				insert: "/disconnect " + sub.Provider,
				label:  sub.Provider,
				hint:   "subs.hint.remove", // i18n key, translated at render
			})
		}
	}
	return out
}

// startConnectFlow — for the MVP, synchronous via env variables or a hint.
// Z.ai: we expect the user to send the command with an API key, e.g. /connect zai <key>
// or just /connect zai → we explain how to do it.
func (m *tuiModel) startConnectFlow(arg string) string {
	parts := strings.Fields(arg)
	if len(parts) == 0 {
		return i18n.T("subs.connect.usage")
	}
	provider := parts[0]
	// claude-cli — special case: no key needed, only a check that the binary is in PATH.
	if provider == "claude-cli" {
		if _, err := llm.NewClaudeCLIClient(""); err != nil {
			return "✗ " + err.Error()
		}
		if m.subs == nil {
			m.subs, _ = subscriptions.Load()
		}
		m.subs.Add(subscriptions.Subscription{
			Provider: "claude-cli",
			APIKey:   "",
			Plan:     "Pro/Max (OAuth)",
		})
		if err := m.subs.Save(); err != nil {
			return i18n.Tf("subs.saveError", err)
		}
		return i18n.T("subs.connect.claudecliOK")
	}
	// codex-cli — same thing for the OpenAI Codex CLI (`codex` binary).
	if provider == "codex-cli" {
		if _, err := llm.NewCodexCLIClient(""); err != nil {
			return "✗ " + err.Error()
		}
		if m.subs == nil {
			m.subs, _ = subscriptions.Load()
		}
		m.subs.Add(subscriptions.Subscription{
			Provider: "codex-cli",
			APIKey:   "",
			Plan:     "ChatGPT Plus/Pro (OAuth)",
		})
		if err := m.subs.Save(); err != nil {
			return i18n.Tf("subs.saveError", err)
		}
		return i18n.T("subs.connect.codexcliOK")
	}
	// ollama — 2 modes:
	//   * Cloud (default): https://ollama.com, Anthropic-compat, API key required.
	//     Models like glm-5.2, qwen3-coder etc. — hosted on their servers,
	//     billing via their subscription.
	//   * Local: http://localhost:11434 (or a custom URL), OpenAI-compat, no key.
	//     Models via `ollama pull ...`, dynamic catalog via /api/tags.
	//
	// Forms:
	//   /connect ollama              — description + example
	//   /connect ollama <api-key>    — cloud, default https://ollama.com
	//   /connect ollama <key> <url>  — cloud custom URL
	//   /connect ollama local        — localhost:11434
	//   /connect ollama local <url>  — custom local URL
	if provider == "ollama" {
		if len(parts) < 2 {
			return i18n.T("subs.connect.ollamaHelp")
		}
		// Local mode
		if parts[1] == "local" {
			baseURL := "http://localhost:11434"
			if len(parts) >= 3 {
				baseURL = parts[2]
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			models, err := llm.FetchOllamaModels(ctx, baseURL)
			cancel()
			if err != nil {
				return "✗ " + err.Error()
			}
			if m.subs == nil {
				m.subs, _ = subscriptions.Load()
			}
			m.subs.Add(subscriptions.Subscription{
				Provider: "ollama", APIKey: "", BaseURL: baseURL, Plan: "local",
			})
			if err := m.subs.Save(); err != nil {
				return i18n.Tf("subs.saveError", err)
			}
			m.ollamaModels = models
			names := make([]string, 0, len(models))
			for _, mm := range models {
				names = append(names, mm.ID)
			}
			return i18n.Tf("subs.connect.ollamaLocalOK",
				baseURL, len(models), strings.Join(names, ", "))
		}
		// Cloud mode (default)
		apiKey := parts[1]
		baseURL := "https://ollama.com"
		if len(parts) >= 3 {
			baseURL = parts[2]
		}
		if m.subs == nil {
			m.subs, _ = subscriptions.Load()
		}
		m.subs.Add(subscriptions.Subscription{
			Provider: "ollama", APIKey: apiKey, BaseURL: baseURL, Plan: "cloud",
		})
		if err := m.subs.Save(); err != nil {
			return i18n.Tf("subs.saveError", err)
		}
		return i18n.Tf("subs.connect.ollamaCloudOK", baseURL)
	}
	if provider != "zai" && provider != "anthropic" && provider != "kimi" && provider != "kimi-api" && provider != "openai" {
		return i18n.Tf("subs.connect.unsupported", provider)
	}
	if len(parts) < 2 {
		hint := i18n.T("subs.connect.example.zai")
		switch provider {
		case "anthropic":
			hint = i18n.T("subs.connect.example.anthropic")
		case "kimi":
			hint = i18n.T("subs.connect.example.kimi")
		case "kimi-api":
			hint = i18n.T("subs.connect.example.kimiapi")
		case "openai":
			hint = i18n.T("subs.connect.example.openai")
		}
		return i18n.Tf("subs.connect.keyRequired", hint)
	}
	apiKey := parts[1]
	baseURL := ""
	if len(parts) >= 3 {
		baseURL = parts[2]
	}
	if m.subs == nil {
		m.subs, _ = subscriptions.Load()
	}
	plan := "coding" // default for zai/kimi
	if provider == "anthropic" || provider == "kimi-api" || provider == "openai" {
		plan = "api"
	}
	// Kimi Code (kimi.com/code) — Coding Plan only, endpoint api.kimi.com/coding.
	// Validation: immediately try GET /models — if 200/401, the key reached the right service.
	if provider == "kimi" && baseURL == "" {
		baseURL = "https://api.kimi.com/coding"
	}
	if provider == "kimi-api" && baseURL == "" {
		baseURL = "https://api.moonshot.ai/v1"
	}
	if provider == "openai" && baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	sub := subscriptions.Subscription{
		Provider: provider,
		APIKey:   apiKey,
		BaseURL:  baseURL,
		Plan:     plan,
	}
	// Kimi Code: fetch the available-models list → show the plan in the status bar.
	if provider == "kimi" {
		if avail := fetchKimiAvailableModels(baseURL, apiKey); len(avail) > 0 {
			sub.AvailableModels = avail
		}
	}
	// Moonshot Platform: fetch /v1/models with key validation.
	// If 401 — reject the connection (the key is not for this endpoint,
	// the user probably mixed up a Kimi Code key with Moonshot Platform).
	if provider == "kimi-api" {
		avail, status := fetchKimiAvailableModelsWithStatus(baseURL, apiKey)
		if status == 401 || status == 403 {
			return i18n.Tf("subs.connect.kimiApiRejected", status)
		}
		if len(avail) > 0 {
			sub.AvailableModels = avail
		}
	}
	// OpenAI Platform: the same /v1/models with validation.
	if provider == "openai" {
		avail, status := fetchKimiAvailableModelsWithStatus(baseURL, apiKey)
		if status == 401 || status == 403 {
			return i18n.Tf("subs.connect.openaiRejected", status)
		}
		if len(avail) > 0 {
			sub.AvailableModels = avail
		}
	}
	m.subs.Add(sub)
	if err := m.subs.Save(); err != nil {
		return i18n.Tf("subs.saveError", err)
	}
	if provider == "kimi" || provider == "kimi-api" {
		label := "Kimi Code (kimi.com/code) Coding Plan"
		if provider == "kimi-api" {
			label = "Moonshot Platform (platform.moonshot.ai) pay-per-token"
		}
		extra := ""
		if len(sub.AvailableModels) > 0 {
			extra = i18n.Tf("subs.connect.availableModels", strings.Join(sub.AvailableModels, ", "))
		}
		return i18n.Tf("subs.connected.via", provider, label, extra, provider)
	}
	return i18n.Tf("subs.connected", provider, provider)
}

// detectKimiPlan — probe Moonshot endpoints with the same key. Try both types
// (Anthropic-compat and OpenAI-compat) on both domains (.ai global and .cn
// Chinese). Returns plan+base_url of the first one that passes, or empty+diag.
type probeAttempt struct {
	url    string
	plan   string
	base   string
	authAn bool // true = x-api-key + anthropic-version, false = Bearer
}

func detectKimiPlan(apiKey string) (plan, endpoint, diag string) {
	client := &http.Client{Timeout: 8 * time.Second}
	attempts := []probeAttempt{
		// Kimi Code (kimi.com/code) — priority: their Coding Plan subscription goes through
		// api.kimi.com/coding, documented at www.kimi.com/code/docs/en/.
		{"https://api.kimi.com/coding/v1/messages", "coding", "https://api.kimi.com/coding", true},
		{"https://api.kimi.com/coding/v1/chat/completions", "api", "https://api.kimi.com/coding/v1", false},
		// Moonshot Platform (platform.moonshot.ai) — regular pay-per-token API keys.
		{"https://api.moonshot.ai/anthropic/v1/messages", "coding", "https://api.moonshot.ai/anthropic", true},
		{"https://api.moonshot.cn/anthropic/v1/messages", "coding", "https://api.moonshot.cn/anthropic", true},
		{"https://api.moonshot.ai/v1/chat/completions", "api", "https://api.moonshot.ai/v1", false},
		{"https://api.moonshot.cn/v1/chat/completions", "api", "https://api.moonshot.cn/v1", false},
	}
	var results []string
	for _, a := range attempts {
		code, err := probeKimiOne(client, a, apiKey)
		if err != nil {
			results = append(results, fmt.Sprintf("%s: %v", a.url, err))
			continue
		}
		results = append(results, fmt.Sprintf("%s → HTTP %d", a.url, code))
		// 200 (ok) or 400 (we sent a minimal body — body validation) — key accepted.
		// 404 (wrong endpoint path, but the key might have worked) — treated as not suitable.
		// 401/403 — definitely rejected.
		if code == 200 || code == 400 {
			return a.plan, a.base, ""
		}
	}
	return "", "", strings.Join(results, "\n     ")
}

func probeKimiOne(client *http.Client, a probeAttempt, apiKey string) (int, error) {
	// Model "k3" is a valid ID at kimi.com/coding; moonshot returns 400
	// (unknown model), which also counts as an accepted key.
	body := `{"model":"k3","max_tokens":1,"messages":[{"role":"user","content":"hi"}]}`
	req, _ := http.NewRequest("POST", a.url, strings.NewReader(body))
	req.Header.Set("User-Agent", version.UserAgent())
	req.Header.Set("Content-Type", "application/json")
	if a.authAn {
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

// refreshOpenAIAvailability — like refreshKimiAvailability but for SourceOpenAI.
// Uses the same fetchKimiAvailableModels — it is a generic OpenAI-compat GET /v1/models.
func (m *tuiModel) refreshOpenAIAvailability(active subscriptions.Subscription) {
	avail := fetchKimiAvailableModels(active.BaseURL, active.APIKey)
	if len(avail) == 0 || m.subs == nil {
		return
	}
	sub, ok := m.subs.Subscriptions[subscriptions.SourceOpenAI]
	if !ok {
		return
	}
	sub.AvailableModels = avail
	m.subs.Subscriptions[subscriptions.SourceOpenAI] = sub
	_ = m.subs.Save()
}

// buildOpenAIDynamicCatalog — builds Models[] from a raw list of IDs with human-readable
// names and primary priority. Analogous to buildKimiAPIDynamicCatalog.
func buildOpenAIDynamicCatalog(ids []string) []llm.Model {
	// Flagship on top: gpt-5 > o3 > gpt-4.1 > gpt-4o > o4-mini > gpt-4o-mini.
	primaryOrder := []string{"gpt-5", "gpt-5-mini", "o3", "gpt-4.1", "gpt-4o", "o4-mini", "gpt-4o-mini"}
	// Look for the BEST available one by priority — iterate primaryOrder, not ids.
	// Otherwise if ids=[gpt-4o, gpt-5], gpt-4o would come first and become primary.
	idsSet := map[string]bool{}
	for _, id := range ids {
		idsSet[id] = true
	}
	primaryID := ""
	for _, want := range primaryOrder {
		if idsSet[want] {
			primaryID = want
			break
		}
	}
	models := make([]llm.Model, 0, len(ids))
	for _, id := range ids {
		mod := llm.Model{ID: id, Provider: "openai", HasTools: true, Tier: "standard", Name: openAIDisplayName(id)}
		if strings.HasPrefix(id, "gpt-5") || strings.HasPrefix(id, "o3") || strings.HasPrefix(id, "gpt-4.1") {
			mod.Tier = "flagship"
		}
		if id == primaryID {
			mod.IsPrimary = true
		}
		models = append(models, mod)
	}
	// Fallback: if none of the priority entries were found as primary, use the first one.
	if primaryID == "" && len(models) > 0 {
		models[0].IsPrimary = true
	}
	return models
}

func openAIDisplayName(id string) string {
	switch id {
	case "gpt-5":
		return "GPT-5"
	case "gpt-5-mini":
		return "GPT-5 mini"
	case "gpt-4.1":
		return "GPT-4.1"
	case "gpt-4o":
		return "GPT-4o"
	case "gpt-4o-mini":
		return "GPT-4o mini"
	case "o3":
		return "o3"
	case "o4-mini":
		return "o4-mini"
	}
	return id
}

// refreshKimiAvailability — background fetch of the available-models list and
// saving it into the subscription. Ignores errors (the plan just stays unlabeled).
// Works for both Kimi sources (kimi Coding Plan and kimi-api pay-per-token).
func (m *tuiModel) refreshKimiAvailability(active subscriptions.Subscription) {
	avail := fetchKimiAvailableModels(active.BaseURL, active.APIKey)
	if len(avail) == 0 {
		return
	}
	if m.subs == nil {
		return
	}
	sub, ok := m.subs.Subscriptions[active.Provider]
	if !ok {
		return
	}
	sub.AvailableModels = avail
	m.subs.Subscriptions[active.Provider] = sub
	_ = m.subs.Save()
}

// buildKimiAPIDynamicCatalog — generates Models[] from the list of IDs obtained
// from the Moonshot server's /v1/models. Metadata (Name/Description/Tier) — heuristics
// by ID prefix: kimi-latest → flagship primary; kimi-thinking → thinking;
// moonshot-v1-{ctx}k → by context size.
func buildKimiAPIDynamicCatalog(ids []string) []llm.Model {
	// Primary priority: first any K3 (Moonshot's flagship), then
	// kimi-latest (auto — under the hood usually the same K3), then K2/moonshot.
	// Find the BEST one by priority by iterating primaryOrder (not ids), otherwise
	// something like gpt-4o would come before gpt-5.
	primaryOrder := []string{
		"kimi-k3", "kimi-k3-preview", "kimi-k3-turbo", "k3",
		"kimi-latest",
		"kimi-k2-turbo-preview",
		"moonshot-v1-auto", "moonshot-v1-128k",
	}
	idsSet := map[string]bool{}
	for _, id := range ids {
		idsSet[id] = true
	}
	primaryID := ""
	for _, want := range primaryOrder {
		if idsSet[want] {
			primaryID = want
			break
		}
	}
	models := make([]llm.Model, 0, len(ids))
	for _, id := range ids {
		mod := llm.Model{ID: id, Provider: "kimi-api", HasTools: true, Tier: "standard", Name: kimiAPIDisplayName(id)}
		if strings.HasPrefix(id, "kimi-") || id == "moonshot-v1-auto" {
			mod.Tier = "flagship"
		}
		if id == primaryID {
			mod.IsPrimary = true
		}
		models = append(models, mod)
	}
	// Fallback: none of the priority entries found as primary — first in the list.
	if primaryID == "" && len(models) > 0 {
		models[0].IsPrimary = true
	}
	return models
}

func kimiAPIDisplayName(id string) string {
	// Simple heuristics for a human-readable name.
	switch id {
	case "kimi-latest":
		return "Kimi Latest"
	case "kimi-k2-turbo-preview":
		return "Kimi K2 Turbo"
	case "kimi-thinking-preview":
		return "Kimi Thinking"
	case "moonshot-v1-auto":
		return "Moonshot v1 Auto"
	case "moonshot-v1-8k":
		return "Moonshot v1 8K"
	case "moonshot-v1-32k":
		return "Moonshot v1 32K"
	case "moonshot-v1-128k":
		return "Moonshot v1 128K"
	}
	return id
}

// fetchKimiAvailableModelsWithStatus — like fetchKimiAvailableModels, but
// also returns the HTTP status so the caller can distinguish "endpoint did not respond"
// from "key rejected". Used in /connect kimi-api for validation.
func fetchKimiAvailableModelsWithStatus(baseURL, apiKey string) ([]string, int) {
	if baseURL == "" {
		baseURL = "https://api.kimi.com/coding"
	}
	url := strings.TrimSuffix(baseURL, "/")
	if !strings.HasSuffix(url, "/v1") {
		url += "/v1"
	}
	url += "/models"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, 0
	}
	req.Header.Set("User-Agent", version.UserAgent())
	req.Header.Set("Authorization", "Bearer "+apiKey)
	client := &http.Client{Timeout: 6 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, resp.StatusCode
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, resp.StatusCode
	}
	ids := make([]string, 0, len(out.Data))
	for _, m := range out.Data {
		if m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	return ids, 200
}

// fetchKimiAvailableModels — GET /v1/models on the Coding Plan endpoint.
// Returns the list of model IDs the server reports as available;
// used to automatically show the plan in the status bar.
// Empty list if the endpoint does not respond or the format is unexpected.
func fetchKimiAvailableModels(baseURL, apiKey string) []string {
	if baseURL == "" {
		baseURL = "https://api.kimi.com/coding"
	}
	// The Anthropic-compat base ends with /coding, while OpenAI-compat ends with /coding/v1.
	// Normalize to /coding/v1/models.
	url := strings.TrimSuffix(baseURL, "/")
	if !strings.HasSuffix(url, "/v1") {
		url += "/v1"
	}
	url += "/models"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", version.UserAgent())
	req.Header.Set("Authorization", "Bearer "+apiKey)
	client := &http.Client{Timeout: 6 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil
	}
	ids := make([]string, 0, len(out.Data))
	for _, m := range out.Data {
		if m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	return ids
}

// credsToken returns the ExecAI JWT or "" when not logged in. The ExecAI
// source itself requires login, but external subscriptions (kimi/zai/openai/
// anthropic/ollama/…) work without an ExecAI account — creds may be nil.
func (m *tuiModel) credsToken() string {
	if m.creds == nil {
		return ""
	}
	return m.creds.Token
}

// pickOllamaCloudPrimary reorders primary in the dynamic ollama.com catalog:
// the server list is sorted by date, but we prefer well-known coding flagships.
func pickOllamaCloudPrimary(ms []llm.Model) []llm.Model {
	priority := []string{"glm-5.2", "kimi-k3", "qwen3-coder:480b-cloud", "deepseek-v4-flash", "kimi-k2.5"}
	idx := -1
	for _, want := range priority {
		for i, m := range ms {
			if m.ID == want {
				idx = i
				break
			}
		}
		if idx >= 0 {
			break
		}
	}
	for i := range ms {
		ms[i].IsPrimary = false
	}
	if idx < 0 {
		idx = 0
	}
	ms[idx].IsPrimary = true
	return ms
}
