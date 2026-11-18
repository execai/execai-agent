// Package messages/en_notify — English strings of the terminal notifications
// (/notifications, E105).
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var enNotifyMessages = map[string]string{
	"hint.notifications":     "notify when a turn ends or the agent waits: system / bell / off / test",
	"hint.notificationsTest": "send a test notification now",

	// The notifications themselves: a folder name or a tool name at most.
	"notify.turnDone":   "ExecAI: turn finished — %s",
	"notify.approve":    "ExecAI is waiting for approval: %s",
	"notify.ask":        "ExecAI has a question",
	"notify.approveAny": "ExecAI is waiting for approval",
	"notify.test":       "ExecAI: test notification",

	// Background work (/loop, wake-ups): its end, an error, the step limit.
	"notify.bgDone":    "ExecAI: background work finished — %s",
	"notify.bgStopped": "ExecAI: background work stopped — %s",
	"notify.bgError":   "ExecAI: background turn ended with an error — %s",
	"notify.bgLimit":   "ExecAI: background turn hit the step limit — %s",

	// /notifications
	"notify.title":         "Notifications — when a turn finishes or the agent waits for you while you are in another window:",
	"notify.desc.system":   "a desktop notification (the terminal's own where it has one) and the bell",
	"notify.desc.bell":     "only through the terminal: its notification and the bell — starts nothing, works over SSH",
	"notify.desc.off":      "no notifications",
	"notify.here":          "On this computer: %s",
	"notify.bell":          "bell",
	"notify.focus.known":   "The terminal reports focus: you are notified only while its window is in the background.",
	"notify.focus.unknown": "The terminal has not reported focus: a turn of 30 s or more since your last keystroke, or a question left for 15 s, is announced anyway.",
	"notify.howto":         "Change: /notifications system | bell | off · try it: /notifications test\nThe setting stays on this computer — sync does not carry it.",
	"notify.set":           "Notifications: %s — %s",
	"notify.unknown":       "Unknown mode \"%s\" — system, bell or off.",
	"notify.offNow":        "Notifications are off — /notifications system turns them on.",
	"notify.testSent":      "Test notification sent: %s",
}

func init() { i18n.Register("en", enNotifyMessages) }
