// Package messages/es_machine — textos en español del tutorial (/start), la
// tarjeta de la máquina (/machine) y el enlace para el móvil (/phone).
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var esMachineMessages = map[string]string{
	"hint.start":        "primeros pasos: cinco pasos y lo que ya está hecho",
	"hint.machine":      "mi máquina: modelos, sincronización, desde dónde se puede llamar",
	"hint.machineCheck": "comprobar que las fuentes de modelos responden",
	"hint.phone":        "código QR: llamar a esta máquina desde el móvil",

	// === /start ===
	"start.title":    "Primeros pasos",
	"start.progress": "%d de %d hechos",
	"start.allDone":  "Todo listo: esta máquina está configurada por completo.",
	"start.footer": "Mi máquina: /machine · todos los comandos: /help · esta lista otra vez: /start\n" +
		"Enter — enviar, Shift+Enter — nueva línea.",

	"start.login.title": "Iniciar sesión en ExecAI",
	"start.login.why":   "La cuenta une esta máquina con el chat web, tu móvil y tus otros ordenadores.",
	"start.login.cmd":   "Escribe: /login — y confirma en el navegador.",

	"start.models.title": "Conectar modelos",
	"start.models.why":   "Tus propias claves de proveedores o un plan de ExecAI — la memoria y las llamadas desde el móvil funcionan con ambos.",
	"start.models.cmd":   "Escribe: /connect <proveedor> — tu propia clave, /plan — el plan de ExecAI, /machine check — comprobar que la fuente responde.",

	"start.sync.title":    "Activar la sincronización",
	"start.sync.why":      "Los mismos ajustes y la misma memoria en todos tus ordenadores.",
	"start.sync.what":     "Viaja, cifrado en este ordenador con tu contraseña de sincronización: la configuración con las claves de los proveedores, tu memoria personal, los archivos de los proyectos vinculados.",
	"start.sync.notWhat":  "Se queda aquí: api_base (la dirección del servidor de esta máquina) y el inicio de sesión.",
	"start.sync.password": "Solo tú conoces la contraseña de sincronización: no la guardamos y no podemos recuperarla.",
	"start.sync.cmd":      "Escribe: /sync on (planes «Work» y «Max»).",

	"start.project.title": "Vincular una carpeta a un proyecto web",
	"start.project.why":   "La carpeta se convierte en un proyecto del chat web: las tareas de la web se ejecutan en ella, la memoria del proyecto es común.",
	"start.project.cmd":   "Escribe: /project publish — crear un proyecto con esta carpeta, /project bind <nombre> — vincularla a uno existente.",

	"start.phone.title": "Llamar al agente desde el móvil",
	"start.phone.why":   "Da tareas a esta máquina desde el chat web o el móvil — aunque funcione con tus propias claves.",
	"start.phone.cmd":   "Escribe: /phone — un código QR para el móvil. Para recibir tareas, ejecuta execai serve.",

	// === /machine ===
	"machine.title":          "Mi máquina",
	"machine.label.machine":  "Máquina",
	"machine.label.account":  "Cuenta",
	"machine.label.models":   "Modelos",
	"machine.label.sync":     "Sincronización",
	"machine.label.presence": "Accesible",
	"machine.identity":       "%s · %s · agente %s",
	"machine.account.in":     "%s · plan: %s",
	"machine.account.noPlan": "%s · sin plan de ExecAI",
	"machine.account.out":    "sin iniciar sesión — /login",

	"machine.source.alive":     "responde (comprobado %s)",
	"machine.source.down":      "no respondió (comprobado %s)",
	"machine.source.unchecked": "aún sin comprobar — /machine check",
	"machine.source.needLogin": "hace falta iniciar sesión — /login",
	"machine.source.noPlan":    "sin plan activo — /plan",
	"machine.source.active":    "en uso",

	"machine.sync.off":      "apagada — /sync on",
	"machine.sync.on":       "encendida · último envío: %s",
	"machine.sync.never":    "encendida · aún no se ha enviado nada",
	"machine.sync.paused":   "en pausa — detalles: /sync",
	"machine.sync.inside":   "contenido: %s",
	"machine.sync.config":   "configuración y claves de proveedores",
	"machine.sync.memory":   "memoria personal (archivos: %d)",
	"machine.sync.projects": "proyectos: %d",
	"machine.sync.empty":    "nada todavía",
	"machine.sync.mode":     "modo: %s",
	"machine.mode.all":      "todos los proyectos",
	"machine.mode.selected": "solo carpetas vinculadas",
	"machine.mode.none":     "sin proyectos",

	"machine.presence.serve":      "recibe tareas: sí — execai serve (pid %d)",
	"machine.presence.ide":        "recibe tareas: sí — una ventana del editor (pid %d)",
	"machine.presence.noListener": "recibe tareas: no — ejecuta execai serve",
	"machine.presence.reachable":  "llamar desde la web y el móvil: sí — /phone",
	"machine.presence.blocked":    "llamar desde la web y el móvil: todavía no — %s",
	"machine.block.login":         "inicia sesión (/login)",
	"machine.block.plan":          "hace falta el plan «Work» o «Max»",
	"machine.block.listener":      "ejecuta execai serve",

	"machine.footer":   "Comprobar las fuentes otra vez: /machine check · tutorial: /start",
	"machine.checking": "Comprobando las fuentes…",

	// === /phone ===
	"phone.title":     "Llamar a esta máquina desde el móvil",
	"phone.scan":      "Apunta la cámara del móvil al código o abre el enlace:",
	"phone.what":      "La página abre el chat web con esta máquina ya elegida para las tareas.",
	"phone.needLogin": "El enlace para el móvil aparece al iniciar sesión: /login",
	"phone.listener":  "Para recibir tareas, esta máquina necesita execai serve en marcha (en otra ventana).",
	"phone.plan":      "Las tareas desde la web y el móvil están en los planes «Work» y «Max»: %s",
	"phone.qrFailed":  "No se pudo dibujar el código QR: %v",
}

func init() { i18n.Register("es", esMachineMessages) }
