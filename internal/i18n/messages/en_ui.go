// Package messages/en_ui — English strings for execai ui (the agent panel in
// a browser). Merged into the "en" catalog via i18n.Register.
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var enUIMessages = map[string]string{
	"ui.cmd.short": "Open the agent panel in the browser (on this computer only)",
	"ui.cmd.long": "The same chat panel as in VS Code and ExecAI Studio, in a browser tab on 127.0.0.1.\n\n" +
		"The address carries a one-time key: the first visit swaps it for a cookie, after that the link opens nothing. " +
		"Only this computer can connect. Tasks from the site and the plan gate work as in the editor panel. Stop: Ctrl+C.",
	"ui.flag.cwd":    "project directory (default: the current one)",
	"ui.flag.port":   "port on 127.0.0.1 (default: a free one)",
	"ui.flag.noOpen": "do not open the browser, only print the address",
	"ui.badCwd":      "not a directory: %s",
	"ui.address":     "ExecAI panel: %s",
	"ui.hint":        "The link works once — the browser that opens it keeps access until execai ui stops. Stop: Ctrl+C.",
	"ui.openFailed":  "Could not open the browser (%v) — open the link yourself.",
	"ui.stopped":     "The panel is stopped.",
	"ui.http.linkUsed": "This link has already been used. Open the panel in the browser where you opened it first, " +
		"or restart execai ui for a new link.",
	"ui.http.noSession": "Open the link execai ui printed in the terminal.",
}

func init() { i18n.Register("en", enUIMessages) }
