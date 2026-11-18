// Package messages/es_notify — Spanish strings of the terminal notifications
// (/notifications, E105).
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var esNotifyMessages = map[string]string{
	"hint.notifications":     "avisar cuando termina un turno o el agente espera: system / bell / off / test",
	"hint.notificationsTest": "enviar ahora una notificación de prueba",

	"notify.turnDone":   "ExecAI: turno terminado — %s",
	"notify.approve":    "ExecAI espera confirmación: %s",
	"notify.ask":        "ExecAI tiene una pregunta",
	"notify.approveAny": "ExecAI espera confirmación",
	"notify.test":       "ExecAI: notificación de prueba",

	// Background work (/loop, wake-ups): its end, an error, the step limit.
	"notify.bgDone":    "ExecAI: trabajo en segundo plano terminado — %s",
	"notify.bgStopped": "ExecAI: trabajo en segundo plano detenido — %s",
	"notify.bgError":   "ExecAI: turno en segundo plano terminado con un error — %s",
	"notify.bgLimit":   "ExecAI: turno en segundo plano alcanzó el límite de pasos — %s",

	"notify.title":         "Notificaciones — cuando termina un turno o el agente te espera mientras estás en otra ventana:",
	"notify.desc.system":   "una notificación de escritorio (la propia del terminal, donde la tiene) y la campana",
	"notify.desc.bell":     "solo a través del terminal: su notificación y la campana — no inicia nada, funciona por SSH",
	"notify.desc.off":      "sin notificaciones",
	"notify.here":          "En este ordenador: %s",
	"notify.bell":          "campana",
	"notify.focus.known":   "El terminal informa del foco: solo se avisa mientras su ventana está en segundo plano.",
	"notify.focus.unknown": "El terminal no ha informado del foco: un turno de 30 s o más desde tu última tecla, o una pregunta sin respuesta durante 15 s, se avisa igualmente.",
	"notify.howto":         "Cambiar: /notifications system | bell | off · probar: /notifications test\nEl ajuste se queda en este ordenador — la sincronización no lo lleva.",
	"notify.set":           "Notificaciones: %s — %s",
	"notify.unknown":       "Modo desconocido «%s» — system, bell u off.",
	"notify.offNow":        "Las notificaciones están desactivadas — /notifications system las activa.",
	"notify.testSent":      "Notificación de prueba enviada: %s",
}

func init() { i18n.Register("es", esNotifyMessages) }
