// Package messages/ru_usage — русские строки вывода команды /usage.
// Значения = оригинальные строки из internal/chat/usage.go (поведение для ru идентично).
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var ruUsageMessages = map[string]string{
	// === /usage основной блок (биллинг ExecAI) ===
	"usage.header":       "📊 USAGE",
	"usage.plan":         "Тариф: %s (%s) — %s",
	"usage.active":       "активен",
	"usage.notActive":    "НЕ активен",
	"usage.until":        "до %s",
	"usage.wallet":       "Кошелёк: %s ₽",
	"usage.limits":       "Лимиты:",
	"usage.remainingRub": "(осталось %.0f ₽)",
	"usage.remainingFmt": "(осталось %s)",
	"usage.noEvents":     "(нет событий)",
	"usage.iterations":   "AI итерации (последние %d · итого %.2f ₽):",

	// === Метки окон лимитов ===
	"usage.window.5h":    "5 час",
	"usage.window.day":   "день",
	"usage.window.week":  "неделя",
	"usage.window.month": "месяц",

	// === Обратный отсчёт до сброса (humanDuration) ===
	"usage.resetIn.sec":     "через %dс",
	"usage.resetIn.min":     "через %dм",
	"usage.resetIn.hour":    "через %dч",
	"usage.resetIn.hourMin": "через %dч %dм",
	"usage.resetIn.day":     "через %dд",
	"usage.resets":          "обновится %s",
	"usage.refreshing":      "обновление",

	// === Kimi Code Coding Plan ===
	"usage.kimi.header":         "🔌 Kimi Code (kimi.com/code)",
	"usage.kimi.plan":           "Тариф: %s (по доступным моделям)",
	"usage.kimi.models":         "Модели: %s",
	"usage.kimi.rollingWindows": "Rolling-окна:",
	"usage.kimi.manage":         "ⓘ Управление подпиской: %s",

	// === Метки единиц rolling-окон ===
	"usage.unit.hour": "%dч",
	"usage.unit.min":  "%dмин",
	"usage.unit.day":  "%dд",
	"usage.window.n":  "окно #%d",

	// === Anthropic-совместимые source'ы (локальный счётчик requests.log) ===
	"usage.source.header":        "🔌 %s source",
	"usage.source.emptyLog":      "🔌 %s source\n\nЛокальный лог запросов пуст. Реальная квота: %s",
	"usage.source.noRequests":    "(нет запросов в локальном логе)",
	"usage.source.quotaAt":       "Реальную квоту смотри на %s",
	"usage.source.totalRequests": "Всего запросов в логе:  %d",
	"usage.source.totalInput":    "Суммарно input tokens:  %s",
	"usage.source.totalOutput":   "Суммарно output tokens: %s",
	"usage.source.cacheTokens":   "Cache tokens:           %s",
	"usage.source.last24h":       "За последние 24ч:       %d запросов · %s input · %s output",
	"usage.source.byModel":       "По моделям:",
	"usage.source.modelLine":     "%-22s  %4d запр · ↓%s ↑%s",
	"usage.source.errors":        "  ⚠ %d ошибок",
	"usage.localCounterNote":     "ⓘ Это локальный счётчик. Реальный биллинг и квоты:",
}

func init() {
	i18n.Register("ru", ruUsageMessages)
}
