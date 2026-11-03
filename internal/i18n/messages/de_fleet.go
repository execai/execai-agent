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
	"fleet.result.timeout":   "der Subagent wurde nicht innerhalb von %s fertig (Limit über EXECAI_SUBAGENT_TIMEOUT, 0 = kein Limit)",
	"fleet.result.cancelled": "der Subagent wurde zusammen mit seinem Elternagenten gestoppt",
	"fleet.result.stopped":   "der Subagent wurde vom Menschen gestoppt",
	"fleet.result.limit":     "(beim Schrittlimit von %d gestoppt — das Ergebnis kann unvollständig sein; um dort weiterzumachen, TaskExtend mit id %s)",
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
	"fleet.tui.listHint":        "/agents open <n> — Verlauf · /agents stop <n> — stoppen · /agents more <n> [Schritte] — mehr Schritte geben · /agents limit [N] — Schritte für neue",
	"fleet.tui.openHeader":      "Agent «%s» · %s · %s",
	"fleet.tui.openEmpty":       "(der Verlauf ist noch leer)",
	"fleet.tui.notFound":        "kein Agent %s — siehe /agents",
	"fleet.tui.stopping":        "stoppe Agent «%s»",
	"fleet.tui.notRunning":      "Agent «%s» ist bereits fertig",
	"fleet.tui.usage":           "Aufruf: /agents · /agents open <n> · /agents stop <n> · /agents more <n> [Schritte] · /agents limit [N]",
	"fleet.tui.autoReply":       "🤖 Hintergrund-Agenten sind fertig — ihre Ergebnisse gehen an den Agenten",
	"fleet.tui.cardDone":        "%s Agent «%s» — %s",
	"fleet.tui.approveAgent":    "🤖 fragt Agent «%s»",
	"fleet.tui.stoppedChildren": "gestoppte Subagenten: %d",
	"status.agents":             "Agenten",
	"hint.agents":               "Subagenten: Liste · open <n> · stop <n> · more <n> · limit <N>",
	"fleet.extend.raised":       "Agent «%s» bekam %d Schritte mehr — jetzt bei Schritt %d von %d",
	"fleet.extend.resumed":      "Agent «%s» macht ab Schritt %d weiter: %d Schritte mehr (Limit %d). Das Ergebnis kommt als eigene Nachricht oder über TaskWait mit id %s.",
	"fleet.extend.ceiling":      "Agent «%s» hat schon das höchste Limit — %d Schritte",
	"fleet.extend.finished":     "Agent «%s» ist schon fertig (%s) und nicht am Schrittlimit gestoppt — nichts zu verlängern; für mehr Arbeit einen neuen Task starten",
	"fleet.extend.prompt":       "Du hast %d weitere Schritte bekommen. Mach dort weiter, wo du aufgehört hast, bring die Aufgabe zu Ende und gib in deiner letzten Nachricht das ganze Ergebnis.",
	"fleet.tui.steps":           "Schritt %d/%d · noch %d",
	"fleet.tui.atLimit":         "Schrittlimit erreicht",
	"fleet.tui.moreHint":        "wenige Schritte übrig — /agents more %s",
	"fleet.tui.limitShow":       "Schritte für einen neuen Subagenten: %d (Standard %d, erlaubt 1–%d). Ändern: /agents limit <N>",
	"fleet.tui.limitSet":        "neue Subagenten bekommen %d Schritte",
	"fleet.tui.limitBad":        "eine Zahl von 1 bis %d nötig",

	"fleet.ide.notFound": "kein solcher Agent: %s",
	"fleet.ide.stopping": "stoppe Agent «%s»",
}

func init() { i18n.Register("de", deFleetMessages) }
