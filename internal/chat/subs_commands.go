// Команды и хелперы для управления подписками на внешние провайдеры
// (/connect zai, /use, /subscriptions). Локальный кеш моделей под каждый source.
package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

// makeLLMClient собирает LLM-клиент исходя из активной подписки.
// Если active="" или "execai" → AICoreClient (наш gateway).
// Иначе → клиент соответствующего провайдера.
func (m *tuiModel) makeLLMClient() llm.StreamingLLM {
	if m.subs != nil {
		if active := m.subs.ActiveSubscription(); active != nil {
			switch active.Provider {
			case subscriptions.SourceZAI:
				// Z.ai Coding Plan ключ работает ТОЛЬКО через Anthropic-совместимый
				// endpoint (как Claude Code). Дефолтный base = https://api.z.ai/api/anthropic.
				// OpenAI /chat/completions с Coding Plan ключом даёт 429 "Insufficient balance"
				// потому что биллится отдельно от подписки.
				base := active.BaseURL
				if base == "" {
					base = "https://api.z.ai/api/anthropic"
				}
				return llm.NewAnthropicClient(base, active.APIKey, m.current.ID, m.cfg.ThinkingBudget)
			case subscriptions.SourceAnthropic:
				// Настоящий Anthropic API — sk-ant-... key из console.anthropic.com.
				// Pay-per-token. Endpoint api.anthropic.com.
				base := active.BaseURL
				if base == "" {
					base = "https://api.anthropic.com"
				}
				return llm.NewAnthropicClient(base, active.APIKey, m.current.ID, m.cfg.ThinkingBudget)
			case subscriptions.SourceKimi:
				// Kimi Code Coding Plan подписка (kimi.com/code).
				// Endpoint: api.kimi.com/coding — Anthropic-compat + thinking.
				base := active.BaseURL
				if base == "" {
					base = "https://api.kimi.com/coding"
				}
				return llm.NewAnthropicClient(base, active.APIKey, m.current.ID, m.cfg.ThinkingBudget)
			case subscriptions.SourceKimiAPI:
				// Moonshot Platform pay-per-token API-key (platform.moonshot.ai).
				// Endpoint: api.moonshot.ai/v1 — OpenAI-compat через GLMClient.
				base := active.BaseURL
				if base == "" {
					base = "https://api.moonshot.ai/v1"
				}
				return llm.NewGLMClient(base, active.APIKey, m.current.ID)
			case subscriptions.SourceOpenAI:
				// OpenAI Platform pay-per-token API-key.
				// Endpoint: api.openai.com/v1 — OpenAI-compat через GLMClient
				// (ProviderLabel определится как "OpenAI" по BaseURL).
				base := active.BaseURL
				if base == "" {
					base = "https://api.openai.com/v1"
				}
				return llm.NewGLMClient(base, active.APIKey, m.current.ID)
			case subscriptions.SourceCodexCLI:
				// Делегирование в локальный `codex` CLI.
				cli, err := llm.NewCodexCLIClient(m.current.ID)
				if err != nil {
					return llm.NewAICoreClient(m.cfg.APIBase, m.creds.Token, m.current.ID, m.current.Provider)
				}
				return cli
			case subscriptions.SourceClaudeCLI:
				// Делегируем в локальный `claude` CLI. OAuth-сессия Claude Code,
				// квота из Pro/Max-подписки.
				cli, err := llm.NewClaudeCLIClient(m.current.ID)
				if err != nil {
					// Молча fallback — UI должен показать что недоступно.
					return llm.NewAICoreClient(m.cfg.APIBase, m.creds.Token, m.current.ID, m.current.Provider)
				}
				return cli
			case subscriptions.SourceOllama:
				base := active.BaseURL
				// Cloud (ollama.com) — Anthropic-совместимый endpoint, нужен API-key.
				// Local (localhost:11434 или свой) — OpenAI-compat, без ключа.
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
	return llm.NewAICoreClient(m.cfg.APIBase, m.creds.Token, m.current.ID, m.current.Provider)
}

// applySubscriptionSource синхронизирует m.models/m.current с активным источником
// и пересоздаёт m.cli. Вызывается после /use и при boot.
func (m *tuiModel) applySubscriptionSource() {
	if m.subs == nil {
		m.cli = m.makeLLMClient()
		return
	}
	active := m.subs.ActiveSubscription()
	if active == nil {
		// ExecAI — восстанавливаем исходный каталог из снапшота, иначе после
		// /source zai → /source execai в m.models останутся GLMModels и
		// m.current.Provider будет "zai" → запрос пойдёт в /aicore-vbai/agent-stream
		// с чужой моделью и провайдером → 401.
		if len(m.execAIModels) > 0 {
			m.models = m.execAIModels
		}
		m.current = pickForNewCatalog(m.models, m.current)
		m.cfg.SelectedModelID = m.current.ID
		m.cli = m.makeLLMClient()
		return
	}
	// Внешняя подписка — заменяем каталог моделей на её.
	switch active.Provider {
	case subscriptions.SourceZAI:
		m.models = llm.GLMModels()
	case subscriptions.SourceAnthropic:
		m.models = llm.AnthropicModels()
	case subscriptions.SourceKimi:
		m.models = llm.KimiModels()
		// Ленивая подтяжка списка доступных моделей — для тех кто connect'нулся
		// до появления этой фичи (или чтобы обновить после апгрейда плана).
		// Разово на источник, в фоне; в UI прилетит после сохранения.
		if len(active.AvailableModels) == 0 {
			go m.refreshKimiAvailability(*active)
		}
	case subscriptions.SourceKimiAPI:
		// Приоритет: реальный список моделей с сервера (заполняется в /connect).
		// Fallback: hardcoded MoonshotModels() — если сервер не ответил или
		// подключение старое (до появления fetchKimiAvailableModels).
		if len(active.AvailableModels) > 0 {
			m.models = buildKimiAPIDynamicCatalog(active.AvailableModels)
		} else {
			m.models = llm.MoonshotModels()
			// Пробуем подтянуть в фоне — на следующее переключение будет свежее.
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
			// Cloud — хардкод известных моделей ollama.com. Каталог редко
			// меняется, не имеет смысла бить в сеть на каждое переключение.
			m.models = llm.OllamaCloudModels()
		} else {
			// Local — динамика через /api/tags. Кеш держим на сессию.
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
		// Неизвестный provider — fallback на ExecAI каталог, но клиент мог и не отработать.
	}
	m.current = pickForNewCatalog(m.models, m.current)
	m.cfg.SelectedModelID = m.current.ID
	m.cli = m.makeLLMClient()
}

// pickForNewCatalog выбирает Model из нового каталога:
//   - если есть запись с таким же ID — берём ЕЁ (важно: у одинаковых ID
//     разные Provider, напр. glm-5.2 из zai vs ollama-cloud);
//   - иначе — primary.
// Если каталог пуст — оставляем current как есть.
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

// sourceSupportsThinking — можно ли крутить effort/thinking для текущего source.
// Anthropic-compat клиенты (zai, kimi, anthropic, ollama-cloud, claude-cli) — да.
// OpenAI-compat local ollama и наш ExecAI gateway — нет (пока).
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

// isOllamaCloud — эвристика: ollama.com/api.ollama.com или plan="cloud".
// Всё остальное считаем локальным (localhost, LAN IP, свой домен).
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

// handleSubsCommand обрабатывает /connect, /disconnect, /use, /subscriptions.
// Возвращает (handled, statusMessage). Если handled=false — команда не наша.
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
			return true, "подписок нет"
		}
		if _, ok := m.subs.Subscriptions[provider]; !ok {
			return true, fmt.Sprintf("подписка %q не найдена", provider)
		}
		m.subs.Remove(provider)
		if err := m.subs.Save(); err != nil {
			return true, "ошибка сохранения: " + err.Error()
		}
		m.applySubscriptionSource()
		return true, fmt.Sprintf("✓ %s отключён", provider)

	case cmd == "/source" || cmd == "/use":
		// Голый /source — список вариантов. /use оставлен как deprecated alias.
		var b strings.Builder
		if cmd == "/use" {
			b.WriteString("ℹ /use — deprecated, используй /source\n\n")
		}
		b.WriteString("Доступные источники (/source <name>):\n")
		b.WriteString("  • execai  — наш биллинг (дефолт)\n")
		if m.subs != nil {
			for _, sub := range m.subs.List() {
				fmt.Fprintf(&b, "  • %-8s — подписка %s\n", sub.Provider, sub.Plan)
			}
		}
		b.WriteString("\nПодсказка: набери '/source ' и Tab — будет меню.")
		return true, b.String()

	case strings.HasPrefix(cmd, "/source ") || strings.HasPrefix(cmd, "/use "):
		// Поддерживаем оба префикса. /use — deprecated alias.
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
			// Удобный how-to для конкретных провайдеров.
			extra := ""
			switch target {
			case "zai":
				extra = "\n\nКак подключить Z.ai:\n" +
					"  1. Возьми ключ на https://z.ai/manage-apikey/apikey-list\n" +
					"     (раздел Individual Coding Plan → Plan Overview)\n" +
					"  2. /connect zai sk-zai-XXXXXX\n" +
					"  3. /source zai"
			case "anthropic":
				extra = "\n\nКак подключить Anthropic:\n" +
					"  1. Возьми API-key на https://console.anthropic.com/settings/keys\n" +
					"     (формат sk-ant-... pay-per-token биллинг)\n" +
					"  2. /connect anthropic sk-ant-XXXXXX\n" +
					"  3. /source anthropic"
			}
			return true, "✗ " + err.Error() + extra
		}
		if err := m.subs.Save(); err != nil {
			return true, "ошибка сохранения: " + err.Error()
		}
		m.applySubscriptionSource()
		return true, fmt.Sprintf("✓ источник: %s · модель: %s", m.subs.SourceLabel(), m.current.ID)

	case cmd == "/connect":
		return true, "Использование: /connect <provider> <api_key>\n" +
			"Поддерживается: zai (Z.ai Coding Plan)\n" +
			"Подсказка: '/connect ' + Tab — будет меню провайдеров."

	case cmd == "/disconnect":
		if m.subs == nil || len(m.subs.Subscriptions) == 0 {
			return true, "Нет подключенных подписок."
		}
		var b strings.Builder
		b.WriteString("/disconnect <provider>. Подключены:\n")
		for _, sub := range m.subs.List() {
			fmt.Fprintf(&b, "  • %s\n", sub.Provider)
		}
		return true, b.String()
	}
	return false, ""
}

// formatSubsList — компактный вывод для /subscriptions.
func (m *tuiModel) formatSubsList() string {
	if m.subs == nil || len(m.subs.Subscriptions) == 0 {
		return "Подключенных подписок нет.\n" +
			"Подключить: /connect zai (Z.ai Coding Plan)\n" +
			"Текущий источник: ExecAI (наш биллинг)"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Подключенные подписки (активна: %s):\n", m.subs.SourceLabel())
	for _, sub := range m.subs.List() {
		mark := "  "
		if sub.Provider == m.subs.Active {
			mark = "● "
		}
		fmt.Fprintf(&b, "%s%-12s  plan=%s  connected=%s\n",
			mark, sub.Provider, sub.Plan, sub.ConnectedAt.Format("2006-01-02"))
	}
	fmt.Fprintf(&b, "\nПереключить: /source <provider>  (или /source execai → наш биллинг)\n")
	fmt.Fprintf(&b, "Отключить:   /disconnect <provider>")
	return b.String()
}

// filterUseOptions — что предложить для /source <arg> (и /use deprecated).
// Базовый ExecAI всегда есть. Подключенные подписки тоже.
// cmd — конкретный slash-command который ввёл юзер (для сохранения префикса).
func filterUseOptions(s *subscriptions.Store, prefix string) []suggestItem {
	return filterUseOptionsFor(s, prefix, "/source")
}

func filterUseOptionsFor(s *subscriptions.Store, prefix, cmdPrefix string) []suggestItem {
	low := strings.ToLower(prefix)
	// Известные провайдеры (порядок = в каком будут видны в меню).
	knownProviders := []struct {
		name string
		desc string
	}{
		{"execai", "наш биллинг (дефолт)"},
		{"zai", "Z.ai GLM Coding Plan"},
		{"kimi", "Kimi Code Coding Plan (K3/K2.7, подписка kimi.com/code)"},
		{"kimi-api", "Moonshot Platform (pay-per-token, platform.moonshot.ai)"},
		{"anthropic", "Anthropic API (sk-ant-…)"},
		{"openai", "OpenAI Platform (sk-… из platform.openai.com, pay-per-token)"},
		{"codex-cli", "локальный OpenAI Codex CLI (квота ChatGPT Plus/Pro)"},
		{"claude-cli", "локальный Claude Code (квота Pro/Max-подписки)"},
		{"ollama", "локальный Ollama runner (localhost:11434)"},
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
			h := "подключено"
			if sub.Plan != "" {
				h = "подключено · " + sub.Plan
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
			// Не подключено — показываем как подсказку, при выборе предложит /connect.
			all = append(all, suggestItem{
				insert: cmdPrefix + " " + p.name,
				label:  p.name,
				hint:   "не подключено — /connect " + p.name,
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

// filterConnectOptions — что предложить для /connect <arg>.
// Пока только zai. Добавятся anthropic/openai когда сделаем.
func filterConnectOptions(prefix string) []suggestItem {
	low := strings.ToLower(prefix)
	all := []suggestItem{
		{insert: "/connect zai ", label: "zai", hint: "Z.ai GLM Coding Plan (Coding Plan API key)"},
		{insert: "/connect kimi ", label: "kimi", hint: "Kimi Code Coding Plan (K3/K2.7, ключ kimi.com/code/console)"},
		{insert: "/connect kimi-api ", label: "kimi-api", hint: "Moonshot Platform pay-per-token (ключ platform.moonshot.ai)"},
		{insert: "/connect anthropic ", label: "anthropic", hint: "Claude API (sk-ant-... из console.anthropic.com)"},
		{insert: "/connect openai ", label: "openai", hint: "OpenAI Platform pay-per-token (sk-… из platform.openai.com)"},
		{insert: "/connect codex-cli", label: "codex-cli", hint: "локальный OpenAI Codex CLI (без ключа, нужна установленная `codex`)"},
		{insert: "/connect claude-cli", label: "claude-cli", hint: "локальный Claude Code (без ключа, нужна установленная `claude`)"},
		{insert: "/connect ollama", label: "ollama", hint: "локальный Ollama runner (localhost:11434, без ключа)"},
	}
	out := make([]suggestItem, 0, len(all))
	for _, it := range all {
		if low == "" || strings.Contains(strings.ToLower(it.label), low) {
			out = append(out, it)
		}
	}
	return out
}

// filterDisconnectOptions — только подключенные.
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
				hint:   "удалить подписку",
			})
		}
	}
	return out
}

// startConnectFlow — для MVP синхронно через env-переменные или подсказку.
// Z.ai: ожидаем что юзер прислал команду с API-ключом, например /connect zai <key>
// либо просто /connect zai → говорим как сделать.
func (m *tuiModel) startConnectFlow(arg string) string {
	parts := strings.Fields(arg)
	if len(parts) == 0 {
		return "Использование: /connect <provider> <api_key> [base_url]\n" +
			"Поддерживается: zai (Z.ai Coding Plan)\n" +
			"Пример:  /connect zai sk-zai-XXXXX\n" +
			"         /connect zai sk-zai-XXXXX https://open.bigmodel.cn/api/paas/v4  (для CN)"
	}
	provider := parts[0]
	// claude-cli — особый случай: ключ не нужен, только проверка что binary в PATH.
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
			return "ошибка сохранения: " + err.Error()
		}
		return "✓ claude-cli подключен (квота из твоей Pro/Max-подписки через OAuth Claude Code).\n" +
			"Переключись: /source claude-cli\n" +
			"\n" +
			"Ограничения:\n" +
			"  • execai-tools (Bash/Read/Write) НЕ работают — claude CLI запускает СВОИ tools.\n" +
			"  • Управление моделью — через `claude config set defaultModel <id>` снаружи.\n" +
			"  • История передаётся как plain-text промт (без session-id)."
	}
	// codex-cli — то же самое для OpenAI Codex CLI (`codex` binary).
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
			return "ошибка сохранения: " + err.Error()
		}
		return "✓ codex-cli подключен (квота из твоей ChatGPT Plus/Pro-подписки через OAuth OpenAI Codex).\n" +
			"Переключись: /source codex-cli\n" +
			"\n" +
			"Требования:\n" +
			"  • Установлен `codex` в PATH (github.com/openai/codex).\n" +
			"  • Выполнен `codex login` в свой ChatGPT-аккаунт.\n" +
			"\n" +
			"Ограничения:\n" +
			"  • execai-tools НЕ работают — codex запускает СВОИ tools.\n" +
			"  • Стриминга нет — codex exec возвращает финальный текст."
	}
	// ollama — 2 режима:
	//   * Cloud (по умолчанию): https://ollama.com, Anthropic-compat, нужен API-key.
	//     Модели типа glm-5.2, qwen3-coder и т.д. — размещены у них на серверах,
	//     billing через их подписку.
	//   * Local: http://localhost:11434 (или свой URL), OpenAI-compat, без ключа.
	//     Модели через `ollama pull ...`, каталог динамический через /api/tags.
	//
	// Формы:
	//   /connect ollama              — описание + пример
	//   /connect ollama <api-key>    — cloud, дефолт https://ollama.com
	//   /connect ollama <key> <url>  — cloud custom URL
	//   /connect ollama local        — localhost:11434
	//   /connect ollama local <url>  — свой local URL
	if provider == "ollama" {
		if len(parts) < 2 {
			return "Ollama — 2 режима подключения:\n" +
				"\n" +
				"🌩  CLOUD (ollama.com):\n" +
				"   Модели glm-5.2, qwen3-coder-30b и др. крутятся у них на серверах.\n" +
				"   Anthropic-совместимый endpoint. Нужен API-ключ с https://ollama.com/settings/keys\n" +
				"   Использование:  /connect ollama <api-key>\n" +
				"\n" +
				"🏠  LOCAL (свой Ollama):\n" +
				"   Модели через `ollama pull <name>`, крутятся локально, 0 ₽.\n" +
				"   OpenAI-совместимый endpoint. Без ключа.\n" +
				"   Использование:  /connect ollama local\n" +
				"                   /connect ollama local http://192.168.1.10:11434  (свой URL)"
		}
		// Local режим
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
				return "ошибка сохранения: " + err.Error()
			}
			m.ollamaModels = models
			names := make([]string, 0, len(models))
			for _, mm := range models {
				names = append(names, mm.ID)
			}
			return fmt.Sprintf("✓ ollama (local) подключен: %s\nМодели (%d):  %s\n\nПереключись: /source ollama",
				baseURL, len(models), strings.Join(names, ", "))
		}
		// Cloud режим (дефолт)
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
			return "ошибка сохранения: " + err.Error()
		}
		return fmt.Sprintf("✓ ollama (cloud) подключен: %s\n"+
			"Модели: glm-5.2, qwen3-coder-30b, kimi-k2 и др. (см. https://ollama.com/library)\n"+
			"\nПереключись: /source ollama\n"+
			"Смени модель:  /model", baseURL)
	}
	if provider != "zai" && provider != "anthropic" && provider != "kimi" && provider != "kimi-api" && provider != "openai" {
		return fmt.Sprintf("провайдер %q пока не поддерживается. Доступно: zai, anthropic, openai, kimi (Kimi Code Coding Plan), kimi-api (Moonshot Platform pay-per-token), claude-cli, codex-cli, ollama", provider)
	}
	if len(parts) < 2 {
		hint := "Пример: /connect zai sk-zai-XXXXX"
		switch provider {
		case "anthropic":
			hint = "Пример: /connect anthropic sk-ant-XXXXX  (ключ из https://console.anthropic.com/settings/keys)"
		case "kimi":
			hint = "Пример: /connect kimi sk-XXXXX  (Kimi Code Coding Plan из https://www.kimi.com/code/console)"
		case "kimi-api":
			hint = "Пример: /connect kimi-api sk-XXXXX  (Moonshot Platform pay-per-token из https://platform.moonshot.ai/console/api-keys)"
		case "openai":
			hint = "Пример: /connect openai sk-proj-XXXXX  (ключ из https://platform.openai.com/api-keys)"
		}
		return "API-key обязателен. " + hint
	}
	apiKey := parts[1]
	baseURL := ""
	if len(parts) >= 3 {
		baseURL = parts[2]
	}
	if m.subs == nil {
		m.subs, _ = subscriptions.Load()
	}
	plan := "coding" // дефолт для zai/kimi
	if provider == "anthropic" || provider == "kimi-api" || provider == "openai" {
		plan = "api"
	}
	// Kimi Code (kimi.com/code) — только Coding Plan, endpoint api.kimi.com/coding.
	// Валидация: сразу пробуем GET /models — если 200/401, ключ дошёл до правильного сервиса.
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
	// Kimi Code: подтягиваем список доступных моделей → отображаем тариф в статус-баре.
	if provider == "kimi" {
		if avail := fetchKimiAvailableModels(baseURL, apiKey); len(avail) > 0 {
			sub.AvailableModels = avail
		}
	}
	// Moonshot Platform: подтягиваем /v1/models с валидацией ключа.
	// Если 401 — reject подключения (ключ не для этого endpoint'а,
	// вероятно юзер попутал Kimi Code ключ с Moonshot Platform).
	if provider == "kimi-api" {
		avail, status := fetchKimiAvailableModelsWithStatus(baseURL, apiKey)
		if status == 401 || status == 403 {
			return "✗ ключ отвергнут api.moonshot.ai (HTTP " + fmt.Sprintf("%d", status) + ").\n" +
				"Возможно ты дал Kimi Code ключ вместо Moonshot Platform.\n\n" +
				"Ключи разные:\n" +
				"  • Kimi Code (подписка):    /connect kimi <key>  — из https://www.kimi.com/code/console\n" +
				"  • Moonshot pay-per-token:  /connect kimi-api <key> — из https://platform.moonshot.ai/console/api-keys"
		}
		if len(avail) > 0 {
			sub.AvailableModels = avail
		}
	}
	// OpenAI Platform: то же самое /v1/models c валидацией.
	if provider == "openai" {
		avail, status := fetchKimiAvailableModelsWithStatus(baseURL, apiKey)
		if status == 401 || status == 403 {
			return fmt.Sprintf("✗ ключ отвергнут api.openai.com (HTTP %d).\n"+
				"Проверь: скопирован ли ключ целиком, актуален ли (не отозван).\n"+
				"Получить ключ: https://platform.openai.com/api-keys", status)
		}
		if len(avail) > 0 {
			sub.AvailableModels = avail
		}
	}
	m.subs.Add(sub)
	if err := m.subs.Save(); err != nil {
		return "ошибка сохранения: " + err.Error()
	}
	if provider == "kimi" || provider == "kimi-api" {
		label := "Kimi Code (kimi.com/code) Coding Plan"
		if provider == "kimi-api" {
			label = "Moonshot Platform (platform.moonshot.ai) pay-per-token"
		}
		extra := ""
		if len(sub.AvailableModels) > 0 {
			extra = fmt.Sprintf("\n  Доступно моделей: %s", strings.Join(sub.AvailableModels, ", "))
		}
		return fmt.Sprintf("✓ %s подключен через %s.%s\nПереключись:  /source %s",
			provider, label, extra, provider)
	}
	return fmt.Sprintf("✓ %s подключен. Переключись:  /source %s", provider, provider)
}

// detectKimiPlan — probe endpoint'ы Moonshot тем же ключом. Пробуем оба типа
// (Anthropic-compat и OpenAI-compat) на обеих доменах (.ai глобальный и .cn
// китайский). Возвращает plan+base_url первого прошедшего или пусто+diag.
type probeAttempt struct {
	url    string
	plan   string
	base   string
	authAn bool // true = x-api-key + anthropic-version, false = Bearer
}

func detectKimiPlan(apiKey string) (plan, endpoint, diag string) {
	client := &http.Client{Timeout: 8 * time.Second}
	attempts := []probeAttempt{
		// Kimi Code (kimi.com/code) — приоритет: их Coding Plan-подписка идёт через
		// api.kimi.com/coding, задокументировано на www.kimi.com/code/docs/en/.
		{"https://api.kimi.com/coding/v1/messages", "coding", "https://api.kimi.com/coding", true},
		{"https://api.kimi.com/coding/v1/chat/completions", "api", "https://api.kimi.com/coding/v1", false},
		// Moonshot Platform (platform.moonshot.ai) — обычные API-keys pay-per-token.
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
		// 200 (ok) или 400 (мы отправили минимальное тело — валидация тела) — ключ принят.
		// 404 (endpoint не тот путь, но ключ мог бы работать) — считаем не подходит.
		// 401/403 — точно отвергнут.
		if code == 200 || code == 400 {
			return a.plan, a.base, ""
		}
	}
	return "", "", strings.Join(results, "\n     ")
}

func probeKimiOne(client *http.Client, a probeAttempt, apiKey string) (int, error) {
	// Модель "k3" — валидный ID у kimi.com/coding; у moonshot вернёт 400
	// (unknown model), что тоже считается за принятый ключ.
	body := `{"model":"k3","max_tokens":1,"messages":[{"role":"user","content":"hi"}]}`
	req, _ := http.NewRequest("POST", a.url, strings.NewReader(body))
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

// refreshOpenAIAvailability — как refreshKimiAvailability но для SourceOpenAI.
// Использует тот же fetchKimiAvailableModels — это generic OpenAI-compat GET /v1/models.
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

// buildOpenAIDynamicCatalog — из сырого списка ID строит Models[] с человекочитаемыми
// именами и приоритетом primary. Аналог buildKimiAPIDynamicCatalog.
func buildOpenAIDynamicCatalog(ids []string) []llm.Model {
	// Флагман сверху: gpt-5 > o3 > gpt-4.1 > gpt-4o > o4-mini > gpt-4o-mini.
	primaryOrder := []string{"gpt-5", "gpt-5-mini", "o3", "gpt-4.1", "gpt-4o", "o4-mini", "gpt-4o-mini"}
	// Ищем ЛУЧШУЮ доступную по приоритету — итерируем primaryOrder, не ids.
	// Иначе если ids=[gpt-4o, gpt-5], gpt-4o встретится первым и станет primary.
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
	// Fallback: если primary ни из priority ни один не найден, ставим первую.
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

// refreshKimiAvailability — фоновая подтяжка списка доступных моделей и
// сохранение в подписку. Игнорирует ошибки (тариф просто останется без метки).
// Работает для обоих Kimi-source'ов (kimi Coding Plan и kimi-api pay-per-token).
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

// buildKimiAPIDynamicCatalog — генерит Models[] из списка IDs, полученных
// с /v1/models сервера Moonshot. Метаданные (Name/Description/Tier) — эвристикой
// по префиксу ID: kimi-latest → flagship primary; kimi-thinking → thinking;
// moonshot-v1-{ctx}k → по объёму контекста.
func buildKimiAPIDynamicCatalog(ids []string) []llm.Model {
	// Приоритет для primary: сначала любые K3 (флагман Moonshot), потом
	// kimi-latest (auto — под капотом обычно тот же K3), потом K2/moonshot.
	// ЛУЧШУЮ по приоритету находим итерируя primaryOrder (не ids), иначе
	// gpt-4o типа встретился бы раньше gpt-5.
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
	// Fallback: primary ни один из priority не найден — первая по списку.
	if primaryID == "" && len(models) > 0 {
		models[0].IsPrimary = true
	}
	return models
}

func kimiAPIDisplayName(id string) string {
	// Простые эвристики для человеко-читаемого имени.
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

// fetchKimiAvailableModelsWithStatus — как fetchKimiAvailableModels, но
// возвращает ещё HTTP-статус чтобы caller мог различить "endpoint не ответил"
// от "ключ отвергнут". Используется в /connect kimi-api для валидации.
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

// fetchKimiAvailableModels — GET /v1/models на Coding Plan endpoint'е.
// Возвращает список model IDs которые сервер сообщает как доступные;
// используется чтобы автоматически показать тариф в status bar.
// Пустой список если endpoint не отвечает или неожиданный формат.
func fetchKimiAvailableModels(baseURL, apiKey string) []string {
	if baseURL == "" {
		baseURL = "https://api.kimi.com/coding"
	}
	// Anthropic-compat base заканчивается на /coding, а OpenAI-compat — на /coding/v1.
	// Приводим к /coding/v1/models.
	url := strings.TrimSuffix(baseURL, "/")
	if !strings.HasSuffix(url, "/v1") {
		url += "/v1"
	}
	url += "/models"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil
	}
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
