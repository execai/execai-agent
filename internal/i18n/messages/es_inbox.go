// Package messages/es_inbox — cadenas en español del canal «web → agente»:
// tareas del sitio que acepta execai serve o el panel del editor.
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var esInboxMessages = map[string]string{
	"inbox.err.parse": "no se pudo leer la respuesta del servidor",

	"inbox.lock.busy":         "el bloqueo de la bandeja cambia de dueño sin parar, inténtalo de nuevo",
	"inbox.lock.heldByIDE":    "Las tareas del sitio ya las acepta una ventana del editor (pid %d, carpeta %s). Cierra allí el panel de ExecAI o lanza las tareas desde esa ventana.",
	"inbox.lock.heldByServe":  "Las tareas del sitio ya las acepta el proceso en segundo plano execai serve (pid %d).",
	"inbox.lock.signalFailed": "no se pudo enviar una señal al proceso %d: %v",
	"inbox.lock.stopTimeout":  "execai serve (pid %d) no terminó en %s — quizá está acabando una tarea. Inténtalo de nuevo o ejecuta: execai serve --stop --force",

	"inbox.plan.refused":   "Lanzar tareas desde el sitio está incluido en los planes Work y Max: %s",
	"inbox.plan.serveWarn": "⚠ Tu plan no incluye lanzar tareas desde el sitio — cada tarea recibirá un rechazo:",

	"inbox.result.interrupted": "(interrumpido: %s)",
	"inbox.result.noText":      "(el agente terminó, pero no dio una respuesta de texto)",

	"inbox.task.bindingsFailed": "no se pudieron obtener las vinculaciones de proyectos: %v",
	"inbox.task.dirMissing":     "no se encontró el directorio del proyecto: %s",
	"inbox.task.noDir":          "este proyecto no tiene directorio — vincúlalo aquí: execai → /project bind",
	"inbox.task.notBound":       "esta máquina no está vinculada al proyecto de la tarea — ejecuta: execai → /project bind",
	"inbox.task.empty":          "tarea vacía",
	"inbox.task.stopped":        "detenido en el editor",
	"inbox.task.timeout":        "la tarea no terminó en %s",
	"inbox.task.editorClosed":   "el editor se cerró antes de que terminara la tarea",

	"inbox.serve.refused":     "✗ tarea %s rechazada: el plan no incluye tareas desde el sitio",
	"inbox.serve.statusIDE":   "no está en marcha: las tareas del sitio las acepta una ventana del editor (pid %d, carpeta %s)",
	"inbox.serve.stopIDE":     "las tareas del sitio las acepta una ventana del editor (pid %d, carpeta %s), no execai serve — cierra esa ventana",
	"inbox.ide.otherProject":  "Una tarea para otro proyecto (%s) está esperando — ábrelo en el editor o ejecuta execai serve",
	"inbox.ide.anotherWindow": "Las tareas del sitio las acepta otra ventana del editor: %s",
	"inbox.ide.taken":         "Ahora las tareas del sitio las acepta esta ventana",
	"inbox.ide.bindWhich":     "¿qué proyecto? Indica su id (list_workspaces los muestra)",
}

func init() { i18n.Register("es", esInboxMessages) }
