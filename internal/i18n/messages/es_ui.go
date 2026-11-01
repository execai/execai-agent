// Package messages/es_ui — Spanish strings for execai ui (the agent panel in
// a browser). Merged into the "es" catalog via i18n.Register.
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var esUIMessages = map[string]string{
	"ui.cmd.short": "Abrir el panel del agente en el navegador (solo en este ordenador)",
	"ui.cmd.long": "El mismo panel de chat que en VS Code y ExecAI Studio, en una pestaña del navegador en 127.0.0.1.\n\n" +
		"La dirección lleva una clave de un solo uso: la primera visita la cambia por una cookie y después el enlace ya no abre nada. " +
		"Solo este ordenador puede conectarse. Las tareas del sitio y la comprobación del plan funcionan como en el panel del editor. Detener: Ctrl+C.",
	"ui.flag.cwd":    "directorio del proyecto (por defecto, el actual)",
	"ui.flag.port":   "puerto en 127.0.0.1 (por defecto, uno libre)",
	"ui.flag.noOpen": "no abrir el navegador, solo mostrar la dirección",
	"ui.badCwd":      "no es un directorio: %s",
	"ui.address":     "Panel de ExecAI: %s",
	"ui.hint":        "El enlace funciona una vez: el navegador que lo abre conserva el acceso mientras execai ui siga en marcha. Detener: Ctrl+C.",
	"ui.openFailed":  "No se pudo abrir el navegador (%v): abre el enlace tú mismo.",
	"ui.stopped":     "El panel se ha detenido.",
	"ui.http.linkUsed": "Este enlace ya se ha usado. Abre el panel en el navegador donde lo abriste la primera vez " +
		"o reinicia execai ui para obtener un enlace nuevo.",
	"ui.http.noSession": "Abre el enlace que execai ui mostró en la terminal.",
}

func init() { i18n.Register("es", esUIMessages) }
