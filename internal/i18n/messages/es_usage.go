// Package messages/es_usage — cadenas en español para la salida del comando /usage.
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var esUsageMessages = map[string]string{
	// === Bloque principal de /usage (facturación ExecAI) ===
	"usage.header":       "📊 USAGE",
	"usage.plan":         "Plan: %s (%s) — %s",
	"usage.active":       "activo",
	"usage.notActive":    "NO activo",
	"usage.until":        "hasta %s",
	"usage.wallet":       "Monedero: %s ₽",
	"usage.limits":       "Límites:",
	"usage.remainingRub": "(quedan %.0f ₽)",
	"usage.remainingFmt": "(quedan %s)",
	"usage.noEvents":     "(sin eventos)",
	"usage.iterations":   "Iteraciones AI (últimas %d · total %.2f ₽):",

	// === Etiquetas de ventanas de límites ===
	"usage.window.5h":    "5 h",
	"usage.window.day":   "día",
	"usage.window.week":  "semana",
	"usage.window.month": "mes",

	// === Cuenta atrás hasta el reinicio (humanDuration) ===
	"usage.resetIn.sec":     "en %ds",
	"usage.resetIn.min":     "en %dm",
	"usage.resetIn.hour":    "en %dh",
	"usage.resetIn.hourMin": "en %dh %dm",
	"usage.resetIn.day":     "en %dd",
	"usage.resets":          "se renueva %s",
	"usage.refreshing":      "renovando",

	// === Kimi Code Coding Plan ===
	"usage.kimi.header":         "🔌 Kimi Code (kimi.com/code)",
	"usage.kimi.plan":           "Plan: %s (según los modelos disponibles)",
	"usage.kimi.models":         "Modelos: %s",
	"usage.kimi.rollingWindows": "Ventanas rolling:",
	"usage.kimi.manage":         "ⓘ Gestión de la suscripción: %s",

	// === Etiquetas de unidades de ventanas rolling ===
	"usage.unit.hour": "%dh",
	"usage.unit.min":  "%dmin",
	"usage.unit.day":  "%dd",
	"usage.window.n":  "ventana #%d",

	// === Sources compatibles con Anthropic (contador local requests.log) ===
	"usage.source.header":        "🔌 Fuente %s",
	"usage.source.emptyLog":      "🔌 Fuente %s\n\nEl registro local de solicitudes está vacío. Cuota real: %s",
	"usage.source.noRequests":    "(no hay solicitudes en el registro local)",
	"usage.source.quotaAt":       "Consulta la cuota real en %s",
	"usage.source.totalRequests": "Solicitudes totales en el registro: %d",
	"usage.source.totalInput":    "Total input tokens:                 %s",
	"usage.source.totalOutput":   "Total output tokens:                %s",
	"usage.source.cacheTokens":   "Tokens de caché:                    %s",
	"usage.source.last24h":       "Últimas 24 h:                       %d solicitudes · %s input · %s output",
	"usage.source.byModel":       "Por modelo:",
	"usage.source.modelLine":     "%-22s  %4d sol · ↓%s ↑%s",
	"usage.source.errors":        "  ⚠ %d errores",
	"usage.localCounterNote":     "ⓘ Este es un contador local. Facturación y cuotas reales:",
}

func init() {
	i18n.Register("es", esUsageMessages)
}
