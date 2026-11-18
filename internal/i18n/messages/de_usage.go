// Package messages/de_usage — deutsche Strings für die Ausgabe des /usage-Befehls.
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var deUsageMessages = map[string]string{
	// === /usage Hauptblock (ExecAI-Abrechnung) ===
	"usage.header":       "📊 USAGE",
	"usage.plan":         "Tarif: %s (%s) — %s",
	"usage.active":       "aktiv",
	"usage.notActive":    "NICHT aktiv",
	"usage.until":        "bis %s",
	"usage.wallet":       "Guthaben: %s ₽",
	"usage.limits":       "Limits:",
	"usage.remainingRub": "(%.0f ₽ übrig)",
	"usage.remainingFmt": "(%s übrig)",
	"usage.noEvents":     "(keine Ereignisse)",
	"usage.iterations":   "AI-Iterationen (letzte %d · gesamt %.2f ₽):",

	// === Limit-Fenster-Beschriftungen ===
	"usage.window.5h":    "5 Std",
	"usage.window.day":   "Tag",
	"usage.window.week":  "Woche",
	"usage.window.month": "Monat",

	// === Countdown bis zum Reset (humanDuration) ===
	"usage.resetIn.sec":     "in %ds",
	"usage.resetIn.min":     "in %dm",
	"usage.resetIn.hour":    "in %dh",
	"usage.resetIn.hourMin": "in %dh %dm",
	"usage.resetIn.day":     "in %dd",
	"usage.resets":          "erneuert sich %s",
	"usage.refreshing":      "wird erneuert",

	// === Kimi Code Coding Plan ===
	"usage.kimi.header":         "🔌 Kimi Code (kimi.com/code)",
	"usage.kimi.plan":           "Tarif: %s (nach verfügbaren Modellen)",
	"usage.kimi.models":         "Modelle: %s",
	"usage.kimi.rollingWindows": "Rolling-Fenster:",
	"usage.kimi.manage":         "ⓘ Abo-Verwaltung: %s",

	// === Einheiten-Beschriftungen der Rolling-Fenster ===
	"usage.unit.hour": "%dh",
	"usage.unit.min":  "%dmin",
	"usage.unit.day":  "%dd",
	"usage.window.n":  "Fenster #%d",

	// === Anthropic-kompatible Sources (lokaler Zähler requests.log) ===
	"usage.source.header":        "🔌 Quelle %s",
	"usage.source.emptyLog":      "🔌 Quelle %s\n\nLokales Anfragen-Log ist leer. Tatsächliches Kontingent: %s",
	"usage.source.noRequests":    "(keine Anfragen im lokalen Log)",
	"usage.source.quotaAt":       "Tatsächliches Kontingent unter %s",
	"usage.source.totalRequests": "Anfragen im Log gesamt:  %d",
	"usage.source.totalInput":    "Input-Tokens gesamt:     %s",
	"usage.source.totalOutput":   "Output-Tokens gesamt:    %s",
	"usage.source.cacheTokens":   "Cache-Tokens:            %s",
	"usage.source.last24h":       "Letzte 24h:              %d Anfragen · %s input · %s output",
	"usage.source.byModel":       "Nach Modell:",
	"usage.source.modelLine":     "%-22s  %4d Anf · ↓%s ↑%s",
	"usage.source.errors":        "  ⚠ %d Fehler",
	"usage.localCounterNote":     "ⓘ Dies ist ein lokaler Zähler. Tatsächliche Abrechnung und Kontingente:",
}

func init() {
	i18n.Register("de", deUsageMessages)
}
