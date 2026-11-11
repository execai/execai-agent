// Package messages/es_ide — Spanish strings the agent sends to the editor
// panel (execai ide): the permission buttons and the list of sources to
// connect.
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var esIDEMessages = map[string]string{
	// The descriptions under the permission buttons; the buttons themselves
	// are approve.* (shared with the terminal chat).
	"ide.approve.onceDesc":    "permitir solo esta llamada",
	"ide.approve.sessionDesc": "hasta el final de este chat",
	"ide.approve.exactDesc":   "exactamente esta llamada hasta el final del chat",
	"ide.approve.foreverDesc": "recordar este permiso — se guarda en permissions.json",
	"ide.approve.denyDesc":    "no ejecutar; el agente recibe un rechazo",

	"ide.connect.withKey":    "%s (clave)",
	"ide.connect.noKey":      "%s (sin clave)",
	"ide.connect.zaiAPI":     "Plataforma Z.ai, pago por token (clave)",
	"ide.connect.openrouter": "OpenRouter — todos los proveedores con una clave (clave)",
	"ide.connect.ollama":     "Ollama — local o en la nube (clave para la nube)",
	"ide.connect.custom":     "Proveedor propio — cualquier dirección compatible con OpenAI o Anthropic",

	// Notices and errors the panel shows (execai ide), the security levels
	// and the errors of the sign-in and connect paths from other packages.
	"ide.err.effort":            "effort: off|low|medium|high|max o un número",
	"ide.err.positive":          "se necesita un número positivo",
	"ide.maxIter.set":           "iteraciones de herramientas por turno: %d",
	"ide.login.startFailed":     "no se pudo iniciar el inicio de sesión",
	"ide.login.saveFailed":      "no se pudo guardar el token",
	"ide.login.expired":         "el código de confirmación caducó — inténtalo de nuevo",
	"ide.login.timeout":         "el inicio de sesión no se confirmó a tiempo",
	"ide.connect.keyRejectedBy": "%s rechazó la clave (HTTP %d)",
	"ide.connect.noModels":      "conectado: %s (%s) — el servidor no devolvió la lista de modelos (HTTP %d); vuelve a conectar indicando los modelos",
	"ide.connect.ollamaKey":     "ollama.com no aceptó la clave (HTTP %d)",
	"ide.connect.ollamaDown":    "Ollama en %s no respondió (HTTP %d) — ¿está en marcha?",
	"ide.connect.unknown":       "fuente desconocida: %s",
	"ide.connect.needKey":       "se necesita la clave del proveedor",
	"ide.connect.keyRejected":   "clave rechazada (HTTP %d) — comprueba que es de %s",
	"ide.connect.done":          "conectado: %s",
	"ide.connect.models":        " — modelos disponibles: %d",
	"ide.connect.unverified":    " (clave guardada; se comprobará en la primera petición)",
	"ide.turn.queued":           "turno en cola: otros chats todavía están respondiendo",
	"ide.err.chdir":             "no se puede entrar en la carpeta %s",
	"ide.err.noModel":           "no se pudo elegir un modelo — revisa /model en execai",
	"ide.err.noModelShort":      "sin modelo",
	"ide.err.badMessage":        "no se puede leer el mensaje: %s",
	"ide.err.noChat":            "no existe ese chat: %s",
	"ide.subs.unreadable":       "no se pudo leer subscriptions.json (%s) — se usa la fuente base; las suscripciones no se pierden, pero hay que arreglar el archivo",
	"ide.err.noSuchModel":       "la fuente actual no tiene ese modelo: %s",
	"ide.err.unknownLevel":      "nivel desconocido: %s",
	"ide.security.set":          "nivel de seguridad: %s",
	"ide.logout.done":           "sesión de ExecAI cerrada (las suscripciones se mantienen)",
	"ide.disconnect.done":       "desconectado: %s",
	"ide.resume.none":           "nada que continuar — es el primer chat del proyecto",
	"ide.resume.failed":         "no se puede abrir el último chat: %s",
	"ide.chat.openFailed":       "no se puede abrir el chat: %s",
	"ide.wake.none":             "no había ningún despertar programado",
	"ide.err.unknownCommand":    "comando desconocido: %s",
	"ide.err.unknownType":       "tipo desconocido: %s",
	"ide.chat.busy":             "este chat todavía está respondiendo — espera a que termine el turno o detenlo",
	"security.level.light":      "ligero — no molestar",
	"security.level.deep":       "normal — el proyecto en silencio, fuera de él con una pregunta",
	"security.level.paranoid":   "estricto — preguntar todo",
	"connect.err.api":           "tipo de interfaz %q no admitido: openai (chat/completions) o anthropic (messages)",
	"connect.err.noURL":         "se necesita la dirección del servidor, p. ej. http://localhost:1234/v1",
	"connect.err.badURL":        "la dirección %q no parece una URL http(s)",
	"subs.err.nameLatin":        "el nombre necesita letras latinas o dígitos (de él sale el id para /source)",
	"subs.err.nameLong":         "el nombre tiene más de 40 caracteres",
	"subs.err.builtin":          "%q es una fuente integrada — elige otro nombre",
	"subs.err.notLoaded":        "las suscripciones no están cargadas",
	"subs.err.notConnected":     "la suscripción %q no está conectada — primero /connect %s",
	"auth.err.linkStart":        "agent-link/start devolvió %d: %s",
	"auth.err.linkPoll":         "agent-link/poll devolvió %d: %s",
	"auth.err.pollFailed":       "el sondeo falló 10 veces seguidas",
	"auth.err.linkExpired":      "el enlace caducó — ejecuta /login de nuevo",
}

func init() { i18n.Register("es", esIDEMessages) }
