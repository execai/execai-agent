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
	"fleet.result.timeout":   "субагент не уложился в %s",
	"fleet.result.cancelled": "субагент остановлен вместе с родителем",
	"fleet.result.stopped":   "субагента остановил человек",
	"fleet.result.limit":     "(остановлен на пределе в %d шагов — итог может быть неполным)",
	"fleet.wait.none":        "фоновых субагентов, которых надо ждать, нет",
	"fleet.wait.unknown":     "нет субагента %s",
	"fleet.note.header":      "Фоновые субагенты закончили:",
	"fleet.note.item":        "— «%s» (%s), %s:",
	"fleet.err.model":        "модель %q здесь не выбрать — субагент работает на текущей модели %s",
	"fleet.err.cwdOutside":   "cwd %s выше каталога родителя %s",
	"fleet.err.cwdMissing":   "cwd %s — не каталог",

	"fleet.tui.header":          "🤖 Агенты",
	"fleet.tui.more":            "… ещё %d — /agents",
	"fleet.tui.listHeader":      "Субагенты (новые внизу):",
	"fleet.tui.listEmpty":       "Субагентов пока нет. Агент запускает их инструментом Task.",
	"fleet.tui.listHint":        "/agents open <n> — транскрипт · /agents stop <n> — остановить",
	"fleet.tui.openHeader":      "Агент «%s» · %s · %s",
	"fleet.tui.openEmpty":       "(транскрипт пока пуст)",
	"fleet.tui.notFound":        "нет агента %s — см. /agents",
	"fleet.tui.stopping":        "останавливаю агента «%s»",
	"fleet.tui.notRunning":      "агент «%s» уже закончил",
	"fleet.tui.usage":           "использование: /agents · /agents open <n> · /agents stop <n>",
	"fleet.tui.autoReply":       "🤖 фоновые агенты закончили — их итог передан агенту",
	"fleet.tui.cardDone":        "%s агент «%s» — %s",
	"fleet.tui.approveAgent":    "🤖 спрашивает агент «%s»",
	"fleet.tui.stoppedChildren": "остановлено субагентов: %d",
	"status.agents":             "агенты",
	"hint.agents":               "субагенты: список · open <n> · stop <n>",

	"fleet.ide.notFound": "нет такого агента: %s",
	"fleet.ide.stopping": "останавливаю агента «%s»",
}

func init() { i18n.Register("ru", ruFleetMessages) }
