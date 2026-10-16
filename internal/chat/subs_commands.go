// Commands and helpers for managing subscriptions to external providers
// (/connect zai, /use, /subscriptions). Local model cache per source.
package chat

import (
	"fmt"
	"github.com/velesbsdllc/agent-vbai/internal/version"
	"net/http"
	"strings"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/catalog"
	"github.com/velesbsdllc/agent-vbai/internal/connect"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/llmpick"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

// makeLLMClient builds an LLM client based on the active subscription.
// If active="" or "execai" → AICoreClient (our gateway).
// Otherwise → the client of the corresponding provider.
func (m *tuiModel) makeLLMClient() llm.StreamingLLM {
	// Логика выбора живёт в internal/llmpick — тем же кодом пользуется
	// `execai serve`, иначе фон ходил бы в ExecAI мимо активной подписки.
	return llmpick.Client(m.cfg, m.subs, m.current.ID, m.current.Provider, m.credsToken())
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
		// /source zai → /source execai m.models would keep the Z.ai list and
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
	// The model list is what the provider itself reports (internal/catalog,
	// shared with the editor panel and serve). Never fetched — a connection
	// that could not reach the provider, or a store from an older build — ask
	// now, briefly; merely stale — refresh in the background, the stored list
	// serves meanwhile.
	sub := *active
	if len(sub.AvailableModels) == 0 && (sub.APIKey != "" || !connect.NeedsKey(sub)) {
		if connect.Fill(&sub) == 200 {
			m.subs.Subscriptions[sub.Provider] = sub
			_ = m.subs.Save()
		}
	} else if connect.Stale(sub) {
		m.refreshModelsInBackground(sub.Provider)
	}
	m.models = catalog.For(sub)
	m.current = pickForSource(m.models, m.current, sub.Provider)
	m.cfg.SelectedModelID = m.current.ID
	m.cli = m.makeLLMClient()
}

// pickForSource is pickForNewCatalog that also survives an empty catalog (the
// provider never answered). The current model then becomes a placeholder of
// THIS source: a model of the previous source would send the request to the
// wrong provider (source switch invariant 2). A model typed with /model for
// such a source is stored in the subscription, so the catalog is not empty
// the next time.
func pickForSource(models []llm.Model, current llm.Model, provider string) llm.Model {
	if len(models) > 0 {
		return pickForNewCatalog(models, current)
	}
	if current.Provider == provider {
		return current
	}
	return llm.Model{Provider: provider}
}

// modelsRefreshedMsg — a background fetch stored a fresh model list.
type modelsRefreshedMsg struct{ provider string }

// refreshModelsInBackground re-fetches a source's model list without blocking
// the UI. The fetch works on its own copy of the store (connect.RefreshModels)
// and the result is applied in Update — the goroutine never touches m.
func (m *tuiModel) refreshModelsInBackground(provider string) {
	if m.refreshing == nil {
		m.refreshing = map[string]bool{}
	}
	if m.refreshing[provider] {
		return
	}
	m.refreshing[provider] = true
	go func() {
		connect.RefreshModels(provider)
		// Read the program only now: at boot the fetch starts before the TUI
		// program exists. The message goes even when nothing was stored —
		// it also clears the in-flight mark.
		if prog := teaProgramRef; prog != nil {
			prog.Send(modelsRefreshedMsg{provider: provider})
		}
	}()
}

// applyRefreshedModels picks up a list stored by refreshModelsInBackground.
func (m *tuiModel) applyRefreshedModels(provider string) {
	delete(m.refreshing, provider)
	st, err := subscriptions.Load()
	if err != nil || st == nil || m.subs == nil {
		return
	}
	fresh, ok := st.Subscriptions[provider]
	if !ok {
		return
	}
	if cur, ok := m.subs.Subscriptions[provider]; ok {
		cur.SetModels(fresh.Models, fresh.ModelsFetchedAt)
		m.subs.Subscriptions[provider] = cur
	}
	if a := m.subs.ActiveSubscription(); a != nil && a.Provider == provider {
		m.models = catalog.For(*a)
		m.current = pickForSource(m.models, m.current, provider)
		m.cfg.SelectedModelID = m.current.ID
		m.cli = m.makeLLMClient()
	}
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
	if a := s.ActiveSubscription(); a != nil && a.IsCustom() {
		return a.API == subscriptions.APIAnthropic
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
	return llmpick.IsOllamaCloud(baseURL, plan)
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
	// /provider is a full synonym of /source: "провайдер" is the word users
	// actually know, while "source" stays the internal id. One entity — the
	// alias must never diverge in behavior, hence rewrite instead of cases.
	if cmd == "/provider" {
		cmd = "/source"
	} else if strings.HasPrefix(cmd, "/provider ") {
		cmd = "/source " + strings.TrimSpace(strings.TrimPrefix(cmd, "/provider "))
	}
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
		// The id is what you type in /source, the name is what it means —
		// "kimi" alone never said whether it is the Coding Plan or the platform.
		fmt.Fprintf(&b, "%s%-12s %-16s plan=%s  connected=%s\n",
			mark, sub.Provider, subscriptions.DisplayName(sub), sub.Plan,
			sub.ConnectedAt.Format("2006-01-02"))
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
		{"zai-api", "subs.provider.zaiapi"},
		{"kimi", "subs.provider.kimi"},
		{"kimi-api", "subs.provider.kimiapi"},
		{"anthropic", "subs.provider.anthropic"},
		{"openai", "subs.provider.openai"},
		{"openrouter", "subs.provider.openrouter"},
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
			// The hint carries the readable name: the label has to stay the id
			// (that is what gets typed and filtered), so this is where "kimi"
			// gets to say it means Kimi Code.
			h := subscriptions.ProviderName(p.name) + " · " + i18n.T("subs.hint.connected")
			if sub.Plan != "" {
				h = subscriptions.ProviderName(p.name) + " · " + i18n.Tf("subs.hint.connectedPlan", sub.Plan)
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
	// Custom endpoints have ids nobody can know in advance — list the
	// connected ones after the built-in providers.
	if s != nil {
		for _, sub := range s.List() {
			if !sub.IsCustom() {
				continue
			}
			mark := ""
			if s.Active == sub.Provider {
				mark = "● "
			}
			all = append(all, suggestItem{
				insert: cmdPrefix + " " + sub.Provider,
				label:  mark + sub.Provider,
				hint:   subscriptions.DisplayName(sub) + " · " + i18n.Tf("subs.hint.custom", sub.API, sub.BaseURL),
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
		{insert: "/connect zai-api ", label: "zai-api", hint: "subs.connectHint.zaiapi"},
		{insert: "/connect kimi ", label: "kimi", hint: "subs.connectHint.kimi"},
		{insert: "/connect kimi-api ", label: "kimi-api", hint: "subs.connectHint.kimiapi"},
		{insert: "/connect anthropic ", label: "anthropic", hint: "subs.connectHint.anthropic"},
		{insert: "/connect openai ", label: "openai", hint: "subs.connectHint.openai"},
		{insert: "/connect openrouter ", label: "openrouter", hint: "subs.connectHint.openrouter"},
		{insert: "/connect codex-cli", label: "codex-cli", hint: "subs.connectHint.codexcli"},
		{insert: "/connect claude-cli", label: "claude-cli", hint: "subs.connectHint.claudecli"},
		{insert: "/connect ollama", label: "ollama", hint: "subs.connectHint.ollama"},
		{insert: "/connect custom", label: "custom", hint: "subs.connectHint.custom"},
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
		cli, err := llm.NewClaudeCLIClient("")
		if err != nil {
			return "✗ " + err.Error()
		}
		if m.subs == nil {
			m.subs, _ = subscriptions.Load()
		}
		sub := subscriptions.Subscription{Provider: "claude-cli", Plan: "Pro/Max (OAuth)"}
		// The models of the account and of this CLI version, asked from
		// claude itself (no tokens spent).
		connect.Fill(&sub)
		m.subs.Add(sub)
		if err := m.subs.Save(); err != nil {
			return i18n.Tf("subs.saveError", err)
		}
		return i18n.Tf("subs.connect.claudecliOK", cli.Describe()) + modelsNote(sub)
	}
	// codex-cli — same thing for the OpenAI Codex CLI (`codex` binary).
	if provider == "codex-cli" {
		if _, err := llm.NewCodexCLIClient(""); err != nil {
			return "✗ " + err.Error()
		}
		if m.subs == nil {
			m.subs, _ = subscriptions.Load()
		}
		sub := subscriptions.Subscription{Provider: "codex-cli", Plan: "ChatGPT Plus/Pro (OAuth)"}
		connect.Fill(&sub)
		m.subs.Add(sub)
		if err := m.subs.Save(); err != nil {
			return i18n.Tf("subs.saveError", err)
		}
		return i18n.T("subs.connect.codexcliOK") + modelsNote(sub)
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
			sub := subscriptions.Subscription{Provider: "ollama", BaseURL: baseURL, Plan: "local"}
			if status := connect.Fill(&sub); status != 200 {
				return i18n.Tf("subs.connect.ollamaLocalFailed", baseURL, status)
			}
			if m.subs == nil {
				m.subs, _ = subscriptions.Load()
			}
			m.subs.Add(sub)
			if err := m.subs.Save(); err != nil {
				return i18n.Tf("subs.saveError", err)
			}
			return i18n.Tf("subs.connect.ollamaLocalOK",
				baseURL, len(sub.AvailableModels), strings.Join(sub.AvailableModels, ", "))
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
		sub := subscriptions.Subscription{Provider: "ollama", APIKey: apiKey, BaseURL: baseURL, Plan: "cloud"}
		if status := connect.Fill(&sub); connect.KeyRejected(status) {
			return i18n.Tf("subs.connect.ollamaCloudRejected", status)
		}
		m.subs.Add(sub)
		if err := m.subs.Save(); err != nil {
			return i18n.Tf("subs.saveError", err)
		}
		return i18n.Tf("subs.connect.ollamaCloudOK", baseURL) + modelsNote(sub)
	}
	// custom — any endpoint speaking the OpenAI or the Anthropic protocol.
	// Bare `/connect custom` walks through the fields one by one; the full
	// form is for scripts and for people who already know the answers.
	if provider == subscriptions.SourceCustom {
		rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(arg), provider))
		if rest == "" {
			return m.startCustomWizard()
		}
		c, err := parseCustomArgs(rest)
		if err != nil {
			return "✗ " + err.Error() + "\n\n" + i18n.T("subs.custom.usage")
		}
		return m.connectCustom(c)
	}
	if provider != "zai" && provider != "zai-api" && provider != "anthropic" && provider != "kimi" && provider != "kimi-api" && provider != "openai" && provider != "openrouter" {
		return i18n.Tf("subs.connect.unsupported", provider)
	}
	// Z.ai issues ONE key that works both for the Coding Plan (Anthropic-compat)
	// and for the open platform (pay-per-token) — only the billing differs. So
	// if the subscription is already connected, /connect zai-api needs no
	// argument: reuse the key instead of making the user go find it again. This
	// matters because zai-api is the path we steer people to for ToS reasons.
	reusedZAIKey := false
	if provider == "zai-api" && len(parts) < 2 {
		if m.subs == nil {
			m.subs, _ = subscriptions.Load()
		}
		if m.subs != nil {
			if z, ok := m.subs.Subscriptions[subscriptions.SourceZAI]; ok && z.APIKey != "" {
				parts = append(parts, z.APIKey)
				reusedZAIKey = true
			}
		}
	}
	if len(parts) < 2 {
		hint := i18n.T("subs.connect.example.zai")
		switch provider {
		case "zai-api":
			hint = i18n.T("subs.connect.example.zaiapi")
		case "anthropic":
			hint = i18n.T("subs.connect.example.anthropic")
		case "kimi":
			hint = i18n.T("subs.connect.example.kimi")
		case "kimi-api":
			hint = i18n.T("subs.connect.example.kimiapi")
		case "openai":
			hint = i18n.T("subs.connect.example.openai")
		case "openrouter":
			hint = i18n.T("subs.connect.example.openrouter")
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
	// Endpoint, plan and the live model list come from one place, shared with
	// the editor panel: connecting from either surface must produce the same
	// subscription (the panel used to store just the key).
	sub, status := connect.Prepare(provider, apiKey, baseURL)
	if connect.RejectsBadKey(provider) && connect.KeyRejected(status) {
		switch provider {
		case "kimi-api":
			return i18n.Tf("subs.connect.kimiApiRejected", status)
		case "zai-api":
			return i18n.Tf("subs.connect.zaiApiRejected", status)
		case "openai":
			return i18n.Tf("subs.connect.openaiRejected", status)
		case "openrouter":
			return i18n.Tf("subs.connect.openrouterRejected", status)
		}
	}
	m.subs.Add(sub)
	if err := m.subs.Save(); err != nil {
		return i18n.Tf("subs.saveError", err)
	}
	if provider == "kimi" || provider == "kimi-api" || provider == "zai-api" || provider == "openrouter" {
		label := "Kimi Code (kimi.com/code) Coding Plan"
		switch provider {
		case "openrouter":
			label = "OpenRouter (openrouter.ai) pay-per-token"
		case "kimi-api":
			label = "Moonshot Platform (platform.moonshot.ai) pay-per-token"
		case "zai-api":
			label = "Z.ai open platform (api.z.ai) pay-per-token"
			if reusedZAIKey {
				label += " — " + i18n.T("subs.connect.zaiKeyReused")
			}
		}
		extra := ""
		if len(sub.AvailableModels) > 0 {
			extra = i18n.Tf("subs.connect.availableModels", strings.Join(sub.AvailableModels, ", "))
		}
		return i18n.Tf("subs.connected.via", provider, label, extra, provider)
	}
	// Z.ai Coding Plan: their usage policy limits the subscription to a closed
	// list of tools that does not include us. Warn at connect time — afterwards
	// it is the user's own account that gets throttled or frozen, not ours.
	if provider == "zai" {
		return i18n.Tf("subs.connected", provider, provider) + modelsNote(sub) + "\n\n" + i18n.T("subs.connect.zaiToSWarning")
	}
	return i18n.Tf("subs.connected", provider, provider) + modelsNote(sub)
}

// modelsNote — one line about the live list after a connect: how many models
// the provider reported, or that it reported none (and what to do then).
func modelsNote(sub subscriptions.Subscription) string {
	if n := len(sub.AvailableModels); n > 0 {
		return "\n" + i18n.Tf("subs.connect.modelsCount", n)
	}
	return "\n" + i18n.T("subs.connect.noModels")
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

// fetchKimiAvailableModels — GET /v1/models on the Kimi Code endpoint: the
// IDs the plan has (used to show the plan in the status bar). Empty when the
// endpoint is silent. The request itself lives in internal/connect, so the
// terminal and the panel ask the provider the same way.
func fetchKimiAvailableModels(baseURL, apiKey string) []string {
	if baseURL == "" {
		baseURL = connect.DefaultBaseURL(subscriptions.SourceKimi)
	}
	ids, _ := connect.FetchModels(baseURL, apiKey)
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
