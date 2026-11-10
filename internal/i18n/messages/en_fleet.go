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
	"fleet.result.timeout":   "the subagent did not finish within %s (set EXECAI_SUBAGENT_TIMEOUT to change the limit, 0 for none)",
	"fleet.result.cancelled": "the subagent was stopped together with its parent",
	"fleet.result.stopped":   "the subagent was stopped by the person",
	"fleet.result.limit":     "(stopped at the step limit of %d — the result may be incomplete; to go on from where it stopped, TaskExtend with id %s)",
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
	"fleet.tui.listHint":        "/agents open <n> — transcript · /agents stop <n> — stop · /agents more <n> [steps] — give more steps · /agents limit [N] — steps for new ones",
	"fleet.tui.openHeader":      "Agent «%s» · %s · %s",
	"fleet.tui.openEmpty":       "(the transcript is empty so far)",
	"fleet.tui.notFound":        "no agent %s — see /agents",
	"fleet.tui.stopping":        "stopping agent «%s»",
	"fleet.tui.notRunning":      "agent «%s» has already finished",
	"fleet.tui.usage":           "usage: /agents · /agents open <n> · /agents stop <n> · /agents more <n> [steps] · /agents limit [N]",
	"fleet.tui.autoReply":       "🤖 background agents finished — their results go to the agent",
	"fleet.tui.cardDone":        "%s agent «%s» — %s",
	"fleet.tui.approveAgent":    "🤖 asked by agent «%s»",
	"fleet.tui.stoppedChildren": "stopped subagents: %d",
	"status.agents":             "agents",
	"hint.agents":               "subagents: list · open <n> · stop <n> · more <n> · limit <N>",
	"fleet.extend.raised":       "agent «%s» got %d more steps — now at step %d of %d",
	"fleet.extend.resumed":      "agent «%s» goes on from step %d: %d more steps (limit %d). The result will come as a separate message or through TaskWait with id %s.",
	"fleet.extend.ceiling":      "agent «%s» already has the largest limit — %d steps",
	"fleet.extend.finished":     "agent «%s» already finished (%s) rather than hitting its step limit — nothing to extend; for more work start a new Task",
	"fleet.extend.prompt":       "You have been given %d more steps. Go on from where you stopped, finish the task, and give the whole result in your last message.",
	"fleet.tui.steps":           "step %d/%d · %d left",
	"fleet.tui.atLimit":         "hit the step limit",
	"fleet.tui.moreHint":        "few steps left — /agents more %s",
	"fleet.tui.limitShow":       "Steps for a new subagent: %d (default %d, allowed 1–%d). Change: /agents limit <N>",
	"fleet.tui.limitSet":        "new subagents will get %d steps",
	"fleet.tui.limitBad":        "need a number from 1 to %d",

	"fleet.ide.notFound": "no such agent: %s",
	"fleet.ide.stopping": "stopping agent «%s»",
}

func init() { i18n.Register("en", enFleetMessages) }
