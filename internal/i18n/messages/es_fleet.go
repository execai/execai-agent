// Package messages/es_fleet — textos en español de los subagentes y de la
// lista «quién hace qué» (E101).
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var esFleetMessages = map[string]string{
	"fleet.ask.prefix":      "🤖 el agente «%s» pide:",
	"fleet.card.write":      "puede editar",
	"fleet.card.background": "en segundo plano",

	"fleet.state.queued":    "en cola",
	"fleet.state.running":   "trabajando",
	"fleet.state.waiting":   "esperando tu respuesta",
	"fleet.state.done":      "listo",
	"fleet.state.failed":    "error",
	"fleet.state.cancelled": "detenido",

	"fleet.started":          "El subagente %s («%s») se inició en segundo plano. Su resultado llegará como un mensaje aparte cuando termine; para esperarlo ahora, llama a TaskWait con id %s.",
	"fleet.result.timeout":   "el subagente no terminó en %s (el límite se cambia con EXECAI_SUBAGENT_TIMEOUT, 0 = sin límite)",
	"fleet.result.cancelled": "el subagente se detuvo junto con su agente padre",
	"fleet.result.stopped":   "la persona detuvo el subagente",
	"fleet.result.limit":     "(detenido en el límite de %d pasos — el resultado puede estar incompleto; para seguir desde donde se detuvo, TaskExtend con id %s)",
	"fleet.wait.none":        "no hay subagentes en segundo plano que esperar",
	"fleet.wait.unknown":     "no existe el subagente %s",
	"fleet.note.header":      "Los subagentes en segundo plano terminaron:",
	"fleet.note.item":        "— «%s» (%s), %s:",
	"fleet.err.model":        "aquí no se puede elegir el modelo %q — el subagente usa el modelo actual de %s",
	"fleet.err.cwdOutside":   "cwd %s está fuera del directorio del agente padre %s",
	"fleet.err.cwdMissing":   "cwd %s no es un directorio",

	"fleet.tui.header":          "🤖 Agentes",
	"fleet.tui.more":            "… %d más — /agents",
	"fleet.tui.listHeader":      "Subagentes (los nuevos abajo):",
	"fleet.tui.listEmpty":       "Aún no hay subagentes. El agente los inicia con la herramienta Task.",
	"fleet.tui.listHint":        "/agents open <n> — transcripción · /agents stop <n> — detener · /agents more <n> [pasos] — dar más pasos · /agents limit [N] — pasos para los nuevos",
	"fleet.tui.openHeader":      "Agente «%s» · %s · %s",
	"fleet.tui.openEmpty":       "(la transcripción aún está vacía)",
	"fleet.tui.notFound":        "no hay agente %s — mira /agents",
	"fleet.tui.stopping":        "deteniendo el agente «%s»",
	"fleet.tui.notRunning":      "el agente «%s» ya terminó",
	"fleet.tui.usage":           "uso: /agents · /agents open <n> · /agents stop <n> · /agents more <n> [pasos] · /agents limit [N]",
	"fleet.tui.autoReply":       "🤖 los agentes en segundo plano terminaron — sus resultados van al agente",
	"fleet.tui.cardDone":        "%s agente «%s» — %s",
	"fleet.tui.approveAgent":    "🤖 pregunta el agente «%s»",
	"fleet.tui.stoppedChildren": "subagentes detenidos: %d",
	"status.agents":             "agentes",
	"hint.agents":               "subagentes: lista · open <n> · stop <n> · more <n> · limit <N>",
	"fleet.extend.raised":       "el agente «%s» recibió %d pasos más — ahora en el paso %d de %d",
	"fleet.extend.resumed":      "el agente «%s» sigue desde el paso %d: %d pasos más (límite %d). El resultado llegará en un mensaje aparte o con TaskWait con id %s.",
	"fleet.extend.ceiling":      "el agente «%s» ya tiene el límite máximo — %d pasos",
	"fleet.extend.finished":     "el agente «%s» ya terminó (%s), no se detuvo por el límite de pasos — no hay nada que extender; para más trabajo, un Task nuevo",
	"fleet.extend.prompt":       "Te han dado %d pasos más. Sigue desde donde te detuviste, termina la tarea y da el resultado completo en tu último mensaje.",
	"fleet.tui.steps":           "paso %d/%d · quedan %d",
	"fleet.tui.atLimit":         "llegó al límite de pasos",
	"fleet.tui.moreHint":        "quedan pocos pasos — /agents more %s",
	"fleet.tui.limitShow":       "Pasos para un subagente nuevo: %d (por defecto %d, permitido 1–%d). Cambiar: /agents limit <N>",
	"fleet.tui.limitSet":        "los subagentes nuevos tendrán %d pasos",
	"fleet.tui.limitBad":        "hace falta un número de 1 a %d",

	"fleet.ide.notFound": "no existe el agente: %s",
	"fleet.ide.stopping": "deteniendo el agente «%s»",
}

func init() { i18n.Register("es", esFleetMessages) }
