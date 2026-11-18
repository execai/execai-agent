// Package messages/ru_fleet — русские строки субагентов и списка «кто что
// делает» (E101): инструмент Task, блок агентов в терминале и /agents,
// панель редактора.
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var ruFleetMessages = map[string]string{
	"fleet.ask.prefix":      "🤖 агент «%s» просит:",
	"fleet.card.write":      "может править",
	"fleet.card.background": "в фоне",

	"fleet.state.queued":    "в очереди",
	"fleet.state.running":   "работает",
	"fleet.state.waiting":   "ждёт ответа",
	"fleet.state.done":      "готов",
	"fleet.state.failed":    "ошибка",
	"fleet.state.cancelled": "остановлен",

	"fleet.started":          "Субагент %s («%s») запущен в фоне. Итог придёт отдельным сообщением, когда он закончит; дождаться сейчас — TaskWait с id %s.",
	"fleet.result.timeout":   "субагент не уложился в %s (предел — переменная EXECAI_SUBAGENT_TIMEOUT, 0 — без предела)",
	"fleet.result.cancelled": "субагент остановлен вместе с родителем",
	"fleet.result.stopped":   "субагента остановил человек",
	"fleet.result.limit":     "(остановлен на пределе в %d шагов — итог может быть неполным; продолжить с того же места — TaskExtend с id %s)",
	"fleet.wait.none":        "фоновых субагентов, которых надо ждать, нет",
	"fleet.wait.unknown":     "нет субагента %s",
	"fleet.note.header":      "Фоновые субагенты закончили:",
	"fleet.note.item":        "— «%s» (%s), %s:",
	"fleet.err.model":        "модель %q здесь выбрать нельзя — субагент работает на текущей модели %s",
	"fleet.err.cwdOutside":   "cwd %s выше каталога родителя %s",
	"fleet.err.cwdMissing":   "cwd %s — не каталог",

	"fleet.tui.header":          "🤖 Агенты",
	"fleet.tui.more":            "… ещё %d — /agents",
	"fleet.tui.listHeader":      "Субагенты (новые внизу):",
	"fleet.tui.listEmpty":       "Субагентов пока нет. Агент запускает их инструментом Task.",
	"fleet.tui.listHint":        "/agents open <n> — транскрипт · /agents stop <n> — остановить · /agents more <n> [шагов] — дать ещё шагов · /agents limit [N] — шагов новым",
	"fleet.tui.openHeader":      "Агент «%s» · %s · %s",
	"fleet.tui.openEmpty":       "(транскрипт пока пуст)",
	"fleet.tui.notFound":        "нет агента %s — см. /agents",
	"fleet.tui.stopping":        "останавливаю агента «%s»",
	"fleet.tui.notRunning":      "агент «%s» уже закончил",
	"fleet.tui.usage":           "использование: /agents · /agents open <n> · /agents stop <n> · /agents more <n> [шагов] · /agents limit [N]",
	"fleet.tui.autoReply":       "🤖 фоновые агенты закончили — их итог передан агенту",
	"fleet.tui.cardDone":        "%s агент «%s» — %s",
	"fleet.tui.approveAgent":    "🤖 спрашивает агент «%s»",
	"fleet.tui.stoppedChildren": "остановлено субагентов: %d",
	"status.agents":             "агенты",
	"hint.agents":               "субагенты: список · open <n> · stop <n> · more <n> · limit <N>",
	"fleet.extend.raised":       "агенту «%s» добавлено %d шагов — теперь шаг %d из %d",
	"fleet.extend.resumed":      "агент «%s» продолжает с шага %d: ещё %d шагов (предел %d). Итог придёт отдельным сообщением или через TaskWait с id %s.",
	"fleet.extend.ceiling":      "у агента «%s» уже наибольший предел — %d шагов",
	"fleet.extend.finished":     "агент «%s» уже закончил (%s), а не упёрся в предел шагов — продлевать нечего; нужна ещё работа — новый Task",
	"fleet.extend.prompt":       "Тебе дали ещё %d шагов. Продолжай с того места, где остановился, доведи задачу до конца и в последнем сообщении дай итог целиком.",
	"fleet.tui.steps":           "шаг %d/%d · ещё %d",
	"fleet.tui.atLimit":         "упёрся в предел шагов",
	"fleet.tui.moreHint":        "мало шагов — /agents more %s",
	"fleet.tui.limitShow":       "Шагов новому субагенту: %d (по умолчанию %d, можно 1–%d). Поменять: /agents limit <N>",
	"fleet.tui.limitSet":        "новые субагенты получат %d шагов",
	"fleet.tui.limitBad":        "нужно число от 1 до %d",

	"fleet.ide.notFound": "нет такого агента: %s",
	"fleet.ide.stopping": "останавливаю агента «%s»",
}

func init() { i18n.Register("ru", ruFleetMessages) }
