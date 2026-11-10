// Package messages/es_sync — Spanish strings for sync between computers
// (/sync, execai sync). Merged into the "es" catalog via i18n.Register.
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var esSyncMessages = map[string]string{
	"hint.sync":        "sincronizar memoria y ajustes entre tus ordenadores",
	"sync.usage":       "/sync — estado · /sync on · off · now · password · forget · keys export · keys import",
	"sync.placeholder": "respuesta — Enter, cancelar — Esc",
	"sync.working":     "Un momento…",
	"sync.cancelled":   "Cancelado.",
	"sync.error":       "Sincronización: error: %v",
	"sync.failed":      "No se pudo contactar con el servidor de sincronización: %v",

	// === gate ===
	"sync.gate.noPlan": "La sincronización está en los planes «Work» y «Max»: %s",
	"sync.gate.login":  "Para sincronizar, inicia sesión en ExecAI: ejecuta execai y confirma en el navegador (en el chat: /login).",

	// === status ===
	"sync.status.off":         "La sincronización está apagada. Activarla: %[1]s on — la memoria, los ajustes y las claves de los proveedores serán iguales en todos tus ordenadores (planes «Work» y «Max»).",
	"sync.status.on":          "Sincronización activada · último intercambio: %s",
	"sync.status.onNever":     "Sincronización activada · aún sin intercambio",
	"sync.status.what":        "Se sincroniza: %s — todo cifrado en este ordenador.",
	"sync.status.pausedPlan":  "En pausa: la sincronización está en los planes «Work» y «Max»: %s",
	"sync.status.pausedLogin": "En pausa: inicia sesión en ExecAI (en el chat: /login).",
	"sync.status.pausedKey": "En pausa: la clave de sincronización cambió en otro ordenador. " +
		"Introduce de nuevo la contraseña de sincronización: %s on",
	"sync.status.unreadable": "Archivos del servidor que no se abren aquí: %d — están cifrados con otra clave. " +
		"Introduce la contraseña de sincronización: %s on",
	"sync.status.tooBig":    "Demasiado grandes, se quedan solo aquí: %s",
	"sync.status.quota":     "El espacio de sincronización en el servidor se agotó — los archivos nuevos no se envían.",
	"sync.status.lastError": "Último error: %s",
	"sync.status.commands":  "Comandos: %[1]s now · %[1]s off · %[1]s password · %[1]s forget · %[1]s keys export | import",

	"sync.what.memory":   "memoria (archivos: %d)",
	"sync.what.projects": "proyectos (%d)",

	// === on ===
	"sync.on.newIntro": "Este es tu primer ordenador con sincronización. La memoria, los ajustes y las claves de los proveedores se cifran aquí, en el ordenador: el servidor guarda solo la copia cifrada y no vemos qué hay dentro.",
	"sync.on.newPassword": "Inventa una contraseña de sincronización (al menos %d caracteres). La introducirás " +
		"en tus otros ordenadores. No se puede recuperar.",
	"sync.on.repeatPassword": "Repite la contraseña:",
	"sync.on.joinIntro":      "La sincronización ya está activada en otro de tus ordenadores.",
	"sync.on.enterPassword":  "Introduce la contraseña de sincronización de tu primer ordenador:",
	"sync.on.keyDiffers": "Este ordenador ya tiene su propia clave de cifrado y no coincide con la del servidor. " +
		"La memoria cifrada con una clave no se lee con la otra.",
	"sync.on.chooseKey": "1 — usar la clave del servidor (la tuya se guarda al lado)\n" +
		"2 — mantener la tuya y reemplazar la del servidor: todo en el servidor se sobrescribe desde aquí, " +
		"tus otros ordenadores pedirán la contraseña otra vez",
	"sync.on.oldKeySaved": "La clave anterior se guardó: %s",
	"sync.on.raced":       "Mientras tanto, la clave del servidor cambió desde otro ordenador. Inténtalo de nuevo: %s on",
	"sync.on.racedJoin":   "Se acaba de fijar una contraseña de sincronización en otro ordenador — introdúcela.",
	"sync.on.created":     "Sincronización activada.",
	"sync.on.joined":      "Clave recibida, sincronización activada.",
	"sync.on.replaced":    "Sincronización activada; el servidor guarda ahora los datos de este ordenador.",
	"sync.on.done":        "Sincronización activada.",
	"sync.on.firstFailed": "El primer intercambio falló: %v. Lo reintentaré en 10 minutos, o ahora: %s now",

	// === password ===
	"sync.password.wrong":    "La contraseña no es correcta.",
	"sync.password.short":    "Demasiado corta: al menos %d caracteres.",
	"sync.password.mismatch": "Las contraseñas no coinciden, otra vez.",
	"sync.password.new":      "Nueva contraseña de sincronización (al menos %d caracteres, no se puede recuperar):",
	"sync.password.changed": "Contraseña cambiada. En tus otros ordenadores no hay que hacer nada — " +
		"la contraseña solo hace falta para añadir uno nuevo.",

	// === now / off ===
	"sync.now.done":    "Hecho: %d recibidos, %d enviados.",
	"sync.now.off":     "La sincronización está apagada. Activarla: %s on",
	"sync.off.already": "La sincronización ya está apagada. Activarla: %s on",
	"sync.off.done": "Sincronización apagada en este ordenador. Los datos siguen en el servidor; " +
		"para borrarlos: %[1]s forget. Volver a activarla: %[1]s on",

	// === forget ===
	"sync.forget.warn":      "Se borrarán del servidor la clave de cifrado, toda la memoria sincronizada, los ajustes y las claves de los proveedores, y la lista de ordenadores. En tus ordenadores todo se queda. No se puede deshacer.",
	"sync.forget.confirm":   "Para borrar, escribe la palabra «%s»:",
	"sync.forget.word":      "borrar",
	"sync.forget.cancelled": "No se borra nada.",
	"sync.forget.done": "Borrado del servidor, archivos: %d. En tus ordenadores todo sigue ahí; " +
		"los demás apagarán la sincronización en su próxima comprobación. Volver a activarla: %s on",

	// === background ===
	"sync.bg.paused":  "Sincronización en pausa — la memoria y los ajustes de este ordenador no se tocan.",
	"sync.bg.resumed": "La sincronización vuelve a funcionar.",
	"sync.bg.wiped": "Los datos de sincronización se borraron del servidor (desde otro ordenador) — aquí la sincronización está apagada. " +
		"Volver a activarla: %s on",
	"sync.bg.keyChanged": "Sincronización en pausa: la clave cambió en otro ordenador. " +
		"Introduce de nuevo la contraseña de sincronización: %s on",

	// === keys ===
	"sync.keys.nothing":  "Nada que llevar: no hay ninguna clave de proveedor conectada en este ordenador.",
	"sync.keys.exported": "Claves de proveedores: %s.\nTraspaso sin servidor y sin plan. Aquí está el código de traspaso — pégalo en el otro ordenador (%[3]s keys import):\n\n%[2]s",
	"sync.keys.phrase": "La frase del código — envíala por separado y por otra vía:\n\n    %s\n\n" +
		"Juntos, el código y la frase abren tus claves: no se los muestres a nadie.",
	"sync.keys.importIntro": "Traer claves de proveedores desde otro ordenador (allí: keys export).",
	"sync.keys.enterCode":   "Pega el código de traspaso:",
	"sync.keys.enterPhrase": "Introduce la frase de %d palabras:",
	"sync.keys.badCode":     "Eso no parece un código de traspaso — cópialo entero.",
	"sync.keys.badPhrase":   "La frase no es correcta.",
	"sync.keys.added":       "Añadidas: %s",
	"sync.keys.updated":     "Actualizadas (la clave de allí es más nueva): %s",
	"sync.keys.kept":        "Sin cambios: %s",
	"sync.keys.active":      "La fuente activa no cambió: %s. Para cambiar: /source <nombre>",

	// === execai sync ===
	"sync.cmd.short":            "Sincronizar memoria y ajustes entre tus ordenadores",
	"sync.cmd.long":             "La memoria, los ajustes, los permisos y las claves de los proveedores del agente son iguales en todos tus ordenadores. Todo se cifra en el ordenador con la contraseña de sincronización; el servidor guarda solo la copia cifrada y no vemos qué hay dentro.\nSin plan, las claves de los proveedores se pueden llevar directamente: execai sync keys export / import.\n\nLa sincronización está en los planes «Work» y «Max».",
	"sync.cmd.status":           "Estado de la sincronización (por defecto)",
	"sync.cmd.on":               "Activar la sincronización en este ordenador",
	"sync.cmd.off":              "Apagarla en este ordenador (los datos siguen en el servidor)",
	"sync.cmd.now":              "Sincronizar ahora",
	"sync.cmd.password":         "Cambiar la contraseña de sincronización",
	"sync.cmd.forget":           "Borrar todo del servidor",
	"sync.cmd.keys":             "Llevar claves de proveedores entre ordenadores sin servidor",
	"sync.cmd.keysExport":       "Mostrar el código de traspaso y su frase",
	"sync.cmd.keysImport":       "Recibir claves de otro ordenador con código y frase",
	"sync.cmd.needTerminal":     "Hace falta un terminal: las respuestas se escriben a mano.",
	"sync.what.config":          "ajustes y claves de los proveedores",
	"sync.status.keysFallback":  "Sin sincronización, las claves de los proveedores se pueden llevar directamente — sin servidor y sin plan: %s keys export",
	"sync.pulledSource":         "La fuente activa se cambió en otro ordenador: %s",
	"sync.status.projects":      "Proyectos: %d (%s)",
	"sync.project.off":          "Los archivos del proyecto (.execai/) se sincronizan con la sincronización activada: %s on",
	"sync.project.pulled":       "Archivos del proyecto (.execai/) recibidos del servidor.",
	"sync.project.pushed":       "Archivos del proyecto (.execai/) enviados al servidor — cifrados.",
	"sync.project.same":         "Los archivos del proyecto (.execai/) coinciden con el servidor.",
	"sync.project.empty":        "El proyecto aún no tiene archivos .execai/ — se sincronizarán cuando aparezcan.",
	"sync.project.backup":       "El proyecto cambió aquí y en otro ordenador — gana la copia más reciente, la otra se guarda al lado: %s",
	"project.published":         "Proyecto «%s» creado en el chat web.",
	"project.publishedNotBound": "Proyecto «%s» creado en el chat web, pero no se pudo vincular este directorio: %v",
	"hint.projectPublish":       "crear un proyecto del chat web a partir de este directorio",
	"project.cmd.short":         "Proyectos: un directorio del ordenador ↔ un proyecto del chat web",
	"project.cmd.publish":       "Crear un proyecto del chat web con el directorio actual, vincularlo y enviar .execai/",
}

func init() { i18n.Register("es", esSyncMessages) }
