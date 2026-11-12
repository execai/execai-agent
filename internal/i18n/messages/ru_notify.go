// Package messages/ru_notify — Russian strings of the terminal notifications
// (/notifications, E105).
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var ruNotifyMessages = map[string]string{
	"hint.notifications":     "сообщать, когда ход закончен или агент ждёт: system / bell / off / test",
	"hint.notificationsTest": "отправить пробное уведомление сейчас",

	"notify.turnDone":   "ExecAI: ход завершён — %s",
	"notify.approve":    "ExecAI ждёт подтверждения: %s",
	"notify.ask":        "ExecAI задаёт вопрос",
	"notify.approveAny": "ExecAI ждёт подтверждения",
	"notify.test":       "ExecAI: проверка уведомлений",

	// Background work (/loop, wake-ups): its end, an error, the step limit.
	"notify.bgDone":    "ExecAI: фоновая работа закончена — %s",
	"notify.bgStopped": "ExecAI: фоновая работа остановлена — %s",
	"notify.bgError":   "ExecAI: фоновый ход закончился ошибкой — %s",
	"notify.bgLimit":   "ExecAI: фоновый ход упёрся в предел шагов — %s",

	"notify.title":         "Уведомления — когда ход закончен или агент ждёт тебя, а ты в другом окне:",
	"notify.desc.system":   "уведомление на рабочем столе (собственное у терминала, где оно есть) и звонок",
	"notify.desc.bell":     "только через терминал: его уведомление и звонок — ничего не запускает, работает по SSH",
	"notify.desc.off":      "без уведомлений",
	"notify.here":          "На этом компьютере: %s",
	"notify.bell":          "звонок",
	"notify.focus.known":   "Терминал сообщает о фокусе: уведомления приходят, только пока его окно в фоне.",
	"notify.focus.unknown": "Терминал не сообщал о фокусе: ход от 30 с после твоего последнего нажатия или вопрос без ответа 15 с всё равно придут уведомлением.",
	"notify.howto":         "Сменить: /notifications system | bell | off · проверить: /notifications test\nНастройка остаётся на этом компьютере — синк её не переносит.",
	"notify.set":           "Уведомления: %s — %s",
	"notify.unknown":       "Неизвестный режим «%s» — system, bell или off.",
	"notify.offNow":        "Уведомления выключены — /notifications system включит их.",
	"notify.testSent":      "Пробное уведомление отправлено: %s",
}

func init() { i18n.Register("ru", ruNotifyMessages) }
