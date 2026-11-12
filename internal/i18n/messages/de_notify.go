// Package messages/de_notify — German strings of the terminal notifications
// (/notifications, E105).
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var deNotifyMessages = map[string]string{
	"hint.notifications":     "melden, wenn ein Zug endet oder der Agent wartet: system / bell / off / test",
	"hint.notificationsTest": "jetzt eine Testbenachrichtigung senden",

	"notify.turnDone":   "ExecAI: Zug beendet — %s",
	"notify.approve":    "ExecAI wartet auf Bestätigung: %s",
	"notify.ask":        "ExecAI hat eine Frage",
	"notify.approveAny": "ExecAI wartet auf Bestätigung",
	"notify.test":       "ExecAI: Testbenachrichtigung",

	// Background work (/loop, wake-ups): its end, an error, the step limit.
	"notify.bgDone":    "ExecAI: Hintergrundarbeit beendet — %s",
	"notify.bgStopped": "ExecAI: Hintergrundarbeit gestoppt — %s",
	"notify.bgError":   "ExecAI: Hintergrund-Zug mit Fehler beendet — %s",
	"notify.bgLimit":   "ExecAI: Hintergrund-Zug hat das Schrittlimit erreicht — %s",

	"notify.title":         "Benachrichtigungen — wenn ein Zug endet oder der Agent auf dich wartet, während du in einem anderen Fenster bist:",
	"notify.desc.system":   "eine Desktop-Benachrichtigung (die eigene des Terminals, wo es eine hat) und die Glocke",
	"notify.desc.bell":     "nur über das Terminal: seine Benachrichtigung und die Glocke — startet nichts, funktioniert über SSH",
	"notify.desc.off":      "keine Benachrichtigungen",
	"notify.here":          "Auf diesem Computer: %s",
	"notify.bell":          "Glocke",
	"notify.focus.known":   "Das Terminal meldet den Fokus: Benachrichtigungen kommen nur, solange sein Fenster im Hintergrund ist.",
	"notify.focus.unknown": "Das Terminal hat keinen Fokus gemeldet: ein Zug ab 30 s seit deinem letzten Tastendruck oder eine Frage, die 15 s offen bleibt, wird trotzdem gemeldet.",
	"notify.howto":         "Ändern: /notifications system | bell | off · ausprobieren: /notifications test\nDie Einstellung bleibt auf diesem Computer — der Sync überträgt sie nicht.",
	"notify.set":           "Benachrichtigungen: %s — %s",
	"notify.unknown":       "Unbekannter Modus „%s“ — system, bell oder off.",
	"notify.offNow":        "Benachrichtigungen sind aus — /notifications system schaltet sie ein.",
	"notify.testSent":      "Testbenachrichtigung gesendet: %s",
}

func init() { i18n.Register("de", deNotifyMessages) }
