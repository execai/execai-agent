// Package messages/en_sync — English strings for sync between computers
// (/sync, execai sync). Merged into the "en" catalog via i18n.Register.
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var enSyncMessages = map[string]string{
	"hint.sync":        "sync memory and settings between your computers",
	"sync.usage":       "/sync — status · /sync on · off · now · password · forget · keys export · keys import",
	"sync.placeholder": "answer — Enter, cancel — Esc",
	"sync.working":     "One moment…",
	"sync.cancelled":   "Cancelled.",
	"sync.error":       "Sync: error: %v",
	"sync.failed":      "Could not reach the sync server: %v",

	// === gate ===
	"sync.gate.noPlan": "Sync comes with the «Work» and «Max» plans: %s",
	"sync.gate.login":  "To sync, sign in to ExecAI: run execai and confirm in the browser (in chat: /login).",

	// === status ===
	"sync.status.off":         "Sync is off. Turn it on: %[1]s on — memory, settings and provider keys become the same on all your computers («Work» and «Max» plans).",
	"sync.status.on":          "Sync is on · last exchange: %s",
	"sync.status.onNever":     "Sync is on · no exchange yet",
	"sync.status.what":        "Syncing: %s — all encrypted on this computer.",
	"sync.status.pausedPlan":  "Paused: sync comes with the «Work» and «Max» plans: %s",
	"sync.status.pausedLogin": "Paused: sign in to ExecAI (in chat: /login).",
	"sync.status.pausedKey": "Paused: the sync key was changed on another computer. " +
		"Enter the sync password again: %s on",
	"sync.status.unreadable": "Files on the server that do not open here: %d — they are encrypted with another key. " +
		"Enter the sync password: %s on",
	"sync.status.tooBig":    "Too large, kept only here: %s",
	"sync.status.quota":     "The sync space on the server is used up — new files are not sent.",
	"sync.status.lastError": "Last error: %s",
	"sync.status.commands":  "Commands: %[1]s now · %[1]s off · %[1]s password · %[1]s forget · %[1]s keys export | import",

	"sync.what.memory":   "memory (files: %d)",
	"sync.what.projects": "projects (%d)",

	// === on ===
	"sync.on.newIntro": "This is your first computer with sync. Memory, settings and provider keys are encrypted here, on the computer: the server keeps only the encrypted copy, and we do not see what is inside.",
	"sync.on.newPassword": "Make up a sync password (at least %d characters). You will enter it " +
		"on your other computers. It cannot be recovered.",
	"sync.on.repeatPassword": "Repeat the password:",
	"sync.on.joinIntro":      "Sync is already on on another of your computers.",
	"sync.on.enterPassword":  "Enter the sync password from your first computer:",
	"sync.on.keyDiffers": "This computer already has its own encryption key, and it differs from the key on the server. " +
		"Memory encrypted with one key cannot be read with the other.",
	"sync.on.chooseKey": "1 — take the key from the server (yours is kept next to it)\n" +
		"2 — keep yours and replace the server's: everything on the server is overwritten from here, " +
		"your other computers will ask for the password again",
	"sync.on.oldKeySaved": "The previous key is kept: %s",
	"sync.on.raced":       "Meanwhile the key on the server was changed from another computer. Try again: %s on",
	"sync.on.racedJoin":   "A sync password was just set on another computer — enter it.",
	"sync.on.created":     "Sync is on.",
	"sync.on.joined":      "Key received, sync is on.",
	"sync.on.replaced":    "Sync is on; the server now holds this computer's data.",
	"sync.on.done":        "Sync is on.",
	"sync.on.firstFailed": "The first exchange failed: %v. I will retry in 10 minutes, or now: %s now",

	// === password ===
	"sync.password.wrong":    "The password did not fit.",
	"sync.password.short":    "Too short: at least %d characters.",
	"sync.password.mismatch": "The passwords differ, let's try again.",
	"sync.password.new":      "New sync password (at least %d characters, it cannot be recovered):",
	"sync.password.changed": "Password changed. Nothing to do on your other computers — " +
		"the password is needed only to add a new one.",

	// === now / off ===
	"sync.now.done":    "Done: %d received, %d sent.",
	"sync.now.off":     "Sync is off. Turn it on: %s on",
	"sync.off.already": "Sync is already off. Turn it on: %s on",
	"sync.off.done": "Sync is off on this computer. The data stays on the server; " +
		"to delete it: %[1]s forget. Turn it on again: %[1]s on",

	// === forget ===
	"sync.forget.warn":      "This deletes from the server the encryption key, all synced memory, settings and provider keys, and the list of computers. Everything stays on your computers. This cannot be undone.",
	"sync.forget.confirm":   "To delete, type the word «%s»:",
	"sync.forget.word":      "delete",
	"sync.forget.cancelled": "Not deleting.",
	"sync.forget.done": "Deleted from the server, files: %d. Everything is still on your computers; " +
		"the others switch sync off at their next check. Turn it on again: %s on",

	// === background ===
	"sync.bg.paused":  "Sync is paused — memory and settings on this computer are untouched.",
	"sync.bg.resumed": "Sync works again.",
	"sync.bg.wiped": "Sync data was deleted from the server (from another computer) — sync is off here. " +
		"Turn it on again: %s on",
	"sync.bg.keyChanged": "Sync is paused: the key was changed on another computer. " +
		"Enter the sync password again: %s on",

	// === keys ===
	"sync.keys.nothing":  "Nothing to move: no provider key is connected on this computer.",
	"sync.keys.exported": "Provider keys: %s.\nA transfer without the server and without a plan. Here is the transfer code — paste it on the other computer (%[3]s keys import):\n\n%[2]s",
	"sync.keys.phrase": "The phrase for the code — send it separately from the code, by another way:\n\n    %s\n\n" +
		"Together the code and the phrase open your keys: do not show them to anyone.",
	"sync.keys.importIntro": "Moving provider keys from another computer (there: keys export).",
	"sync.keys.enterCode":   "Paste the transfer code:",
	"sync.keys.enterPhrase": "Enter the %d-word phrase:",
	"sync.keys.badCode":     "That does not look like a transfer code — copy it whole.",
	"sync.keys.badPhrase":   "The phrase did not fit.",
	"sync.keys.added":       "Added: %s",
	"sync.keys.updated":     "Updated (the key there is newer): %s",
	"sync.keys.kept":        "Unchanged: %s",
	"sync.keys.active":      "The active source did not change: %s. To switch: /source <name>",

	// === execai sync ===
	"sync.cmd.short":            "Sync memory and settings between your computers",
	"sync.cmd.long":             "The agent's memory, settings, permissions and provider keys are the same on all your computers. Everything is encrypted on the computer with the sync password; the server keeps only the encrypted copy, and we do not see what is inside.\nWithout a plan, provider keys can be moved directly: execai sync keys export / import.\n\nSync comes with the «Work» and «Max» plans.",
	"sync.cmd.status":           "Sync status (default)",
	"sync.cmd.on":               "Turn sync on on this computer",
	"sync.cmd.off":              "Turn sync off on this computer (the server keeps the data)",
	"sync.cmd.now":              "Sync now",
	"sync.cmd.password":         "Change the sync password",
	"sync.cmd.forget":           "Delete everything from the server",
	"sync.cmd.keys":             "Move provider keys between computers without the server",
	"sync.cmd.keysExport":       "Show the key transfer code and its phrase",
	"sync.cmd.keysImport":       "Take keys from another computer by code and phrase",
	"sync.cmd.needTerminal":     "This needs a terminal: the answers are typed by hand.",
	"sync.what.config":          "settings and provider keys",
	"sync.status.keysFallback":  "Without sync, provider keys can be moved directly — no server, no plan: %s keys export",
	"sync.pulledSource":         "The active source was changed on another computer: %s",
	"sync.status.projects":      "Projects: %d (%s)",
	"sync.project.off":          "Project files (.execai/) sync when sync is on: %s on",
	"sync.project.pulled":       "Project files (.execai/) received from the server.",
	"sync.project.pushed":       "Project files (.execai/) sent to the server — encrypted.",
	"sync.project.same":         "Project files (.execai/) match the server.",
	"sync.project.empty":        "The project has no .execai/ files yet — they will sync once they appear.",
	"sync.project.backup":       "The project was changed both here and on another computer — the newer copy wins, the other one is kept next to it: %s",
	"project.published":         "Project «%s» created in the web chat.",
	"project.publishedNotBound": "Project «%s» created in the web chat, but binding this directory to it failed: %v",
	"hint.projectPublish":       "make a web chat project from this directory",
	"project.cmd.short":         "Projects: a directory on the computer ↔ a project in the web chat",
	"project.cmd.publish":       "Make a web chat project from the current directory, bind it and send .execai/",
}

func init() { i18n.Register("en", enSyncMessages) }
