// Package messages/en_fleet — English strings of subagents and the "who is
// doing what" list (E101): the Task tool, the terminal's agents block and
// /agents, the editor panel.
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var enFleetMessages = map[string]string{
	"fleet.ask.prefix":      "🤖 agent «%s» asks:",
	"fleet.card.write":      "may edit",
	"fleet.card.background": "in background",

	"fleet.state.queued":    "queued",
	"fleet.state.running":   "working",
	"fleet.state.waiting":   "waiting for you",
	"fleet.state.done":      "done",
	"fleet.state.failed":    "failed",
	"fleet.state.cancelled": "stopped",

	"fleet.started":          "Subagent %s («%s») started in the background. Its result will come as a separate message when it finishes; to wait for it now, call TaskWait with id %s.",
	"fleet.result.timeout":   "the subagent did not finish within %s",
	"fleet.result.cancelled": "the subagent was stopped together with its parent",
	"fleet.result.stopped":   "the subagent was stopped by the person",
	"fleet.result.limit":     "(stopped at the step limit of %d — the result may be incomplete)",
	"fleet.wait.none":        "no background subagents to wait for",
	"fleet.wait.unknown":     "no subagent %s",
	"fleet.note.header":      "Background subagents finished:",
	"fleet.note.item":        "— «%s» (%s), %s:",
	"fleet.err.model":        "model %q cannot be chosen here — the subagent runs on the current model of %s",
	"fleet.err.cwdOutside":   "cwd %s is outside the parent's directory %s",
	"fleet.err.cwdMissing":   "cwd %s is not a directory",

	"fleet.tui.header":          "🤖 Agents",
	"fleet.tui.more":            "… %d more — /agents",
	"fleet.tui.listHeader":      "Subagents (newest last):",
	"fleet.tui.listEmpty":       "No subagents yet. The agent starts them with the Task tool.",
	"fleet.tui.listHint":        "/agents open <n> — transcript · /agents stop <n> — stop",
	"fleet.tui.openHeader":      "Agent «%s» · %s · %s",
	"fleet.tui.openEmpty":       "(the transcript is empty so far)",
	"fleet.tui.notFound":        "no agent %s — see /agents",
	"fleet.tui.stopping":        "stopping agent «%s»",
	"fleet.tui.notRunning":      "agent «%s» has already finished",
	"fleet.tui.usage":           "usage: /agents · /agents open <n> · /agents stop <n>",
	"fleet.tui.autoReply":       "🤖 background agents finished — their results go to the agent",
	"fleet.tui.cardDone":        "%s agent «%s» — %s",
	"fleet.tui.approveAgent":    "🤖 asked by agent «%s»",
	"fleet.tui.stoppedChildren": "stopped subagents: %d",
	"status.agents":             "agents",
	"hint.agents":               "subagents: list · open <n> · stop <n>",

	"fleet.ide.notFound": "no such agent: %s",
	"fleet.ide.stopping": "stopping agent «%s»",
}

func init() { i18n.Register("en", enFleetMessages) }
