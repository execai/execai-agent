// Package messages/en_usage — English strings for the /usage command output.
// Merged into the "en" catalog via i18n.Register (see i18n.Register merge semantics).
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var enUsageMessages = map[string]string{
	// === /usage main block (ExecAI billing) ===
	"usage.header":       "📊 USAGE",
	"usage.plan":         "Plan: %s (%s) — %s",
	"usage.active":       "active",
	"usage.notActive":    "NOT active",
	"usage.until":        "until %s",
	"usage.wallet":       "Wallet: %s ₽",
	"usage.limits":       "Limits:",
	"usage.remainingRub": "(%.0f ₽ left)",
	"usage.remainingFmt": "(%s left)",
	"usage.noEvents":     "(no events)",
	"usage.iterations":   "AI iterations (last %d · total %.2f ₽):",

	// === Limit window labels ===
	"usage.window.5h":    "5 hr",
	"usage.window.day":   "day",
	"usage.window.week":  "week",
	"usage.window.month": "month",

	// === Reset countdowns (humanDuration) ===
	"usage.resetIn.sec":     "in %ds",
	"usage.resetIn.min":     "in %dm",
	"usage.resetIn.hour":    "in %dh",
	"usage.resetIn.hourMin": "in %dh %dm",
	"usage.resetIn.day":     "in %dd",
	"usage.resets":          "resets %s",
	"usage.refreshing":      "refreshing",

	// === Kimi Code Coding Plan ===
	"usage.kimi.header":         "🔌 Kimi Code (kimi.com/code)",
	"usage.kimi.plan":           "Plan: %s (by available models)",
	"usage.kimi.models":         "Models: %s",
	"usage.kimi.rollingWindows": "Rolling windows:",
	"usage.kimi.manage":         "ⓘ Manage subscription: %s",

	// === Rolling-window unit labels ===
	"usage.unit.hour": "%dh",
	"usage.unit.min":  "%dmin",
	"usage.unit.day":  "%dd",
	"usage.window.n":  "window #%d",

	// === Anthropic-compatible sources (local requests.log counter) ===
	"usage.source.header":        "🔌 %s source",
	"usage.source.emptyLog":      "🔌 %s source\n\nLocal request log is empty. Actual quota: %s",
	"usage.source.noRequests":    "(no requests in the local log)",
	"usage.source.quotaAt":       "Check your actual quota at %s",
	"usage.source.totalRequests": "Total requests in log:  %d",
	"usage.source.totalInput":    "Total input tokens:     %s",
	"usage.source.totalOutput":   "Total output tokens:    %s",
	"usage.source.cacheTokens":   "Cache tokens:           %s",
	"usage.source.last24h":       "Last 24h:               %d requests · %s input · %s output",
	"usage.source.byModel":       "By model:",
	"usage.source.modelLine":     "%-22s  %4d req · ↓%s ↑%s",
	"usage.source.errors":        "  ⚠ %d errors",
	"usage.localCounterNote":     "ⓘ This is a local counter. Actual billing and quotas:",
}

func init() {
	i18n.Register("en", enUsageMessages)
}
