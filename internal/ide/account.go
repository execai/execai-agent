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
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/auth"
	"github.com/velesbsdllc/agent-vbai/internal/config"
	subsconnect "github.com/velesbsdllc/agent-vbai/internal/connect"
	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/security"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
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
			return "", fmt.Errorf("effort: off|low|medium|high|max или число")
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
		return "", fmt.Errorf("нужно положительное число")
	}
	cfg.MaxIterations = n
	if err := config.Save(cfg); err != nil {
		return "", err
	}
	return fmt.Sprintf("предел итераций на ход: %d", n), nil
}

// login проводит device-flow: отдаёт ссылку плагину и ждёт подтверждения.
//
// Ждём здесь же, в цикле команд: пока человек не вошёл, работать всё равно
// нечем, а плагин в это время показывает ссылку и код.
func login(ctx context.Context, cfg *config.Config, emit func(Out)) (*config.Credentials, error) {
	start, err := auth.StartAgentLink(ctx, cfg.APIBase)
	if err != nil {
		return nil, fmt.Errorf("не начать вход: %w", err)
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
			cr := &config.Credentials{Token: p.JWT, AgentID: p.AgentID, Email: p.UserEmail, Alias: p.Alias, AgentType: "execai-cli"}
			if err := config.SaveCredentials(cr); err != nil {
				return nil, fmt.Errorf("не сохранить токен: %w", err)
			}
			return cr, nil
		case "expired":
			return nil, fmt.Errorf("код подтверждения истёк — попробуй ещё раз")
		}
	}
	return nil, fmt.Errorf("вход не подтверждён вовремя")
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
func connect(subs *subscriptions.Store, provider, key, baseURL string) (string, error) {
	models := 0
	provider = strings.TrimSpace(provider)
	key = strings.TrimSpace(key)

	switch provider {
	case subscriptions.SourceClaudeCLI:
		if _, err := llm.NewClaudeCLIClient(""); err != nil {
			return "", err
		}
		subs.Add(subscriptions.Subscription{Provider: provider, Plan: "Pro/Max (OAuth)"})
	case subscriptions.SourceCodexCLI:
		if _, err := llm.NewCodexCLIClient(""); err != nil {
			return "", err
		}
		subs.Add(subscriptions.Subscription{Provider: provider, Plan: "ChatGPT Plus/Pro (OAuth)"})
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
		sub := subscriptions.Subscription{Provider: provider, APIKey: key, BaseURL: base}
		if key != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			ms, err := llm.FetchOllamaModelsAuth(ctx, base, key)
			cancel()
			if err != nil {
				return "", fmt.Errorf("ollama.com не принял ключ: %w", err)
			}
			for _, m := range ms {
				sub.AvailableModels = append(sub.AvailableModels, m.ID)
			}
			sub.Plan = "cloud"
		}
		subs.Add(sub)
	default:
		if !knownProvider(provider) {
			return "", fmt.Errorf("неизвестный источник: %s", provider)
		}
		if key == "" {
			return "", fmt.Errorf("нужен ключ провайдера")
		}
		// То же самое, что делает /connect в терминале: подставить endpoint,
		// проверить ключ и забрать живой список моделей. Раньше здесь
		// сохранялся только ключ — подписка выглядела подключённой, а каталог
		// откатывался на встроенный короткий список (у OpenRouter 8 моделей
		// вместо четырёхсот).
		sub, status := subsconnect.Prepare(provider, key, baseURL)
		if subsconnect.RejectsBadKey(provider) && subsconnect.KeyRejected(status) {
			return "", fmt.Errorf("ключ отвергнут (HTTP %d) — проверь, что он от %s", status, provider)
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
	note := "подключено: " + subscriptions.ProviderName(provider)
	switch {
	case models > 0:
		note += fmt.Sprintf(" — моделей доступно: %d", models)
	case key != "" && provider != subscriptions.SourceOllama:
		// Честно: где нет дешёвой проверки, ошибка вылезет на первом запросе.
		note += " (ключ сохранён; проверится на первом запросе)"
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

// ConnectableOptions — что предложить в пикере «подключить источник».
// Ключ нужен не всем: плагин спрашивает его только там, где NeedsKey.
func ConnectableOptions() []NamedItem {
	return []NamedItem{
		{ID: subscriptions.SourceKimi, Label: "Kimi Code (ключ)"},
		{ID: subscriptions.SourceKimiAPI, Label: "Moonshot Platform (ключ)"},
		{ID: subscriptions.SourceZAI, Label: "Z.ai GLM Coding Plan (ключ)"},
		{ID: subscriptions.SourceZAIAPI, Label: "Z.ai платформа, pay-per-token (ключ)"},
		{ID: subscriptions.SourceAnthropic, Label: "Anthropic API (ключ)"},
		{ID: subscriptions.SourceOpenAI, Label: "OpenAI API (ключ)"},
		{ID: subscriptions.SourceOpenRouter, Label: "OpenRouter — все вендоры одним ключом (ключ)"},
		{ID: subscriptions.SourceOllama, Label: "Ollama — локальная или облако (ключ для облака)"},
		{ID: subscriptions.SourceClaudeCLI, Label: "Claude Code CLI (без ключа)"},
		{ID: subscriptions.SourceCodexCLI, Label: "OpenAI Codex CLI (без ключа)"},
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
