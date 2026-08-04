// Package messages/en — English translations for execai.
// Source of truth: any new key MUST be added here first.
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var enMessages = map[string]string{
	// === Placeholders / hero ===
	"placeholder.chat":  "Type a task. Enter — send, /help — commands.",
	"placeholder.login": "Paste the JWT token from your browser ExecAI session and press Enter",
	"header.login":      "login",

	// === Help footer (bottom of TUI) ===
	"help.enter":     "send",
	"help.history":   "history",
	"help.effort":    "effort",
	"help.copy":      "copy",
	"help.stop":      "stop",
	"help.exit":      "exit",
	"help.slashHelp": "/help",

	// === Status bar labels ===
	"status.src":      " src ",
	"status.provider": " provider ",
	"status.model":    " model ",
	"status.effort":   " effort ",
	"status.user":     " user ",
	"status.cwd":      " cwd ",
	"status.loop":     " 🔁 loop ",
	"status.working":  " ⏵ working ",
	"status.unknown":  "—",
	"status.notLogin": "(not logged in)",

	// === Common errors ===
	"err.notLoggedIn":    "not logged in — /login",
	"err.unknownCommand": "unknown command: %s",
	"err.configSave":     "config save: %s",
	"err.noFolder":       "not a folder: %s",

	// === /lang command ===
	"lang.current":     "Current language: %s. Available: %s",
	"lang.usage":       "Usage: /lang <code>  (e.g. /lang ru). Available: %s",
	"lang.changed":     "✓ Language set to %s. Saved to config.",
	"lang.unknown":     "✗ Unknown language: %s. Available: %s",
	"lang.autoDetect":  "🌍 Language: %s (auto-detected from %s). Change: /lang <%s>",
	"lang.notDetected": "🌍 Language: %s (default). Change: /lang <%s>",

	// === Quit / general system ===
	"quit.confirmCtrlC":  "Press Ctrl+C again within 2s to quit",
	"quit.confirmCtrlD":  "Press Ctrl+D again within 2s to quit",
	"sys.historyCleared": "history cleared",
	"sys.imgHintPath":    "⚠ Image path not recognized (probably spaces/non-ASCII in name). Wrap it in single quotes: '/home/user/Pictures/Screenshot.png'",

	// === Suggest menu ===
	"suggest.footer":    "↑↓ select · Tab/Enter confirm · Esc close",
	"effort.sliderHint": "(Shift+Tab to change)",

	// === Slash-command hints (autocomplete menu) ===
	"hint.help":                   "list all commands",
	"hint.model":                  "pick a model",
	"hint.usage":                  "balance + spend per model",
	"hint.compact":                "compact history into a summary",
	"hint.log":                    "recent LLM requests (model_requested vs model_returned)",
	"hint.loop":                   "repeat a prompt periodically (/loop 5m <text>, /loop stop)",
	"hint.effort":                 "open reasoning-effort picker (off/low/medium/high/xhigh/max)",
	"hint.maxIterations":          "tool-use iteration limit per turn (default 50)",
	"hint.subscriptions":          "connected providers (Z.ai/Anthropic/OpenAI)",
	"hint.connect.zai":            "connect Z.ai GLM Coding Plan (api-key required)",
	"hint.connect.kimi":           "Kimi Code Coding Plan (K3/K2.7, key from kimi.com/code/console)",
	"hint.connect.kimiapi":        "Moonshot Platform pay-per-token (moonshot-v1/kimi-latest, key from platform.moonshot.ai)",
	"hint.connect.zaiapi":         "Z.ai open platform, pay-per-token — same key as the subscription, no tool restrictions",
	"hint.connect.anthropic":      "connect Anthropic API (sk-ant-... from console.anthropic.com)",
	"hint.connect.openai":         "OpenAI Platform pay-per-token (sk-… from platform.openai.com)",
	"hint.connect.codexcli":       "local OpenAI Codex CLI (ChatGPT Plus/Pro quota, no key)",
	"hint.connect.claudecli":      "local Claude Code (Pro/Max subscription quota, no key)",
	"hint.connect.ollama":         "local Ollama runner (localhost:11434, no key)",
	"hint.source":                 "switch source (execai/zai/...)",
	"hint.mouseOff":               "disable mouse capture → text selection works",
	"hint.mouseOn":                "enable capture (wheel/clicks on menu)",
	"hint.inlineOn":               "TUI without alt-screen (native scroll, but selection resets on stream)",
	"hint.lang":                   "change UI language (en/ru/es/de/zh), auto-detected from $LANG",
	"hint.paste":                  "list pastes (Ctrl+V of large chunks), /paste show <N> — content",
	"hint.cd":                     "change working directory",
	"hint.sessions":               "list conversations",
	"hint.resume":                 "resume a conversation",
	"hint.new":                    "new conversation",
	"hint.clear":                  "clear history",
	"hint.whoami":                 "who is logged in",
	"hint.config":                 "config path + api_base",
	"hint.permissions":            "persistent allow-list",
	"hint.login":                  "device-flow login",
	"hint.logout":                 "log out",
	"hint.quit":                   "exit",
	"placeholder.confirmBrowser":  "Confirm login in your browser",
	"login.loggedOutStartingFlow": "Logged out. Starting device-flow for a new login…",
	"help.body":                   "Commands:\n\nModels & sources:\n  /model [num|id]      — list models or switch\n  /source [name]       — switch source (execai/zai/zai-api/kimi/kimi-api/anthropic/openai/claude-cli/codex-cli/ollama)\n  /connect <provider>  — connect a subscription (api-key or local CLI)\n  /disconnect <name>   — remove a subscription\n  /subscriptions       — list connected providers\n  /usage               — plan + quotas + spend (per active source)\n  /effort              — reasoning-effort picker (also Shift+Tab)\n  /max-iterations [N]  — tool-use iteration limit per turn (default 50)\n\nConversations:\n  /new                 — new conversation (current one is saved)\n  /sessions, /list     — list saved conversations\n  /resume <num|id>     — open a saved conversation\n  /title <text>        — rename current conversation\n  /compact             — compress history into a summary\n  /clear, Ctrl+L       — clear current history\n  /log                 — recent LLM requests\n\nAutomation:\n  /loop 5m <text>      — repeat a prompt periodically (/loop stop)\n  /paste [show <N>]    — collapsed pastes (Ctrl+V)\n\nSettings:\n  /lang [code]         — UI language (en/ru/es/de/zh)\n  /mouse on|off        — mouse capture (classic TUI)\n  /classic on|off      — alt-screen TUI instead of Ink-style (restart)\n  /permissions         — persistent tool permissions\n  /config              — show config\n  /whoami              — current user\n  /login, /logout      — session\n  /quit, Ctrl+D        — exit\n\nHotkeys: Enter — send · Shift+Enter — new line · ↑↓ — input history · Shift+Tab — effort · Ctrl+C — cancel stream",
	"update.latest":               "✓ execai %s — up to date",
	"stream.interrupted":          "stream interrupted",
	"stream.aborted":              "(aborted)",
	"effort.pickerCancelled":      "effort picker — cancelled",
	"effort.changed":              "🧠 effort → %s (%d tokens)",
	"login.waitingPoll":           "waiting for browser confirmation… (poll #%d)",
	"login.deviceFlowFailed":      "device-flow failed: %s",
	"login.checkingToken":         "Checking token (paste-mode)…",
	"login.authFailed":            "authorization failed: %s",
	"login.browserOpenFailed":     "couldn't open the browser automatically — open the URL manually (see above)",
	"login.openingBrowser":        "opening browser…",
	"login.openManually":          "OK, open the URL manually and confirm in the browser",
	"login.contactingServer":      "Contacting server…",
	"login.linkFailed":            "couldn't get link: %s",
	"login.waitingBrowser":        "waiting for browser confirmation…",
	"login.loadAgentFailed":       "login succeeded but couldn't load the agent: %s",
	"login.greet":                 "Logged in as %s.",
	"login.greetAlias":            "Logged in as %s · agent: %s",
	"login.greetSuffix":           " · %s/%s · /help — commands.",
	"session.untitled":            "(untitled)",
	"approve.title":               "⚠  Confirm %s execution",
	"approve.once":                "Once",
	"approve.allToolSession":      "All %s this session",
	"approve.thisCmdSession":      "This command this session",
	"approve.forever":             "FOREVER",
	"approve.deny":                "Deny",
	"approve.savedForever":        "allowed FOREVER: %s (saved to ~/.config/execai/permissions.json)",
}

func init() {
	i18n.Register("en", enMessages)
}
