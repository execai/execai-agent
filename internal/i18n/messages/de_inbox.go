// Package messages/de_inbox — deutsche Strings des Kanals „Web → Agent“:
// Aufgaben von der Website, die execai serve oder das Editor-Panel annimmt.
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var deInboxMessages = map[string]string{
	"inbox.err.parse": "Serverantwort konnte nicht gelesen werden",

	"inbox.lock.busy":         "die Inbox-Sperre wechselt ständig den Besitzer, bitte erneut versuchen",
	"inbox.lock.heldByIDE":    "Aufgaben von der Website nimmt bereits ein Editorfenster an (PID %d, Ordner %s). Schließe dort das ExecAI-Panel oder starte die Aufgaben aus diesem Fenster.",
	"inbox.lock.heldByServe":  "Aufgaben von der Website nimmt bereits der Hintergrundprozess execai serve an (PID %d).",
	"inbox.lock.signalFailed": "Signal an Prozess %d fehlgeschlagen: %v",
	"inbox.lock.stopTimeout":  "execai serve (PID %d) wurde nicht innerhalb von %s beendet — vielleicht beendet er gerade eine Aufgabe. Erneut versuchen oder: execai serve --stop --force",

	"inbox.plan.refused":   "Aufgaben von der Website gibt es in den Tarifen „Work“ und „Max“: %s",
	"inbox.plan.serveWarn": "⚠ Dein Tarif enthält keine Aufgaben von der Website — jede Aufgabe wird abgelehnt:",

	"inbox.result.interrupted": "(abgebrochen: %s)",
	"inbox.result.noText":      "(der Agent ist fertig, hat aber keine Textantwort gegeben)",

	"inbox.task.bindingsFailed": "Projektzuordnungen nicht abrufbar: %v",
	"inbox.task.dirMissing":     "Projektverzeichnis nicht gefunden: %s",
	"inbox.task.noDir":          "für dieses Projekt ist kein Verzeichnis gesetzt — hier zuordnen: execai → /project bind",
	"inbox.task.notBound":       "dieser Rechner ist dem Projekt der Aufgabe nicht zugeordnet — ausführen: execai → /project bind",
	"inbox.task.empty":          "leere Aufgabe",
	"inbox.task.stopped":        "im Editor gestoppt",
	"inbox.task.timeout":        "die Aufgabe wurde nicht innerhalb von %s fertig",
	"inbox.task.editorClosed":   "der Editor wurde geschlossen, bevor die Aufgabe fertig war",

	"inbox.serve.refused":     "✗ Aufgabe %s abgelehnt: der Tarif enthält keine Aufgaben von der Website",
	"inbox.serve.statusIDE":   "nicht gestartet: Aufgaben von der Website nimmt ein Editorfenster an (PID %d, Ordner %s)",
	"inbox.serve.stopIDE":     "Aufgaben von der Website nimmt ein Editorfenster an (PID %d, Ordner %s), nicht execai serve — schließe stattdessen dieses Fenster",
	"inbox.ide.otherProject":  "Eine Aufgabe für ein anderes Projekt (%s) wartet — öffne es im Editor oder starte execai serve",
	"inbox.ide.anotherWindow": "Aufgaben von der Website nimmt ein anderes Editorfenster an: %s",
	"inbox.ide.taken":         "Aufgaben von der Website nimmt jetzt dieses Fenster an",
	"inbox.ide.bindWhich":     "welches Projekt? Gib seine ID an (list_workspaces zeigt sie)",
}

func init() { i18n.Register("de", deInboxMessages) }
