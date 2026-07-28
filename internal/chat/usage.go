package chat

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

// myBalance — ответ /billing-vbai/my_balance.
type myBalance struct {
	BalanceRubles string `json:"balance_rubles"`
	TariffCode    string `json:"tariff_code"`
	TariffTitle   string `json:"tariff_title"`
	Active        bool   `json:"active"`
	ValidUntil    string `json:"valid_until"`
}

// limitWindow — окно rate-limit из /my_limits.
type limitWindow struct {
	Type             string `json:"type"` // 5h | day | week | month
	UsedCopecks      int64  `json:"used_copecks"`
	LimitCopecks     int64  `json:"limit_copecks"`
	RemainingCopecks int64  `json:"remaining_copecks"`
	PercentUsed      int    `json:"percent_used"`
	ResetInSeconds   int64  `json:"reset_in_seconds"`
	ResetAt          string `json:"reset_at"`
}

type myLimits struct {
	TariffCode string        `json:"tariff_code"`
	Windows    []limitWindow `json:"windows"`
}

// usageEvent — одна строка /billing-vbai/my_usage_events.
type usageEvent struct {
	ID            int64  `json:"id"`
	InteractionID string `json:"interaction_id"`
	Model         string `json:"model"`
	Provider      string `json:"provider"`
	EventType     string `json:"event_type"` // input_tokens | output_tokens | cache_creation_charge | cached_tokens_adjustment | ...
	Tokens        int64  `json:"tokens"`
	CostRub       string `json:"cost_rub"`
	Timestamp     string `json:"timestamp"`
}

// fetchUsageForSource — диспетчер. Берёт active source из subs и зовёт
// соответствующий fetchUsage* (ExecAI / Z.ai / Anthropic / Kimi / ...).
func fetchUsageForSource(apiBase, token string, subs *subscriptions.Store) (string, error) {
	if subs != nil {
		switch subs.Active {
		case subscriptions.SourceZAI:
			return fetchUsageAnthropicCompat("zai", "Z.ai Coding Plan", "https://z.ai/manage-apikey/subscription")
		case subscriptions.SourceAnthropic:
			return fetchUsageAnthropicCompat("anthropic", "Anthropic API", "https://console.anthropic.com/settings/usage")
		case subscriptions.SourceKimi:
			// Kimi Code Coding Plan — есть API квоты.
			if sub, ok := subs.Subscriptions[subscriptions.SourceKimi]; ok {
				return fetchKimiUsage(sub)
			}
		case subscriptions.SourceKimiAPI:
			// Moonshot Platform pay-per-token — квоты нет, только локальный счётчик +
			// ссылка на дашборд с реальным биллингом.
			return fetchUsageAnthropicCompat("kimi-api", "Moonshot Platform (pay-per-token)", "https://platform.moonshot.ai/console")
		}
	}
	return fetchUsage(apiBase, token)
}

// fetchKimiUsage — GET api.kimi.com/coding/v1/usages для Kimi Code Coding Plan.
// Возвращает недельную квоту + rolling-окна (напр. 5h).
// Формат ответа задокументирован в MoonshotAI/kimi-code/packages/oauth/src/managed-usage.ts:
//
//	{ "usage":  {"name":..., "used":n, "limit":n, "resetAt":"ISO"},
//	  "limits": [{"detail":{"used":n,"limit":n,"name":"5h limit"}, "window":{...}}, ...] }
func fetchKimiUsage(sub subscriptions.Subscription) (string, error) {
	baseURL := sub.BaseURL
	if baseURL == "" {
		baseURL = "https://api.kimi.com/coding"
	}
	url := strings.TrimSuffix(baseURL, "/")
	if !strings.HasSuffix(url, "/v1") {
		url += "/v1"
	}
	url += "/usages"

	cli := &http.Client{Timeout: 8 * time.Second}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+sub.APIKey)
	resp, err := cli.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncStr(string(body), 200))
	}

	// used/limit сервер отдаёт то как число, то как строку (см. MoonshotAI/kimi-code
	// managed-usage.ts: toInt(v) принимает и number и string). Ловим оба формата
	// через json.RawMessage → parseFlexInt.
	var raw struct {
		Usage struct {
			Name    string          `json:"name"`
			Used    json.RawMessage `json:"used"`
			Limit   json.RawMessage `json:"limit"`
			ResetAt string          `json:"resetAt"`
		} `json:"usage"`
		Limits []struct {
			Detail struct {
				Name    string          `json:"name"`
				Used    json.RawMessage `json:"used"`
				Limit   json.RawMessage `json:"limit"`
				ResetAt string          `json:"resetAt"`
			} `json:"detail"`
			Window struct {
				Duration json.RawMessage `json:"duration"`
				TimeUnit string          `json:"timeUnit"`
			} `json:"window"`
		} `json:"limits"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return "", fmt.Errorf("json: %w body=%s", err, truncStr(string(body), 200))
	}
	usageUsed := parseFlexInt(raw.Usage.Used)
	usageLimit := parseFlexInt(raw.Usage.Limit)

	var b strings.Builder
	b.WriteString("\n🔌 Kimi Code (kimi.com/code)\n\n")

	// Тариф через набор доступных моделей.
	if tier := kimiTierFromModels(sub.AvailableModels); tier != "" {
		fmt.Fprintf(&b, "  Тариф: %s (по доступным моделям)\n", tier)
	}
	if len(sub.AvailableModels) > 0 {
		fmt.Fprintf(&b, "  Модели: %s\n", strings.Join(sub.AvailableModels, ", "))
	}

	// Недельная квота.
	if usageLimit > 0 || usageUsed > 0 {
		percent := 0
		if usageLimit > 0 {
			percent = int(usageUsed * 100 / usageLimit)
		}
		name := raw.Usage.Name
		if name == "" {
			name = "Weekly limit"
		}
		remaining := usageLimit - usageUsed
		fmt.Fprintf(&b, "\n  %s:\n    %s  %d%%  %s / %s  (осталось %s)",
			name,
			progressBar(percent, 24),
			percent,
			humanInt(usageUsed),
			humanInt(usageLimit),
			humanInt(remaining),
		)
		if hint := kimiResetHint(raw.Usage.ResetAt); hint != "" {
			b.WriteString("  ·  " + hint)
		}
		b.WriteString("\n")
	}

	// Rolling-окна (5h и т.п.).
	if len(raw.Limits) > 0 {
		b.WriteString("\n  Rolling-окна:\n")
		for i, lm := range raw.Limits {
			used := parseFlexInt(lm.Detail.Used)
			limit := parseFlexInt(lm.Detail.Limit)
			duration := parseFlexInt(lm.Window.Duration)
			name := lm.Detail.Name
			if name == "" {
				name = kimiWindowLabel(duration, lm.Window.TimeUnit, i)
			}
			percent := 0
			if limit > 0 {
				percent = int(used * 100 / limit)
			}
			remaining := limit - used
			fmt.Fprintf(&b, "    %-12s %s  %3d%%  %s / %s  (осталось %s)",
				name,
				progressBar(percent, 20),
				percent,
				humanInt(used),
				humanInt(limit),
				humanInt(remaining),
			)
			if hint := kimiResetHint(lm.Detail.ResetAt); hint != "" {
				b.WriteString("  ·  " + hint)
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("\n  ⓘ Управление подпиской: https://www.kimi.com/code/console\n")
	return b.String(), nil
}

// parseFlexInt — парсит json.RawMessage как int64, принимая и число (42) и строку ("42").
// Kimi Code /usages endpoint шлёт цифровые поля в обоих форматах в зависимости от версии
// (см. MoonshotAI/kimi-code/packages/oauth/src/managed-usage.ts::toInt).
func parseFlexInt(raw json.RawMessage) int64 {
	if len(raw) == 0 {
		return 0
	}
	// Число?
	var n int64
	if err := json.Unmarshal(raw, &n); err == nil {
		return n
	}
	// Float?
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return int64(f)
	}
	// Строка "42"?
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		var v int64
		if _, err := fmt.Sscanf(s, "%d", &v); err == nil {
			return v
		}
		var vf float64
		if _, err := fmt.Sscanf(s, "%f", &vf); err == nil {
			return int64(vf)
		}
	}
	return 0
}

// kimiTierFromModels — читаемое имя тарифа по набору моделей на подписке.
// Дублирует subscriptions.deriveKimiTier, но зовётся из внутреннего пакета.
func kimiTierFromModels(models []string) string {
	has := map[string]bool{}
	for _, m := range models {
		has[m] = true
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

// kimiWindowLabel — человеческое имя rolling-окна из duration+timeUnit.
func kimiWindowLabel(duration int64, timeUnit string, idx int) string {
	tu := strings.ToUpper(timeUnit)
	if duration > 0 {
		switch {
		case strings.Contains(tu, "MINUTE"):
			if duration >= 60 && duration%60 == 0 {
				return fmt.Sprintf("%dч", duration/60)
			}
			return fmt.Sprintf("%dмин", duration)
		case strings.Contains(tu, "HOUR"):
			return fmt.Sprintf("%dч", duration)
		case strings.Contains(tu, "DAY"):
			return fmt.Sprintf("%dд", duration)
		}
	}
	return fmt.Sprintf("окно #%d", idx+1)
}

// kimiResetHint — форматирует resetAt (ISO) как "обновится через X" на русском.
func kimiResetHint(iso string) string {
	if iso == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		// Пробуем nano-precision RFC3339 с усечением.
		if len(iso) > 30 {
			t, err = time.Parse(time.RFC3339Nano, iso)
		}
		if err != nil {
			return "обновится " + iso
		}
	}
	diff := int64(time.Until(t).Seconds())
	if diff <= 0 {
		return "обновление"
	}
	return "обновится " + humanDuration(diff)
}

// fetchUsageAnthropicCompat — общий агрегат для Anthropic-совместимых source
// (zai-anthropic, anthropic). Парсит requests.log по source-префиксу,
// агрегирует токены, ссылается на authoritative-источник.
func fetchUsageAnthropicCompat(sourcePrefix, displayName, authoritativeURL string) (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "requests.log")
	f, err := os.Open(path)
	if err != nil {
		return fmt.Sprintf("🔌 %s source\n\nЛокальный лог запросов пуст. Реальная квота: %s", displayName, authoritativeURL), nil
	}
	defer f.Close()
	type stat struct {
		count      int
		inputTok   int64
		outputTok  int64
		cacheTok   int64
		errs       int
		firstTs    string
		lastTs     string
	}
	byModel := map[string]*stat{}
	var total stat
	last24 := time.Now().Add(-24 * time.Hour)
	var last24Cnt int
	var last24In, last24Out int64

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var e struct {
			Ts             string `json:"ts"`
			Source         string `json:"source"`
			ModelReturned  string `json:"model_returned"`
			ModelRequested string `json:"model_requested"`
			Status         string `json:"status"`
			InputTokens    int64  `json:"input_tokens"`
			OutputTokens   int64  `json:"output_tokens"`
			CacheRead      int64  `json:"cache_read_tokens"`
			CacheCreate    int64  `json:"cache_create_tokens"`
		}
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			continue
		}
		if !strings.HasPrefix(e.Source, sourcePrefix) {
			continue
		}
		key := e.ModelReturned
		if key == "" {
			key = e.ModelRequested
		}
		if key == "" {
			key = "(unknown)"
		}
		st, ok := byModel[key]
		if !ok {
			st = &stat{firstTs: e.Ts}
			byModel[key] = st
		}
		st.count++
		st.inputTok += e.InputTokens
		st.outputTok += e.OutputTokens
		st.cacheTok += e.CacheRead + e.CacheCreate
		st.lastTs = e.Ts
		if e.Status != "ok" {
			st.errs++
		}
		total.count++
		total.inputTok += e.InputTokens
		total.outputTok += e.OutputTokens
		total.cacheTok += e.CacheRead + e.CacheCreate
		// Last 24h aggregate
		if ts, err := time.Parse(time.RFC3339, e.Ts); err == nil && ts.After(last24) {
			last24Cnt++
			last24In += e.InputTokens
			last24Out += e.OutputTokens
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "\n🔌 %s source\n\n", displayName)
	if total.count == 0 {
		b.WriteString("  (нет запросов в локальном логе)\n")
		fmt.Fprintf(&b, "\nРеальную квоту смотри на %s", authoritativeURL)
		return b.String(), nil
	}
	fmt.Fprintf(&b, "  Всего запросов в логе:  %d\n", total.count)
	fmt.Fprintf(&b, "  Суммарно input tokens:  %s\n", humanInt(total.inputTok))
	fmt.Fprintf(&b, "  Суммарно output tokens: %s\n", humanInt(total.outputTok))
	if total.cacheTok > 0 {
		fmt.Fprintf(&b, "  Cache tokens:           %s\n", humanInt(total.cacheTok))
	}
	fmt.Fprintf(&b, "\n  За последние 24ч:       %d запросов · %s input · %s output\n",
		last24Cnt, humanInt(last24In), humanInt(last24Out))

	fmt.Fprintf(&b, "\n  По моделям:\n")
	for k, st := range byModel {
		fmt.Fprintf(&b, "    %-22s  %4d запр · ↓%s ↑%s",
			k, st.count, humanInt(st.inputTok), humanInt(st.outputTok))
		if st.errs > 0 {
			fmt.Fprintf(&b, "  ⚠ %d ошибок", st.errs)
		}
		b.WriteString("\n")
	}

	b.WriteString("\n  ⓘ Это локальный счётчик. Реальный биллинг и квоты:\n")
	fmt.Fprintf(&b, "     %s", authoritativeURL)
	return b.String(), nil
}

func humanInt(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1_000_000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%.2fM", float64(n)/1_000_000)
}

// fetchUsage возвращает text-блок для /usage — формат как у web-странцы.
func fetchUsage(apiBase, token string) (string, error) {
	cli := &http.Client{Timeout: 8 * time.Second}

	bal, err := fetchJSON[myBalance](cli, apiBase+"/billing-vbai/my_balance", token)
	if err != nil {
		return "", fmt.Errorf("/my_balance: %w", err)
	}
	limits, err := fetchJSON[myLimits](cli, apiBase+"/billing-vbai/my_limits", token)
	if err != nil {
		// /my_limits недоступен на некоторых тарифах — не считаем фатальным.
		limits = myLimits{}
	}
	events, err := fetchJSON[[]usageEvent](cli, apiBase+"/billing-vbai/my_usage_events?limit=100", token)
	if err != nil {
		return "", fmt.Errorf("/my_usage_events: %w", err)
	}

	var b strings.Builder

	// === Тариф ===
	state := "активен"
	if !bal.Active {
		state = "НЕ активен"
	}
	fmt.Fprintf(&b, "\n📊 USAGE\n\n")
	fmt.Fprintf(&b, "  Тариф: %s (%s) — %s", bal.TariffTitle, bal.TariffCode, state)
	if bal.ValidUntil != "" {
		fmt.Fprintf(&b, "  ·  до %s", bal.ValidUntil[:10])
	}
	fmt.Fprintf(&b, "\n  Кошелёк: %s ₽\n", bal.BalanceRubles)

	// === Лимиты плана ===
	if len(limits.Windows) > 0 {
		fmt.Fprintf(&b, "\n  Лимиты:\n")
		for _, w := range limits.Windows {
			label := windowLabel(w.Type)
			usedR := float64(w.UsedCopecks) / 100
			limR := float64(w.LimitCopecks) / 100
			remR := float64(w.RemainingCopecks) / 100
			bar := progressBar(w.PercentUsed, 24)
			reset := ""
			if w.ResetInSeconds > 0 {
				reset = "  ·  обновится " + humanDuration(w.ResetInSeconds)
			}
			fmt.Fprintf(&b, "    %-7s %s  %3d%%  %.2f / %.0f ₽  (осталось %.0f ₽)%s\n",
				label, bar, w.PercentUsed, usedR, limR, remR, reset)
		}
	}

	if len(events) == 0 {
		b.WriteString("\n  (нет событий)\n")
		return b.String(), nil
	}

	// === AI итерации (группируем по interaction_id) ===
	type iter struct {
		when     string
		model    string
		provider string
		input    int64
		output   int64
		cached   int64
		costR    float64
	}
	byID := map[string]*iter{}
	order := []string{} // порядок появления (от свежих к старым — events так и приходят)
	totalR := 0.0
	for _, e := range events {
		id := e.InteractionID
		if id == "" {
			id = fmt.Sprintf("e%d", e.ID)
		}
		it, ok := byID[id]
		if !ok {
			it = &iter{when: e.Timestamp, model: e.Model, provider: e.Provider}
			byID[id] = it
			order = append(order, id)
		}
		switch e.EventType {
		case "input_tokens":
			it.input += e.Tokens
		case "output_tokens":
			it.output += e.Tokens
		case "cache_creation_charge", "cached_tokens_adjustment":
			it.cached += e.Tokens
		}
		var c float64
		_, _ = fmt.Sscanf(e.CostRub, "%f", &c)
		it.costR += c
		totalR += c
	}

	// Сортируем по времени (свежие сверху). Берём первые 14 (как в web).
	sort.SliceStable(order, func(i, j int) bool {
		return byID[order[i]].when > byID[order[j]].when
	})
	if len(order) > 14 {
		order = order[:14]
	}

	fmt.Fprintf(&b, "\n  AI итерации (последние %d · итого %.2f ₽):\n", len(order), totalR)
	for _, id := range order {
		it := byID[id]
		ts := it.when
		if len(ts) >= 16 {
			ts = strings.Replace(ts[:16], "T", " ", 1)
		}
		model := it.model
		if len(model) > 22 {
			model = model[:22]
		}
		fmt.Fprintf(&b, "    %s  %-22s  ↓%6d ↑%5d  %6.2f ₽\n", ts, model, it.input, it.output, it.costR)
	}
	return b.String(), nil
}

func windowLabel(t string) string {
	switch t {
	case "5h":
		return "5 час"
	case "day":
		return "день"
	case "week":
		return "неделя"
	case "month":
		return "месяц"
	default:
		return t
	}
}

func progressBar(percent, width int) string {
	if percent > 100 {
		percent = 100
	}
	if percent < 0 {
		percent = 0
	}
	filled := percent * width / 100
	return "[" + strings.Repeat("█", filled) + strings.Repeat("░", width-filled) + "]"
}

func humanDuration(sec int64) string {
	if sec < 60 {
		return fmt.Sprintf("через %dс", sec)
	}
	m := sec / 60
	if m < 60 {
		return fmt.Sprintf("через %dм", m)
	}
	h := m / 60
	m = m % 60
	if h < 24 {
		if m > 0 {
			return fmt.Sprintf("через %dч %dм", h, m)
		}
		return fmt.Sprintf("через %dч", h)
	}
	d := h / 24
	return fmt.Sprintf("через %dд", d)
}

// fetchJSON — generic helper для GET-запроса c Bearer JWT и парсингом JSON.
func fetchJSON[T any](cli *http.Client, url, token string) (T, error) {
	var zero T
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return zero, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := cli.Do(req)
	if err != nil {
		return zero, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return zero, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncStr(string(body), 200))
	}
	var out T
	if err := json.Unmarshal(body, &out); err != nil {
		return zero, fmt.Errorf("json: %w body=%s", err, truncStr(string(body), 200))
	}
	return out, nil
}

func truncStr(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
