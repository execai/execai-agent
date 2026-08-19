// Package messages/es_misc — cadenas en español del lote misc (tui.go,
// plain REPL, compact, agent loop, welcome).
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var esMiscMessages = map[string]string{
	// === Boot / login flow (tui.go) ===
	"ui.boot.noLogin": "ℹ Trabajando sin cuenta de ExecAI — fuente: %s · modelo: %s.\n" +
		"  /login — conectar una cuenta de ExecAI (nuestro catálogo de ~34 modelos + facturación).",
	"ui.login.intro": "¡Hola! Para iniciar sesión hay que confirmar el agente en el navegador (como gh auth login).\n" +
		"Si por alguna razón el device-flow no funciona — puedes pegar aquí un token JWT (eyJ…) y pulsar Enter.\n\n" +
		"ℹ La cuenta de ExecAI es OPCIONAL: puedes trabajar con tu propia suscripción sin iniciar sesión.\n" +
		"  /connect kimi <key>   → /source kimi     (Kimi Code)\n" +
		"  /connect zai <key>    → /source zai      (Z.ai GLM)\n" +
		"  /connect openai <key> → /source openai   (OpenAI API)\n" +
		"  También: anthropic, kimi-api, claude-cli, codex-cli, ollama — /connect lo muestra todo.",
	"ui.login.staleToken":          "El token antiguo no es válido en %s. Iniciando device-flow para un nuevo acceso…",
	"ui.login.startFlow":           "Iniciando device-flow para un nuevo acceso…",
	"ui.login.deviceFlowOpen":      "Abre en el navegador y confirma:\n\n  %s\n\nCódigo (por si lo introduces a mano): %s\n\nPulsa Enter / [y] para abrir el navegador automáticamente. [n] para abrirlo tú mismo.",
	"ui.boot.modelsFallbackFailed": "no se pudo obtener ni construir la lista de modelos de reserva",
	"ui.welcome":                   "execai %s · %s/%s · %s · %s\nEscribe una tarea. /model — modelos, /help — comandos, /quit — salir.",

	// === Stream errors / token expiry ===
	"ui.stream.tokenExpiredHint": "→ Cambia /source zai|ollama|anthropic, o /login para volver a confirmar.",
	"ui.stream.tokenExpiredFlow": "El token de ExecAI ha caducado — iniciando device-flow. Confirma en el navegador.",

	// === /compact ===
	"ui.compact.historyNote": "[Historial compactado anteriormente (%d mensajes): %s]",
	"ui.compact.done":        "📦 Historial compactado: %d mensajes → 1 resumen (~%d caracteres)",
	"ui.compact.working":     "compactando el historial…",
	"ui.compact.tooShort":    "el historial aún es corto — no hay nada que compactar (se necesitan >%d mensajes)",
	"ui.compact.truncated":   "…(recortado)",
	"ui.compact.promptSystem": "Eres un compresor de contexto para un agente de IA. Recibes la transcripción de una conversación. " +
		"Devuelve un resumen BREVE (≤500 palabras) que conserve:\n" +
		"  • decisiones clave y sus motivos\n" +
		"  • rutas de archivos y comandos importantes\n" +
		"  • resultados de tool-calls que puedan ser útiles después\n" +
		"  • errores y cómo se resolvieron\n" +
		"Omite la charla y las confirmaciones. Escribe en español, en estilo telegráfico.",
	"ui.compact.promptUser": "Comprime esta conversación:\n\n%s",

	// === Autoloop ===
	"ui.autoloop.defaultPrompt": "continúa",
	"ui.autoloop.wake":          "🌙 autoloop: despertar en %s (%s) → prompt: %q",

	// === /paste ===
	"ui.paste.empty":     "No hay pegados en esta sesión. Ctrl+V con un trozo grande de texto → marcador.",
	"ui.paste.header":    "Pegados (Ctrl+V ≥200 caracteres o con \\n):\n",
	"ui.paste.showHint":  "\nMostrar: /paste show <N>",
	"ui.paste.notNumber": "no es un número: %s",
	"ui.paste.notFound":  "no existe el pegado #%d",
	"ui.paste.usage":     "uso: /paste [list|show <N>]",

	// === /whoami ===
	"ui.whoami.notLoggedIn": "(sin sesión iniciada — /login)",

	// === /classic & /mouse ===
	"ui.classic.on":  "✓ classic TUI ON — reinicia execai (/quit → execai). Alt-screen + barra de estado fija, Shift+arrastrar para copiar.",
	"ui.classic.off": "✓ Ink-style (por defecto) — reinicia execai. Historial en el scrollback, selección y scroll nativos.",
	"ui.mouse.off":   "🖱  captura de ratón OFF — el ratón selecciona texto, el menú no responde a clics. Activar: /mouse on",
	"ui.mouse.on":    "🖱  captura de ratón ON — la rueda hace scroll, clic en el menú. Seleccionar texto: Shift+arrastrar. Desactivar: /mouse off",

	// === /effort ===
	"ui.effort.pickerHint": "selector de effort: ←/→ elegir, Enter confirmar, Esc cancelar",
	"ui.effort.current":    "Effort actual: %s (%d tokens)\nCambiar: /effort <off|low|medium|high|max>\n  off=0  low=1024  medium=4096  high=8192  max=32000\nFunciona con fuentes compatibles con Anthropic (Z.ai, Kimi, Anthropic, ollama-cloud, claude-cli).",
	"ui.effort.set":        "✓ effort=%s (%d tokens)",

	// === /max-iterations ===
	"ui.maxIter.current": "Max iterations actual: %d\nLímite de iteraciones tool-use por turno. Al agotarse — parada suave, el usuario puede decir 'continúa'.\nCambiar: /max-iterations <N>  (recomendado 20-200; por defecto 50)",
	"ui.maxIter.usage":   "/max-iterations <N>  donde N va de 1 a 500 (recomendado 20-200)",

	// === /loop ===
	"ui.loop.status":      "🔁 loop: cada %s — %q. /loop stop para detenerlo",
	"ui.loop.inactive":    "loop inactivo. Uso: /loop <intervalo> <prompt>  (ejemplo: /loop 5m comprueba el estado del build)",
	"ui.loop.notRunning":  "el loop ya está detenido",
	"ui.loop.stopped":     "🔁 loop detenido",
	"ui.loop.usage":       "/loop <intervalo> <prompt>  (por ejemplo: /loop 5m comprueba el estado del build)",
	"ui.loop.badInterval": "no se puede interpretar el intervalo %s — debe ser algo como 30s, 5m, 1h",
	"ui.loop.started":     "🔁 loop iniciado: cada %s — %q\nDetener: /loop stop",

	// === /log ===
	"ui.log.none":   "aún no hay log: %s",
	"ui.log.header": "📜 Últimas %d peticiones (%s):\n",

	// === /usage & /cd ===
	"ui.usage.fetchFailed": "no se pudo obtener usage: %s",
	"ui.cd.current":        "cwd actual: %s",

	// === Sessions ===
	"ui.session.new":         "nueva conversación",
	"ui.sessions.header":     "\nConversaciones guardadas (las más recientes arriba):\n",
	"ui.sessions.empty":      "(vacío)\n",
	"ui.sessions.switchHint": "\nCambiar: /resume <número|id>",
	"ui.resume.notFound":     "conversación %q no encontrada. /sessions — lista",
	"ui.resume.loadFailed":   "no se pudo cargar: %s",
	"ui.resume.resumed":      "continuamos: %s",
	"ui.title.renamed":       "renombrada: %s",

	// === /permissions ===
	"ui.perms.toolsEmpty": "  always allowed tools: (vacío)\n",
	"ui.perms.exactCount": "  always allowed exact commands: %d entradas\n",
	"ui.perms.resetHint":  "\nPara restablecer — borra el archivo a mano o ejecuta: rm ~/.config/execai/permissions.json",

	// === /model ===
	"ui.model.notFound": "modelo %q no encontrado",
	"ui.model.switched": "cambiado a %s/%s — %s (historial conservado)",

	// === Approve ===
	"ui.approve.denied":       "rechazado",
	"ui.approve.allowedTool":  "permitido: todas las llamadas a %s en esta sesión",
	"ui.approve.allowedExact": "permitido: este comando en esta sesión",
	"ui.approve.navHint":      "← → o Tab — cambiar, Enter — elegir, Esc — rechazar",

	// === Tool-call summaries ===
	"ui.toolSummary.write": "Write %s  (%d bytes)",

	// === Plain REPL (chat.go) ===
	"plain.err.fetchModels":    "no se pudo obtener la lista de modelos",
	"plain.err.emptyModels":    "el servidor devolvió una lista de modelos vacía",
	"plain.err.pickModelEmpty": "no se pudo elegir un modelo (¿lista vacía?)",
	"plain.err.pickModel":      "no se pudo elegir un modelo",
	"plain.commands":           "Comandos: /model — elegir modelo, /clear — limpiar historial, /quit — salir.",
	"plain.historyCleared":     "(historial limpiado)",
	"plain.modelSwitchHint":    "\nCambiar: /model <número> o /model <model_name>",
	"plain.modelNotFound":      "(modelo %q no encontrado. /model — ver la lista)",
	"plain.modelSwitched":      "(cambiado a %s/%s — %s; historial conservado)",
	"plain.errorPrefix":        "error:",
	"plain.modelsHeader":       "\nModelos disponibles (★ — primary, • — current):",

	// === Agent loop (internal/agent) ===
	"loop.iterationLimit": "⚠ Se alcanzó el límite de %d iteraciones — la tarea no está terminada. Di «continúa» para conceder %d más, o reformúlala.",

	// === Welcome screen (first launch) ===
	"welcome.text": `¡Hola! Esto es execai — un agente CLI para desarrollo.

Qué sabe hacer:
  • leer/escribir/editar archivos (Read, Write, Edit)
  • buscar (Grep, Glob, LS, Tree)
  • ejecutar comandos de shell (Bash) — read-only sin preguntar, el resto con confirmación
  • hacer peticiones HTTP (WebFetch — sin navegador; el navegador real llegará por separado)
  • llevar una lista de tareas (TodoWrite)

Memoria:
  • ./EXECAI.md           — memoria del proyecto (contexto del repo)
  • ~/.config/execai/EXECAI.md — tus ajustes personales
Ambos archivos se cargan en el system prompt automáticamente en cada sesión.

Comandos:
  /model               — lista de modelos
  /model <num|subcadena> — cambiar (el historial se conserva)
  /clear               — limpiar historial
  /help                — este mensaje
  /quit                — salir

Consejo: Enter — enviar, Shift+Enter — nueva línea.`,

	// === WebSearch / WebFetch tools ===
	"tool.websearch.noLogin": "La búsqueda web no está disponible: pasa por la pasarela de ExecAI y requiere una cuenta de ExecAI.\n" +
		"Sin sesión iniciada queda el navegador local: usa WebFetch para abrir cualquier URL y seguir los enlaces que devuelve.\n" +
		"Ejecuta /login para activar la búsqueda (y con ella el catálogo de modelos de ExecAI).",
	"tool.websearch.sources": "Fuentes:",
	"tool.websearch.empty":   "La búsqueda no devolvió nada. Reformula la consulta o abre una página concreta con WebFetch.",

	// === AskUser picker + субагенты ===
	"ask.title":             "El agente pregunta:",
	"ask.hint":              "↑↓ elegir · Enter confirmar · 1-4 directo · Esc — que decida el agente",
	"ask.answered":          "Pregunta: %s → %s",
	"ask.dismissed":         "a criterio del agente",
	"ask.dismissedForModel": "El usuario cerró la pregunta sin elegir. Decide tú, indica el supuesto que has adoptado y continúa.",
	"subagent.emptyResult":  "el subagente no devolvió nada",

	// === /key — ключ шифрования памяти ===
	"hint.key":  "clave de cifrado de memoria (ver / exportar / importar / crear)",
	"key.usage": "/key — estado · /key new — crear · /key export — mostrar la clave privada · /key import <clave> — instalar una clave de otra máquina",
	"key.absent": "Todavía no hay clave de cifrado.\n" +
		"Se creará automáticamente en la primera sincronización, o ahora con /key new.\n" +
		"Ubicación: %s",
	"key.present": "La clave de cifrado está en su sitio.\n" +
		"Clave pública (se puede compartir; con ella cifran PARA ti):\n" +
		"  %s\n" +
		"Archivo: %s",
	"key.created": "Clave de cifrado creada.\n" +
		"Clave pública: %s\n" +
		"Archivo: %s",
	"key.alreadyExists": "Ya existe una clave (pública: %s). NO se reemplazó: eso supondría perder el acceso a todo lo cifrado con ella.",
	"key.noRecoveryWarning": "⚠ No hay recuperación. No tenemos tu clave y nunca la tendremos.\n" +
		"Si la pierdes, la memoria sincronizada queda ilegible: solo se puede reconstruir desde cero.\n" +
		"\n" +
		"Guarda una copia ahora: /key export, y ponla en un gestor de contraseñas.\n" +
		"Para usar la misma memoria en otra máquina, ejecuta allí /key import <clave>.",
	"key.exportWarning": "⚠ Debajo está tu clave PRIVADA. Quien la tenga puede leer tu memoria sincronizada. No la pegues en chats, issues ni capturas.",
	"key.exportHint":    "Parte pública (esta sí se puede compartir): %s",
	"key.imported":      "Clave instalada. Pública: %s",
	"key.importFailed": "No se pudo instalar la clave: %v\n" +
		"\n" +
		"Si querías reemplazar una existente, bórrala primero, pero asegúrate de tener\n" +
		"una copia: de lo contrario, todo lo cifrado con la anterior será ilegible: %s",
	"key.invalid": "Esto no parece una clave privada (%v). Formato esperado: AGE-SECRET-KEY-1…",
	"key.error":   "La operación con la clave falló: %v",

	// === /memory — импорт и экспорт памяти ===
	"hint.memory":               "memoria del agente: importar de otros agentes, exportar",
	"hint.memoryFind":           "buscar en mi memoria y, si está vacía, en la de otros agentes",
	"memory.usage":              "/memory — estado · /memory import — traer memoria de otros agentes · /memory export [dir] — volcar la memoria en markdown · /memory find <consulta> — buscar en mi memoria y luego en la de otros agentes",
	"memory.findUsage":          "¿Buscar qué? Ejemplo: /memory find precios",
	"memory.findMine":           "🧠 Encontrado en mi propia memoria (%d) para «%s»:",
	"memory.findForeign":        "📁 Nada en mi memoria, pero otro agente en esta máquina lo tiene (%d) para «%s»:",
	"memory.findNothing":        "No se encontró nada, ni en mi memoria ni en la de otro agente: «%s»",
	"memory.findImportQuestion": "¿Incorporarlo a mi memoria? (%d archivo(s))",
	"memory.status":             "Memoria: %d entradas en %s",
	"memory.foundNearby":        "Archivos de memoria de otros agentes encontrados aquí: %d",
	"memory.importHint":         "\nPara traerlos: /memory import",
	"memory.nothingToImport":    "No se encontró memoria de otros agentes en %s.\nBuscado: CLAUDE.md, .claude/, AGENTS.md, .cursorrules, .cursor/rules/, .github/copilot-instructions.md, EXECAI.md",
	"memory.importQuestion":     "¿Importar %d archivo(s) a la memoria del agente?",
	"memory.importYes":          "Importar",
	"memory.importYesDesc":      "El contenido pasa a formar parte de la memoria, que luego se sincroniza al servidor (cifrada con tu clave)",
	"memory.importNo":           "Cancelar",
	"memory.importNoDesc":       "No se lee ni se copia nada",
	"memory.importCancelled":    "Importación cancelada: no se copió nada.",
	"memory.imported":           "Importados: %d",
	"memory.exported":           "Exportados %d archivo(s) a %s",
	"memory.error":              "La operación con la memoria falló: %v",

	// === /project — привязка каталога к проекту ===
	"hint.project":              "vincular este directorio a un proyecto del chat web",
	"project.usage":             "/project — lista de proyectos · bind <nombre> — vincular este directorio · unbind — desvincular · on/off — activar/desactivar el agente en el proyecto",
	"project.listHeader":        "Tus proyectos (● vinculado a este directorio, ○ vinculado en otro sitio):",
	"project.listHint":          "Vincular: /project bind <nombre>",
	"project.none":              "Aún no tienes proyectos: créalos en el chat web.",
	"project.defaultTag":        "[por defecto]",
	"project.bound":             "Proyecto «%s» vinculado a %s",
	"project.unbound":           "Vínculo eliminado de %s",
	"project.notBound":          "Ningún proyecto está vinculado a %s",
	"project.notFound":          "Proyecto «%s» no encontrado. Disponibles: %s",
	"project.needLogin":         "se requiere una cuenta de ExecAI — /login",
	"project.error":             "La operación con el proyecto falló: %v",
	"project.serveQuestion":     "¿Arrancar el escucha en segundo plano para aceptar tareas del chat web?",
	"project.serveYes":          "Arrancar",
	"project.serveYesDesc":      "las herramientas siguen tu permissions.json; sobrevive al cierre de la terminal",
	"project.serveReadOnly":     "Solo lectura",
	"project.serveReadOnlyDesc": "puede mirar, no puede cambiar archivos ni ejecutar comandos",
	"project.serveNo":           "Ahora no",
	"project.serveNoDesc":       "más tarde a mano: execai serve",
	"project.serveStarted":      "▶ Escucha arrancado (pid %d). Estado: execai serve --status · parar: --stop",
	"project.serveLog":          "  salida: %s",
	"project.serveSkipped":      "Escucha no arrancado — las tareas web esperarán en la cola. Arrancar: execai serve",
	"project.serveFailed":       "No se pudo arrancar el escucha: %v",
	"project.serveAlready":      "▶ El escucha ya está en marcha (pid %d)",
	"project.agentOn":           "[agente activo]",
	"project.agentOff":          "[agente APAGADO]",
	"project.enabled":           "Agente «%s» activado en el proyecto «%s»: tomará tareas de ahí",
	"project.disabled":          "Agente «%s» desactivado en el proyecto «%s»: no tomará tareas de ahí",
	"project.notAgent":          "Esta sesión no es un agente, no hay nada que añadir. Inicia sesión con /login en la propia máquina.",
	"project.notInProject":      "El agente no forma parte del proyecto «%s»: ejecuta antes /project bind",
	"project.boundNoTool":       "El directorio %[2]s está vinculado al proyecto «%[1]s», pero no se pudo añadir el agente al proyecto: %[3]v",
}

func init() {
	i18n.Register("es", esMiscMessages)
}
