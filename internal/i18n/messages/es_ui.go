// Package messages/es_ui — Spanish strings for execai ui (the agent panel in
// a browser). Merged into the "es" catalog via i18n.Register.
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var esUIMessages = map[string]string{
	"ui.cmd.short": "Abrir el panel del agente en una ventana propia o en el navegador (solo en este ordenador)",
	"ui.cmd.long": "El mismo panel de chat que en VS Code y ExecAI Studio, en 127.0.0.1.\n\n" +
		"Se abre como una ventana propia — sin pestañas ni barra de direcciones — cuando está instalado Google Chrome, Chromium, Microsoft Edge, " +
		"Yandex Browser, Brave o Vivaldi; si no, como una pestaña del navegador predeterminado. Cerrar la ventana detiene el panel, y Ctrl+C también. " +
		"Un segundo arranque en la misma carpeta muestra la ventana que ya está abierta.\n\n" +
		"La dirección lleva una clave de un solo uso: la primera visita la cambia por una cookie y después el enlace ya no abre nada. " +
		"Solo este ordenador puede conectarse. Las tareas del sitio y la comprobación del plan funcionan como en el panel del editor.",
	"ui.flag.cwd":             "directorio del proyecto (por defecto, el actual)",
	"ui.flag.port":            "puerto en 127.0.0.1 (por defecto, el propio de la carpeta, uno de 23750–23769; si está ocupado, el siguiente)",
	"ui.flag.noOpen":          "no abrir el navegador, solo mostrar la dirección",
	"ui.flag.window":          "solo una ventana propia: sin un navegador adecuado, un error",
	"ui.flag.browser":         "abrir una pestaña del navegador predeterminado, sin ventana propia",
	"ui.flag.last":            "la carpeta del último arranque (así arranca el acceso directo)",
	"ui.flag.code":            "mostrar el código de un solo uso con el que una ventana entra en el panel ya iniciado de esta carpeta",
	"ui.flag.installShortcut": "crear un acceso directo de inicio (con --cwd: para esa carpeta)",
	"ui.flag.removeShortcut":  "quitar el acceso directo de inicio",
	"ui.flags.exclusive":      "--window, --browser y --no-open no van juntos: elige uno",
	"ui.flags.oneAction":      "--install-shortcut, --remove-shortcut, --code y --stop no van juntos: elige uno",
	"ui.badCwd":               "no es un directorio: %s",
	"ui.address":              "Panel de ExecAI: %s",
	"ui.hint":                 "El enlace funciona una vez: el navegador que lo abre conserva el acceso mientras execai ui siga en marcha. Detener: Ctrl+C.",
	"ui.openFailed":           "No se pudo abrir el navegador (%v): abre el enlace tú mismo.",
	"ui.stopped":              "El panel se ha detenido.",
	"ui.window.opened":        "Abro el panel de ExecAI en una ventana propia (%s). Su dirección: %s",
	"ui.window.hint":          "Cierra la ventana o pulsa Ctrl+C para detener el panel.",
	"ui.window.tabFallback":   "Una ventana propia necesita uno de estos navegadores: %s; aquí no hay ninguno, abro una pestaña.",
	"ui.window.none":          "no hay navegador para una ventana propia; sirven: %s",
	"ui.window.noScreen":      "no hay sesión gráfica (DISPLAY y WAYLAND_DISPLAY están vacías): no hay dónde abrir una ventana",
	"ui.window.notHere":       "El navegador indicado en los ajustes (%s) no está en este ordenador: elijo uno yo.",
	"ui.window.startFailed":   "La ventana no se abrió en %s (%v): abro una pestaña.",
	"ui.window.silent":        "La ventana en %s no llegó a mostrar el panel. Puedes entrar con un código (execai ui --code en esta carpeta) o con el enlace:",
	"ui.window.failed":        "la ventana no se abrió en %s: %v",
	"ui.last.none":            "no se ha elegido carpeta de proyecto: ejecuta execai ui en la carpeta del proyecto o crea un acceso directo para ella — execai ui --install-shortcut --cwd CARPETA",
	"ui.last.noneTitle":       "ExecAI: no se ha elegido carpeta de proyecto",
	"ui.last.noneNotice":      "Ejecuta execai ui en la carpeta del proyecto: desde entonces el acceso directo la abrirá.",
	"ui.shortcut.noLast":      "El acceso directo abre la carpeta del último arranque. Aún no hay ninguna: ejecuta primero execai ui en la carpeta del proyecto o crea un acceso directo para una carpeta con --cwd CARPETA.",
	"ui.window.closed":        "La ventana está cerrada: detengo el panel.",
	"ui.alarm.usedTwice":      "Con el enlace de un solo uso del panel llegaron dos: alguien más lo tiene. Se han anulado todas las entradas, se han detenido los turnos en curso y se han rechazado las solicitudes de permiso pendientes: la otra parte pudo enviar un mensaje al agente; revisa qué hicieron los chats por último.",
	"ui.alarm.noPage":         "Se usó el enlace de un solo uso del panel, pero después no se conectó ninguna página del panel. Se han anulado todas las entradas, se han detenido los turnos en curso y se han rechazado las solicitudes de permiso pendientes: quien entró pudo enviar un mensaje al agente; revisa qué hicieron los chats por último.",
	"ui.alarm.reopen":         "Abro la ventana de nuevo, con una clave nueva.",
	"ui.alarm.useCode":        "Para entrar de nuevo, ejecuta execai ui --code en esta carpeta e introduce el código en la página del panel.",
	"ui.already.window":       "El panel de esta carpeta ya está abierto: muestro su ventana (%s).",
	"ui.already.tab":          "El panel de esta carpeta ya está abierto: %s",
	"ui.already.starting":     "El panel de esta carpeta ya está arrancando.",
	"ui.code.line":            "Código de un solo uso para la página %s: %s (válido %d min)",
	"ui.code.notRunning":      "execai ui no está en marcha para esta carpeta",
	"ui.flag.stop":            "detener el panel que está en marcha para esta carpeta",
	"ui.flag.all":             "junto con --stop: detener los paneles de todas las carpetas",
	"ui.flags.allNeedsStop":   "--all solo se usa junto con --stop",
	"ui.stop.done":            "El panel se ha detenido: %s",
	"ui.stop.notRunning":      "No hay ningún panel en marcha para esta carpeta.",
	"ui.stop.noneRunning":     "No hay ningún panel en marcha.",
	"ui.stop.failed":          "el panel no se detuvo: sigue ocupando la carpeta %s",
	"ui.stop.asked":           "Detenido a petición de execai ui --stop.",
	"ui.noScreen":             "No hay sesión gráfica (DISPLAY y WAYLAND_DISPLAY están vacías): no inicio ningún navegador, abre el enlace tú mismo.",
	"ui.shortcut.notOurs":     "lo que hay en %s no es un acceso directo creado por execai ui --install-shortcut: no lo toco",
	"ui.shortcut.noHome":      "no hay carpeta personal: no hay dónde guardar el acceso directo",
	"ui.shortcut.unsupported": "en este sistema no hay acceso directo de inicio",
	"ui.shortcut.comment":     "El panel del agente ExecAI",
	"ui.shortcut.installed":   "Acceso directo de inicio creado: %s",
	"ui.shortcut.fallback":    "No se pudo crear el acceso directo; en su lugar, un archivo de inicio: %s",
	"ui.shortcut.removed":     "Acceso directo de inicio quitado: %s",
	"ui.shortcut.absent":      "No hay acceso directo de inicio: %s",
	"ui.shortcut.tabOnly":     "Una ventana propia necesita uno de estos navegadores: %s. Sin uno de ellos el acceso directo abre una pestaña, y el panel sigue en marcha hasta que se detenga: execai ui --stop --all.",
	"ui.http.linkUsed": "Este enlace ya se ha usado. Abre el panel en el navegador donde lo abriste la primera vez " +
		"o reinicia execai ui para obtener un enlace nuevo.",
	"ui.http.noSession":    "Abre el enlace que execai ui mostró en la terminal.",
	"ui.http.login.code":   "Ejecuta execai ui --code en una terminal, en la carpeta del proyecto, e introduce aquí el código que muestre.",
	"ui.http.login.link":   "O abre el enlace que execai ui mostró en la terminal.",
	"ui.http.login.label":  "Código de un solo uso",
	"ui.http.login.button": "Entrar",
	"ui.http.login.wrong":  "El código no coincide. Si ya se ha usado o tiene más de cinco minutos, ejecuta execai ui --code otra vez.",
	"ui.http.login.down":   "El panel no responde: comprueba que execai ui está en marcha.",
}

func init() { i18n.Register("es", esUIMessages) }
