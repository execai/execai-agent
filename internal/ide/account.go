// Учётные команды IDE-протокола: вход в ExecAI, подписки, настройки хода.
//
// Зачем здесь, а не «иди в терминал»: человек, который поставил плагин, не
// обязан знать про TUI. Всё, что нужно для начала работы — логин и ключ
// провайдера, — должно делаться из панели.
//
// Правила те же, что в TUI: конфиг и подписки пишутся в те же файлы
// (~/.config/execai), поэтому вход из редактора виден в терминале и наоборот.
package ide

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/auth"
	"github.com/velesbsdllc/agent-vbai/internal/config"
	subsconnect "github.com/velesbsdllc/agent-vbai/internal/connect"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/security"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
	"github.com/velesbsdllc/agent-vbai/internal/tools"
)

// effortLevels — те же имена и бюджеты, что у /effort в TUI. Разъедутся —
// человек в редакторе и в терминале получит разное «high».
var effortLevels = map[string]int{"off": 0, "low": 1024, "medium": 4096, "high": 8192, "max": 32000}

// effortName возвращает имя уровня по бюджету («7000» → само число).
func effortName(budget int) string {
	for name, n := range effortLevels {
		if n == budget {
			return name
		}
	}
	return strconv.Itoa(budget)
}

// EffortOptions — набор для пикера плагина.
func EffortOptions(cur int) []NamedItem {
	order := []string{"off", "low", "medium", "high", "max"}
	out := make([]NamedItem, 0, len(order))
	for _, name := range order {
		b := effortLevels[name]
		out = append(out, NamedItem{
			ID:     name,
			Label:  fmt.Sprintf("%s (%d)", name, b),
			Active: b == cur,
		})
	}
	return out
}

// setEffort меняет бюджет размышлений и сохраняет конфиг.
func setEffort(cfg *config.Config, value string) (string, error) {
	b, ok := effortLevels[value]
	if !ok {
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 {
			return "", errors.New(i18n.T("ide.err.effort"))
		}
		b = n
	}
	cfg.ThinkingBudget = b
	if err := config.Save(cfg); err != nil {
		return "", err
	}
	return fmt.Sprintf("effort: %s (%d)", effortName(b), b), nil
}

// setMaxIterations меняет предел итераций инструментов на один ход.
func setMaxIterations(cfg *config.Config, value string) (string, error) {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n <= 0 {
		return "", errors.New(i18n.T("ide.err.positive"))
	}
	cfg.MaxIterations = n
	if err := config.Save(cfg); err != nil {
		return "", err
	}
	return i18n.Tf("ide.maxIter.set", n), nil
}

// setSubagentIterations — steps new subagents get by default.
func setSubagentIterations(cfg *config.Config, value string) (string, error) {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n < 1 || n > tools.SubagentMaxIterations {
		return "", errors.New(i18n.Tf("fleet.tui.limitBad", tools.SubagentMaxIterations))
	}
	cfg.SubagentIterations = n
	if err := config.Save(cfg); err != nil {
		return "", err
	}
	tools.SetSubagentDefault(n)
	return i18n.Tf("fleet.tui.limitSet", n), nil
}

// login проводит device-flow: отдаёт ссылку плагину и ждёт подтверждения.
//
// Ждём здесь же, в цикле команд: пока человек не вошёл, работать всё равно
// нечем, а плагин в это время показывает ссылку и код.
func login(ctx context.Context, cfg *config.Config, emit func(Out)) (*config.Credentials, error) {
	start, err := auth.StartAgentLink(ctx, cfg.APIBase)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("ide.login.startFailed"), err)
	}
	emit(Out{Type: "login_start", Text: start.VerifyURI, ID: start.UserCode})

	deadline := time.Now().Add(time.Duration(maxInt(start.ExpiresIn, 300)) * time.Second)
	interval := time.Duration(maxInt(start.PollInterval, 3)) * time.Second
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
		p, err := auth.PollAgentLink(ctx, cfg.APIBase, start.LinkToken)
		if err != nil {
			continue // сеть моргнула — не повод бросать вход
		}
		switch p.Status {
		case "linked":
			cr := &config.Credentials{Token: p.JWT, AgentID: p.AgentID, Email: p.UserEmail, Alias: p.Alias,
				AgentType: "execai-cli", WebBase: auth.WebBaseOf(start.VerifyURI)}
			if err := config.SaveCredentials(cr); err != nil {
				return nil, fmt.Errorf("%s: %w", i18n.T("ide.login.saveFailed"), err)
			}
			return cr, nil
		case "expired":
			return nil, errors.New(i18n.T("ide.login.expired"))
		}
	}
	return nil, errors.New(i18n.T("ide.login.timeout"))
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// connect подключает подписку провайдера.
//
// Ключ приходит от плагина (он спрашивает его своим полем ввода). CLI-провайдеры
// ключа не требуют — там проверяется наличие бинаря, как в TUI.
func connect(subs *subscriptions.Store, in In) (string, error) {
	models := 0
	provider := strings.TrimSpace(in.Value)
	key := strings.TrimSpace(in.Key)
	baseURL := strings.TrimSpace(in.BaseURL)

	switch provider {
	case subscriptions.SourceCustom:
		// Any endpoint speaking the OpenAI or the Anthropic protocol; the
		// panel asks for name, protocol, address, key and (optionally) models.
		name := strings.TrimSpace(in.Label)
		sub, status, err := subsconnect.PrepareCustom(subsconnect.Custom{
			Name: name, API: in.API, BaseURL: baseURL, APIKey: key,
			Headers: in.Headers, Models: in.Models,
		})
		if err != nil {
			return "", err
		}
		if subsconnect.KeyRejected(status) && status != 429 {
			return "", errors.New(i18n.Tf("ide.connect.keyRejectedBy", sub.BaseURL, status))
		}
		subs.Add(sub)
		provider = sub.Provider
		models = len(sub.AvailableModels) + len(sub.ManualModels)
		if models == 0 {
			if err := subs.Save(); err != nil {
				return "", err
			}
			if err := subs.Activate(provider); err != nil {
				return "", err
			}
			if err := subs.Save(); err != nil {
				return "", err
			}
			return i18n.Tf("ide.connect.noModels", subscriptions.DisplayName(sub), provider, status), nil
		}
	case subscriptions.SourceClaudeCLI:
		if _, err := llm.NewClaudeCLIClient(""); err != nil {
			return "", err
		}
		sub := subscriptions.Subscription{Provider: provider, Plan: "Pro/Max (OAuth)"}
		subsconnect.Fill(&sub) // the models, asked from claude itself
		subs.Add(sub)
		models = len(sub.AvailableModels)
	case subscriptions.SourceCodexCLI:
		if _, err := llm.NewCodexCLIClient(""); err != nil {
			return "", err
		}
		sub := subscriptions.Subscription{Provider: provider, Plan: "ChatGPT Plus/Pro (OAuth)"}
		subsconnect.Fill(&sub)
		subs.Add(sub)
		models = len(sub.AvailableModels)
	case subscriptions.SourceOllama:
		// Локальная Ollama ключа не требует; облачная — требует, и там же
		// сразу видно, живой ли он: тянем список моделей.
		base := baseURL
		if base == "" && key != "" {
			base = "https://ollama.com"
		}
		if base == "" {
			base = "http://localhost:11434"
		}
		sub := subscriptions.Subscription{Provider: provider, APIKey: key, BaseURL: base, Plan: "local"}
		if key != "" {
			sub.Plan = "cloud"
		}
		status := subsconnect.Fill(&sub)
		if key != "" && subsconnect.KeyRejected(status) {
			return "", errors.New(i18n.Tf("ide.connect.ollamaKey", status))
		}
		if key == "" && status != 200 {
			return "", errors.New(i18n.Tf("ide.connect.ollamaDown", base, status))
		}
		subs.Add(sub)
		models = len(sub.AvailableModels)
	default:
		if !knownProvider(provider) {
			return "", errors.New(i18n.Tf("ide.connect.unknown", provider))
		}
		if key == "" {
			return "", errors.New(i18n.T("ide.connect.needKey"))
		}
		// То же самое, что делает /connect в терминале: подставить endpoint,
		// проверить ключ и забрать живой список моделей. Раньше здесь
		// сохранялся только ключ — подписка выглядела подключённой, а каталог
		// откатывался на встроенный короткий список (у OpenRouter 8 моделей
		// вместо четырёхсот).
		sub, status := subsconnect.Prepare(provider, key, baseURL)
		if subsconnect.RejectsBadKey(provider) && subsconnect.KeyRejected(status) {
			return "", errors.New(i18n.Tf("ide.connect.keyRejected", status, provider))
		}
		subs.Add(sub)
		if len(sub.AvailableModels) > 0 {
			models = len(sub.AvailableModels)
		}
	}

	if err := subs.Save(); err != nil {
		return "", err
	}
	if err := subs.Activate(provider); err != nil {
		return "", err
	}
	if err := subs.Save(); err != nil {
		return "", err
	}
	// Честно: ключ мы не «проверили» — там, где нет дешёвой проверки, ошибка
	// вылезет на первом запросе, и врать «подключено успешно» нельзя.
	note := i18n.Tf("ide.connect.done", subscriptions.DisplayName(subs.Subscriptions[provider]))
	switch {
	case models > 0:
		note += i18n.Tf("ide.connect.models", models)
	case key != "" && provider != subscriptions.SourceOllama:
		// Честно: где нет дешёвой проверки, ошибка вылезет на первом запросе.
		note += i18n.T("ide.connect.unverified")
	}
	return note, nil
}

func knownProvider(p string) bool {
	switch p {
	case subscriptions.SourceZAI, subscriptions.SourceZAIAPI, subscriptions.SourceKimi,
		subscriptions.SourceKimiAPI, subscriptions.SourceAnthropic, subscriptions.SourceOpenAI,
		subscriptions.SourceOpenRouter:
		return true
	}
	return false
}

// ConnectableOptions — what the "connect a source" picker offers, labelled
// in the panel's language (the process locale). The panel acts on the IDs;
// it asks for a key only where one is needed.
func ConnectableOptions() []NamedItem {
	key := func(name string) string { return i18n.Tf("ide.connect.withKey", name) }
	noKey := func(name string) string { return i18n.Tf("ide.connect.noKey", name) }
	return []NamedItem{
		{ID: subscriptions.SourceKimi, Label: key("Kimi Code")},
		{ID: subscriptions.SourceKimiAPI, Label: key("Moonshot Platform")},
		{ID: subscriptions.SourceZAI, Label: key("Z.ai GLM Coding Plan")},
		{ID: subscriptions.SourceZAIAPI, Label: i18n.T("ide.connect.zaiAPI")},
		{ID: subscriptions.SourceAnthropic, Label: key("Anthropic API")},
		{ID: subscriptions.SourceOpenAI, Label: key("OpenAI API")},
		{ID: subscriptions.SourceOpenRouter, Label: i18n.T("ide.connect.openrouter")},
		{ID: subscriptions.SourceOllama, Label: i18n.T("ide.connect.ollama")},
		{ID: subscriptions.SourceClaudeCLI, Label: noKey("Claude Code CLI")},
		{ID: subscriptions.SourceCodexCLI, Label: noKey("OpenAI Codex CLI")},
		{ID: subscriptions.SourceCustom, Label: i18n.T("ide.connect.custom")},
	}
}

// securityOptions — варианты уровня доверия для пикера плагина.
//
// Подписи здесь человеческие: «paranoid» без пояснения ничего не говорит о
// том, что именно начнёт спрашиваться.
func securityOptions() []NamedItem {
	cur := security.Current()
	out := make([]NamedItem, 0, 3)
	for _, l := range security.Levels() {
		out = append(out, NamedItem{ID: l.String(), Label: l.Title(), Active: l == cur})
	}
	return out
}
