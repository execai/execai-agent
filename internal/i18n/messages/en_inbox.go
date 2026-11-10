// Package messages/en_inbox — English strings of the "web → agent" channel:
// tasks from the site taken by execai serve or by the editor panel.
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var enInboxMessages = map[string]string{
	"inbox.err.parse": "could not parse the server response",

	"inbox.lock.busy":         "the inbox lock keeps changing hands, try again",
	"inbox.lock.heldByIDE":    "Tasks from the site are already accepted by an editor window (pid %d, folder %s). Close the ExecAI panel there, or run tasks from that window.",
	"inbox.lock.heldByServe":  "Tasks from the site are already accepted by the background execai serve (pid %d).",
	"inbox.lock.signalFailed": "could not signal process %d: %v",
	"inbox.lock.stopTimeout":  "execai serve (pid %d) did not exit within %s — it may be finishing a task. Try again, or run: execai serve --stop --force",

	"inbox.plan.refused":   "Running tasks from the site comes with the Work and Max plans: %s",
	"inbox.plan.serveWarn": "⚠ Your plan does not include running tasks from the site — every task will be answered with a refusal:",

	"inbox.result.interrupted": "(interrupted: %s)",
	"inbox.result.noText":      "(the agent finished but gave no text answer)",

	"inbox.task.bindingsFailed": "could not get the project bindings: %v",
	"inbox.task.dirMissing":     "project directory not found: %s",
	"inbox.task.noDir":          "no directory is set for this project — bind it here: execai → /project bind",
	"inbox.task.notBound":       "this machine is not bound to the task's project — run: execai → /project bind",
	"inbox.task.empty":          "empty task",
	"inbox.task.stopped":        "stopped in the editor",
	"inbox.task.timeout":        "the task did not finish within %s",
	"inbox.task.editorClosed":   "the editor was closed before the task finished",

	"inbox.serve.refused":     "✗ task %s refused: the plan does not include tasks from the site",
	"inbox.serve.statusIDE":   "not running: tasks from the site are accepted by an editor window (pid %d, folder %s)",
	"inbox.serve.stopIDE":     "tasks from the site are accepted by an editor window (pid %d, folder %s), not by execai serve — close that window instead",
	"inbox.ide.otherProject":  "A task for another project (%s) is waiting — open it in the editor or run execai serve",
	"inbox.ide.anotherWindow": "Tasks from the site are accepted by another editor window: %s",
	"inbox.ide.taken":         "Tasks from the site are now accepted in this window",
	"inbox.ide.bindWhich":     "which project? Pass its id (list_workspaces shows them)",
}

func init() { i18n.Register("en", enInboxMessages) }
