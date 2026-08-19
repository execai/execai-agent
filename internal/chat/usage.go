package chat

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/velesbsdllc/agent-vbai/internal/version"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

// myBalance — response of /billing-vbai/my_balance.
type myBalance struct {
	BalanceRubles string `json:"balance_rubles"`
	TariffCode    string `json:"tariff_code"`
	TariffTitle   string `json:"tariff_title"`
	Active        bool   `json:"active"`
	ValidUntil    string `json:"valid_until"`
}

// limitWindow — a rate-limit window from /my_limits.
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

// usageEvent — a single row of /billing-vbai/my_usage_events.
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

// fetchUsageForSource — dispatcher. Takes the active source from subs and calls
// the corresponding fetchUsage* (ExecAI / Z.ai / Anthropic / Kimi / ...).
func fetchUsageForSource(apiBase, token string, subs *subscriptions.Store) (string, error) {
	if subs != nil {
		switch subs.Active {
		case subscriptions.SourceZAI:
			return fetchUsageAnthropicCompat("zai", "Z.ai Coding Plan", "https://z.ai/manage-apikey/subscription")
		case subscriptions.SourceAnthropic:
			return fetchUsageAnthropicCompat("anthropic", "Anthropic API", "https://console.anthropic.com/settings/usage")
		case subscriptions.SourceKimi:
			// Kimi Code Coding Plan — has a quota API.
			if sub, ok := subs.Subscriptions[subscriptions.SourceKimi]; ok {
				return fetchKimiUsage(sub)
			}
		case subscriptions.SourceZAIAPI:
			// Z.ai open platform pay-per-token — same story as Moonshot: no quota
			// endpoint, only a local counter + a link to their billing console.
			return fetchUsageAnthropicCompat("zai-api", "Z.ai open platform (pay-per-token)", "https://z.ai/manage-apikey/apikey-list")
		case subscriptions.SourceKimiAPI:
			// Moonshot Platform pay-per-token — no quota, only a local counter +
			// a link to the dashboard with the real billing.
			return fetchUsageAnthropicCompat("kimi-api", "Moonshot Platform (pay-per-token)", "https://platform.moonshot.ai/console")
		case subscriptions.SourceOpenRouter:
			// OpenRouter bills per token across every vendor; the balance lives
			// in their dashboard, so point there next to the local counter.
			return fetchUsageAnthropicCompat("openrouter", "OpenRouter (pay-per-token)", "https://openrouter.ai/credits")
		}
	}
	return fetchUsage(apiBase, token)
}

// fetchKimiUsage — GET api.kimi.com/coding/v1/usages for the Kimi Code Coding Plan.
// Returns the weekly quota + rolling windows (e.g. 5h).
// The response format is documented in MoonshotAI/kimi-code/packages/oauth/src/managed-usage.ts:
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
	req.Header.Set("User-Agent", version.UserAgent())
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

	// The server returns used/limit sometimes as a number, sometimes as a string (see
	// MoonshotAI/kimi-code managed-usage.ts: toInt(v) accepts both number and string).
	// Catch both formats via json.RawMessage → parseFlexInt.
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
	b.WriteString("\n" + i18n.T("usage.kimi.header") + "\n\n")

	// Plan derived from the set of available models.
	if tier := kimiTierFromModels(sub.AvailableModels); tier != "" {
		fmt.Fprintf(&b, "  %s\n", i18n.Tf("usage.kimi.plan", tier))
	}
	if len(sub.AvailableModels) > 0 {
		fmt.Fprintf(&b, "  %s\n", i18n.Tf("usage.kimi.models", strings.Join(sub.AvailableModels, ", ")))
	}

	// Weekly quota.
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
		fmt.Fprintf(&b, "\n  %s:\n    %s  %d%%  %s / %s  %s",
			name,
			progressBar(percent, 24),
			percent,
			humanInt(usageUsed),
			humanInt(usageLimit),
			i18n.Tf("usage.remainingFmt", humanInt(remaining)),
		)
		if hint := kimiResetHint(raw.Usage.ResetAt); hint != "" {
			b.WriteString("  ·  " + hint)
		}
		b.WriteString("\n")
	}

	// Rolling windows (5h etc.).
	if len(raw.Limits) > 0 {
		b.WriteString("\n  " + i18n.T("usage.kimi.rollingWindows") + "\n")
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
			fmt.Fprintf(&b, "    %-12s %s  %3d%%  %s / %s  %s",
				name,
				progressBar(percent, 20),
				percent,
				humanInt(used),
				humanInt(limit),
				i18n.Tf("usage.remainingFmt", humanInt(remaining)),
			)
			if hint := kimiResetHint(lm.Detail.ResetAt); hint != "" {
				b.WriteString("  ·  " + hint)
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("\n  " + i18n.Tf("usage.kimi.manage", "https://www.kimi.com/code/console") + "\n")
	return b.String(), nil
}

// parseFlexInt — parses json.RawMessage as int64, accepting both a number (42) and a string ("42").
// The Kimi Code /usages endpoint sends numeric fields in both formats depending on the version
// (see MoonshotAI/kimi-code/packages/oauth/src/managed-usage.ts::toInt).
func parseFlexInt(raw json.RawMessage) int64 {
	if len(raw) == 0 {
		return 0
	}
	// Number?
	var n int64
	if err := json.Unmarshal(raw, &n); err == nil {
		return n
	}
	// Float?
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return int64(f)
	}
	// String "42"?
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

// kimiTierFromModels — readable plan name based on the models available on the subscription.
// Duplicates subscriptions.deriveKimiTier, but is called from the internal package.
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

// kimiWindowLabel — human-readable rolling-window name from duration+timeUnit.
func kimiWindowLabel(duration int64, timeUnit string, idx int) string {
	tu := strings.ToUpper(timeUnit)
	if duration > 0 {
		switch {
		case strings.Contains(tu, "MINUTE"):
			if duration >= 60 && duration%60 == 0 {
				return i18n.Tf("usage.unit.hour", duration/60)
			}
			return i18n.Tf("usage.unit.min", duration)
		case strings.Contains(tu, "HOUR"):
			return i18n.Tf("usage.unit.hour", duration)
		case strings.Contains(tu, "DAY"):
			return i18n.Tf("usage.unit.day", duration)
		}
	}
	return i18n.Tf("usage.window.n", idx+1)
}

// kimiResetHint — formats resetAt (ISO) as a localized "resets in X" hint.
func kimiResetHint(iso string) string {
	if iso == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		// Try nano-precision RFC3339 with truncation.
		if len(iso) > 30 {
			t, err = time.Parse(time.RFC3339Nano, iso)
		}
		if err != nil {
			return i18n.Tf("usage.resets", iso)
		}
	}
	diff := int64(time.Until(t).Seconds())
	if diff <= 0 {
		return i18n.T("usage.refreshing")
	}
	return i18n.Tf("usage.resets", humanDuration(diff))
}

// fetchUsageAnthropicCompat — common aggregate for Anthropic-compatible sources
// (zai-anthropic, anthropic). Parses requests.log by source prefix,
// aggregates tokens, links to the authoritative source.
func fetchUsageAnthropicCompat(sourcePrefix, displayName, authoritativeURL string) (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "requests.log")
	f, err := os.Open(path)
	if err != nil {
		return i18n.Tf("usage.source.emptyLog", displayName, authoritativeURL), nil
	}
	defer f.Close()
	type stat struct {
		count     int
		inputTok  int64
		outputTok int64
		cacheTok  int64
		errs      int
		firstTs   string
		lastTs    string
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
	fmt.Fprintf(&b, "\n%s\n\n", i18n.Tf("usage.source.header", displayName))
	if total.count == 0 {
		b.WriteString("  " + i18n.T("usage.source.noRequests") + "\n")
		fmt.Fprintf(&b, "\n%s", i18n.Tf("usage.source.quotaAt", authoritativeURL))
		return b.String(), nil
	}
	fmt.Fprintf(&b, "  %s\n", i18n.Tf("usage.source.totalRequests", total.count))
	fmt.Fprintf(&b, "  %s\n", i18n.Tf("usage.source.totalInput", humanInt(total.inputTok)))
	fmt.Fprintf(&b, "  %s\n", i18n.Tf("usage.source.totalOutput", humanInt(total.outputTok)))
	if total.cacheTok > 0 {
		fmt.Fprintf(&b, "  %s\n", i18n.Tf("usage.source.cacheTokens", humanInt(total.cacheTok)))
	}
	fmt.Fprintf(&b, "\n  %s\n",
		i18n.Tf("usage.source.last24h", last24Cnt, humanInt(last24In), humanInt(last24Out)))

	fmt.Fprintf(&b, "\n  %s\n", i18n.T("usage.source.byModel"))
	for k, st := range byModel {
		fmt.Fprintf(&b, "    %s",
			i18n.Tf("usage.source.modelLine", k, st.count, humanInt(st.inputTok), humanInt(st.outputTok)))
		if st.errs > 0 {
			b.WriteString(i18n.Tf("usage.source.errors", st.errs))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n  " + i18n.T("usage.localCounterNote") + "\n")
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

// fetchUsage returns the text block for /usage — same format as the web page.
func fetchUsage(apiBase, token string) (string, error) {
	cli := &http.Client{Timeout: 8 * time.Second}

	bal, err := fetchJSON[myBalance](cli, apiBase+"/billing-vbai/my_balance", token)
	if err != nil {
		return "", fmt.Errorf("/my_balance: %w", err)
	}
	limits, err := fetchJSON[myLimits](cli, apiBase+"/billing-vbai/my_limits", token)
	if err != nil {
		// /my_limits is unavailable on some plans — not considered fatal.
		limits = myLimits{}
	}
	events, err := fetchJSON[[]usageEvent](cli, apiBase+"/billing-vbai/my_usage_events?limit=100", token)
	if err != nil {
		return "", fmt.Errorf("/my_usage_events: %w", err)
	}

	var b strings.Builder

	// === Plan ===
	state := i18n.T("usage.active")
	if !bal.Active {
		state = i18n.T("usage.notActive")
	}
	fmt.Fprintf(&b, "\n%s\n\n", i18n.T("usage.header"))
	fmt.Fprintf(&b, "  %s", i18n.Tf("usage.plan", bal.TariffTitle, bal.TariffCode, state))
	if bal.ValidUntil != "" {
		fmt.Fprintf(&b, "  ·  %s", i18n.Tf("usage.until", bal.ValidUntil[:10]))
	}
	fmt.Fprintf(&b, "\n  %s\n", i18n.Tf("usage.wallet", bal.BalanceRubles))

	// === Plan limits ===
	if len(limits.Windows) > 0 {
		fmt.Fprintf(&b, "\n  %s\n", i18n.T("usage.limits"))
		for _, w := range limits.Windows {
			label := windowLabel(w.Type)
			usedR := float64(w.UsedCopecks) / 100
			limR := float64(w.LimitCopecks) / 100
			remR := float64(w.RemainingCopecks) / 100
			bar := progressBar(w.PercentUsed, 24)
			reset := ""
			if w.ResetInSeconds > 0 {
				reset = "  ·  " + i18n.Tf("usage.resets", humanDuration(w.ResetInSeconds))
			}
			fmt.Fprintf(&b, "    %-7s %s  %3d%%  %.2f / %.0f ₽  %s%s\n",
				label, bar, w.PercentUsed, usedR, limR,
				i18n.Tf("usage.remainingRub", remR), reset)
		}
	}

	if len(events) == 0 {
		b.WriteString("\n  " + i18n.T("usage.noEvents") + "\n")
		return b.String(), nil
	}

	// === AI iterations (grouped by interaction_id) ===
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
	order := []string{} // appearance order (newest to oldest — events arrive that way)
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

	// Sort by time (newest on top). Take the first 14 (like the web).
	sort.SliceStable(order, func(i, j int) bool {
		return byID[order[i]].when > byID[order[j]].when
	})
	if len(order) > 14 {
		order = order[:14]
	}

	fmt.Fprintf(&b, "\n  %s\n", i18n.Tf("usage.iterations", len(order), totalR))
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
		return i18n.T("usage.window.5h")
	case "day":
		return i18n.T("usage.window.day")
	case "week":
		return i18n.T("usage.window.week")
	case "month":
		return i18n.T("usage.window.month")
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
		return i18n.Tf("usage.resetIn.sec", sec)
	}
	m := sec / 60
	if m < 60 {
		return i18n.Tf("usage.resetIn.min", m)
	}
	h := m / 60
	m = m % 60
	if h < 24 {
		if m > 0 {
			return i18n.Tf("usage.resetIn.hourMin", h, m)
		}
		return i18n.Tf("usage.resetIn.hour", h)
	}
	d := h / 24
	return i18n.Tf("usage.resetIn.day", d)
}

// fetchJSON — generic helper for a GET request with Bearer JWT and JSON parsing.
func fetchJSON[T any](cli *http.Client, url, token string) (T, error) {
	var zero T
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return zero, err
	}
	req.Header.Set("User-Agent", version.UserAgent())
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
