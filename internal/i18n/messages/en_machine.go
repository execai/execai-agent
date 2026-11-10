// Package messages/en_machine — English strings for the tutorial (/start),
// the machine card (/machine) and the phone link (/phone).
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var enMachineMessages = map[string]string{
	"hint.start":        "getting started: five steps and what is already done",
	"hint.machine":      "my machine: models, sync, where it can be called from",
	"hint.machineCheck": "check that the model sources answer",
	"hint.phone":        "QR code: call this machine from your phone",

	// === /start ===
	"start.title":    "Getting started",
	"start.progress": "%d of %d done",
	"start.allDone":  "All set: this machine is fully configured.",
	"start.footer": "My machine: /machine · all commands: /help · this list again: /start\n" +
		"Enter — send, Shift+Enter — new line.",

	"start.login.title": "Sign in to ExecAI",
	"start.login.why":   "The account links this machine to the web chat, your phone and your other computers.",
	"start.login.cmd":   "Type: /login — then confirm in the browser.",

	"start.models.title": "Connect models",
	"start.models.why":   "Your own provider keys or an ExecAI plan — memory and calls from the phone work with either.",
	"start.models.cmd":   "Type: /connect <provider> — your own key, /plan — the ExecAI plan, /machine check — make sure the source answers.",

	"start.sync.title":    "Turn on sync",
	"start.sync.why":      "The same settings and memory on all your computers.",
	"start.sync.what":     "Travels, encrypted on this computer with your sync password: the config with the provider keys, your personal memory, the files of bound projects.",
	"start.sync.notWhat":  "Stays here: api_base (the server address of this machine) and the sign-in.",
	"start.sync.password": "Only you know the sync password: we do not keep it and cannot recover it.",
	"start.sync.cmd":      "Type: /sync on («Work» and «Max» plans).",

	"start.project.title": "Bind a folder to a web project",
	"start.project.why":   "The folder becomes a project in the web chat: tasks from the web run in it, the project memory is shared.",
	"start.project.cmd":   "Type: /project publish — make a project from this folder, /project bind <name> — bind to an existing one.",

	"start.phone.title": "Call the agent from your phone",
	"start.phone.why":   "Give this machine tasks from the web chat or your phone — even when it runs on your own keys.",
	"start.phone.cmd":   "Type: /phone — a QR code for the phone. To take tasks, run execai serve.",

	// === /machine ===
	"machine.title":          "My machine",
	"machine.label.machine":  "Machine",
	"machine.label.account":  "Account",
	"machine.label.models":   "Models",
	"machine.label.sync":     "Sync",
	"machine.label.presence": "Reachable",
	"machine.identity":       "%s · %s · agent %s",
	"machine.account.in":     "%s · plan: %s",
	"machine.account.noPlan": "%s · no ExecAI plan",
	"machine.account.out":    "not signed in — /login",

	"machine.source.alive":     "answers (checked %s)",
	"machine.source.down":      "did not answer (checked %s)",
	"machine.source.unchecked": "not checked yet — /machine check",
	"machine.source.needLogin": "sign in to use it — /login",
	"machine.source.noPlan":    "no active plan — /plan",
	"machine.source.active":    "in use",

	"machine.sync.off":      "off — /sync on",
	"machine.sync.on":       "on · last upload: %s",
	"machine.sync.never":    "on · nothing uploaded yet",
	"machine.sync.paused":   "paused — details: /sync",
	"machine.sync.inside":   "inside: %s",
	"machine.sync.config":   "config and provider keys",
	"machine.sync.memory":   "personal memory (files: %d)",
	"machine.sync.projects": "projects: %d",
	"machine.sync.empty":    "nothing yet",
	"machine.sync.mode":     "mode: %s",
	"machine.mode.all":      "all projects",
	"machine.mode.selected": "bound folders only",
	"machine.mode.none":     "no projects",

	"machine.presence.serve":      "takes tasks: yes — execai serve (pid %d)",
	"machine.presence.ide":        "takes tasks: yes — an editor window (pid %d)",
	"machine.presence.noListener": "takes tasks: no — run execai serve",
	"machine.presence.reachable":  "call from the web and the phone: yes — /phone",
	"machine.presence.blocked":    "call from the web and the phone: not yet — %s",
	"machine.block.login":         "sign in (/login)",
	"machine.block.plan":          "needs the «Work» or «Max» plan",
	"machine.block.listener":      "run execai serve",

	"machine.footer":   "Check the sources again: /machine check · tutorial: /start",
	"machine.checking": "Checking the sources…",

	// === /phone ===
	"phone.title":     "Call this machine from your phone",
	"phone.scan":      "Point the phone camera at the code or open the link:",
	"phone.what":      "The page opens the web chat with this machine chosen for tasks.",
	"phone.needLogin": "The phone link appears once you sign in: /login",
	"phone.listener":  "To take tasks, this machine needs execai serve running (in a separate window).",
	"phone.plan":      "Tasks from the web and the phone come with the «Work» and «Max» plans: %s",
	"phone.qrFailed":  "Could not draw the QR code: %v",
}

func init() { i18n.Register("en", enMachineMessages) }
