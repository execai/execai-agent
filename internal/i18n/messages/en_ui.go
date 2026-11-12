// Package messages/en_ui — English strings for execai ui (the agent panel in
// a browser). Merged into the "en" catalog via i18n.Register.
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var enUIMessages = map[string]string{
	"ui.cmd.short": "Open the agent panel in a window of its own or in the browser (on this computer only)",
	"ui.cmd.long": "The same chat panel as in VS Code and ExecAI Studio, on 127.0.0.1.\n\n" +
		"It opens as a window of its own — no tabs, no address bar — when Google Chrome, Chromium, Microsoft Edge, " +
		"Yandex Browser, Brave or Vivaldi is installed, and as a tab of the default browser otherwise. Closing the window stops the panel; so does Ctrl+C. " +
		"A second run in the same folder shows the window that is already open.\n\n" +
		"The address carries a one-time key: the first visit swaps it for a cookie, after that the link opens nothing. " +
		"Only this computer can connect. Tasks from the site and the plan gate work as in the editor panel.",
	"ui.flag.cwd":             "project directory (default: the current one)",
	"ui.flag.port":            "port on 127.0.0.1 (default: the folder's own, one of 23750–23769; the next one when it is taken)",
	"ui.flag.noOpen":          "do not open the browser, only print the address",
	"ui.flag.window":          "a window of its own only: an error when no suitable browser is installed",
	"ui.flag.browser":         "open a tab of the default browser, not a window of its own",
	"ui.flag.last":            "the folder of the last run (what the launcher shortcut uses)",
	"ui.flag.code":            "print the one-time code that signs a window in to the panel already running for this folder",
	"ui.flag.installShortcut": "create a launcher shortcut (with --cwd: for that folder)",
	"ui.flag.removeShortcut":  "remove the launcher shortcut",
	"ui.flags.exclusive":      "--window, --browser and --no-open do not go together: pick one",
	"ui.flags.oneAction":      "--install-shortcut, --remove-shortcut, --code and --stop do not go together: pick one",
	"ui.badCwd":               "not a directory: %s",
	"ui.address":              "ExecAI panel: %s",
	"ui.hint":                 "The link works once — the browser that opens it keeps access until execai ui stops. Stop: Ctrl+C.",
	"ui.openFailed":           "Could not open the browser (%v) — open the link yourself.",
	"ui.stopped":              "The panel is stopped.",
	"ui.window.opened":        "Opening the ExecAI panel in a window of its own (%s). Its address: %s",
	"ui.window.hint":          "Close the window or press Ctrl+C to stop the panel.",
	"ui.window.tabFallback":   "A window of its own takes one of %s — none is installed here, opening a tab.",
	"ui.window.none":          "no browser for a window of its own; any of these will do: %s",
	"ui.window.noScreen":      "no graphical session (DISPLAY and WAYLAND_DISPLAY are empty) — nowhere to open a window",
	"ui.window.notHere":       "The browser named in the settings (%s) is not on this computer — picking one myself.",
	"ui.window.startFailed":   "The window did not open in %s (%v) — opening a tab.",
	"ui.window.silent":        "The window in %s never showed the panel. You can sign in with a code (execai ui --code in this folder) or with the link:",
	"ui.window.failed":        "the window did not open in %s: %v",
	"ui.last.none":            "no project folder is chosen: run execai ui in the project folder, or make a shortcut for it — execai ui --install-shortcut --cwd FOLDER",
	"ui.last.noneTitle":       "ExecAI: no project folder is chosen",
	"ui.last.noneNotice":      "Run execai ui in the project folder — the shortcut will open it from then on.",
	"ui.shortcut.noLast":      "The shortcut opens the folder of the last run. There is none yet: run execai ui in the project folder first, or make a shortcut for a folder with --cwd FOLDER.",
	"ui.window.closed":        "The window is closed — stopping the panel.",
	"ui.alarm.usedTwice":      "Two came with the panel's one-time link: somebody else has it too. Every sign-in is dropped, running turns are stopped and waiting permission requests refused: the other side may have sent the agent a message — look at what the chats did last.",
	"ui.alarm.noPage":         "The panel's one-time link was used, but no panel page connected afterwards. Every sign-in is dropped, running turns are stopped and waiting permission requests refused: whoever came in may have sent the agent a message — look at what the chats did last.",
	"ui.alarm.reopen":         "Opening the window anew, with a new key.",
	"ui.alarm.useCode":        "To sign in again, run execai ui --code in this folder and enter the code on the panel's page.",
	"ui.already.window":       "The panel of this folder is already open — showing its window (%s).",
	"ui.already.tab":          "The panel of this folder is already open: %s",
	"ui.already.starting":     "The panel of this folder is already starting.",
	"ui.code.line":            "One-time code for the page %s: %s (good for %d min)",
	"ui.code.notRunning":      "execai ui is not running for this folder",
	"ui.flag.stop":            "stop the panel that runs for this folder",
	"ui.flag.all":             "with --stop: stop the panels of all folders",
	"ui.flags.allNeedsStop":   "--all goes only with --stop",
	"ui.stop.done":            "The panel is stopped: %s",
	"ui.stop.notRunning":      "No panel is running for this folder.",
	"ui.stop.noneRunning":     "No panel is running.",
	"ui.stop.failed":          "the panel did not stop: it still holds the folder %s",
	"ui.stop.asked":           "Asked to stop by execai ui --stop.",
	"ui.noScreen":             "No graphical session (DISPLAY and WAYLAND_DISPLAY are empty) — no browser is started: open the link yourself.",
	"ui.shortcut.notOurs":     "what is at %s was not made by execai ui --install-shortcut — left as it is",
	"ui.shortcut.noHome":      "no home folder: nowhere to keep a shortcut",
	"ui.shortcut.unsupported": "there is no launcher shortcut on this system",
	"ui.shortcut.comment":     "The ExecAI agent panel",
	"ui.shortcut.installed":   "Launcher shortcut created: %s",
	"ui.shortcut.fallback":    "The shortcut could not be created — a launcher file instead: %s",
	"ui.shortcut.removed":     "Launcher shortcut removed: %s",
	"ui.shortcut.absent":      "No launcher shortcut: %s",
	"ui.shortcut.tabOnly":     "A window of its own takes one of %s. Without one the shortcut opens a tab, and the panel keeps running until it is stopped: execai ui --stop --all.",
	"ui.http.linkUsed": "This link has already been used. Open the panel in the browser where you opened it first, " +
		"or restart execai ui for a new link.",
	"ui.http.noSession":    "Open the link execai ui printed in the terminal.",
	"ui.http.login.code":   "Run execai ui --code in a terminal, in the project folder, and enter here the code it prints.",
	"ui.http.login.link":   "Or open the link execai ui printed in the terminal.",
	"ui.http.login.label":  "One-time code",
	"ui.http.login.button": "Sign in",
	"ui.http.login.wrong":  "The code did not match. If it has been used or is more than five minutes old, run execai ui --code again.",
	"ui.http.login.down":   "The panel does not answer — check that execai ui is running.",
}

func init() { i18n.Register("en", enUIMessages) }
