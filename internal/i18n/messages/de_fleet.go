// Package messages/de_fleet — deutsche Texte der Subagenten und der Liste
// „wer macht was“ (E101).
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var deFleetMessages = map[string]string{
	"fleet.ask.prefix":      "🤖 Agent «%s» fragt:",
	"fleet.card.write":      "darf ändern",
	"fleet.card.background": "im Hintergrund",

	"fleet.state.queued":    "wartet auf Platz",
	"fleet.state.running":   "arbeitet",
	"fleet.state.waiting":   "wartet auf dich",
	"fleet.state.done":      "fertig",
	"fleet.state.failed":    "Fehler",
	"fleet.state.cancelled": "gestoppt",

	"fleet.started":          "Subagent %s («%s») läuft im Hintergrund. Sein Ergebnis kommt als eigene Nachricht, sobald er fertig ist; um jetzt darauf zu warten, rufe TaskWait mit id %s auf.",
	"fleet.result.timeout":   "der Subagent wurde nicht innerhalb von %s fertig",
	"fleet.result.cancelled": "der Subagent wurde zusammen mit seinem Elternagenten gestoppt",
	"fleet.result.stopped":   "der Subagent wurde vom Menschen gestoppt",
	"fleet.result.limit":     "(beim Schrittlimit von %d gestoppt — das Ergebnis kann unvollständig sein)",
	"fleet.wait.none":        "keine Hintergrund-Subagenten, auf die zu warten wäre",
	"fleet.wait.unknown":     "kein Subagent %s",
	"fleet.note.header":      "Hintergrund-Subagenten sind fertig:",
	"fleet.note.item":        "— «%s» (%s), %s:",
	"fleet.err.model":        "Modell %q ist hier nicht wählbar — der Subagent läuft auf dem aktuellen Modell von %s",
	"fleet.err.cwdOutside":   "cwd %s liegt außerhalb des Verzeichnisses des Elternagenten %s",
	"fleet.err.cwdMissing":   "cwd %s ist kein Verzeichnis",

	"fleet.tui.header":          "🤖 Agenten",
	"fleet.tui.more":            "… %d weitere — /agents",
	"fleet.tui.listHeader":      "Subagenten (neueste unten):",
	"fleet.tui.listEmpty":       "Noch keine Subagenten. Der Agent startet sie mit dem Werkzeug Task.",
	"fleet.tui.listHint":        "/agents open <n> — Verlauf · /agents stop <n> — stoppen",
	"fleet.tui.openHeader":      "Agent «%s» · %s · %s",
	"fleet.tui.openEmpty":       "(der Verlauf ist noch leer)",
	"fleet.tui.notFound":        "kein Agent %s — siehe /agents",
	"fleet.tui.stopping":        "stoppe Agent «%s»",
	"fleet.tui.notRunning":      "Agent «%s» ist bereits fertig",
	"fleet.tui.usage":           "Aufruf: /agents · /agents open <n> · /agents stop <n>",
	"fleet.tui.autoReply":       "🤖 Hintergrund-Agenten sind fertig — ihre Ergebnisse gehen an den Agenten",
	"fleet.tui.cardDone":        "%s Agent «%s» — %s",
	"fleet.tui.approveAgent":    "🤖 fragt Agent «%s»",
	"fleet.tui.stoppedChildren": "gestoppte Subagenten: %d",
	"status.agents":             "Agenten",
	"hint.agents":               "Subagenten: Liste · open <n> · stop <n>",

	"fleet.ide.notFound": "kein solcher Agent: %s",
	"fleet.ide.stopping": "stoppe Agent «%s»",
}

func init() { i18n.Register("de", deFleetMessages) }
