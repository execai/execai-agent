// Package messages/en_misc — English strings for the misc TUI/REPL/agent batch
// (batch 4+: tui.go leftovers, plain REPL, compact, agent loop, welcome screen).
// Source of truth: any new key MUST be added here first.
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var enMiscMessages = map[string]string{
	// === Boot / login flow (tui.go) ===
	"ui.boot.noLogin": "ℹ Working without an ExecAI account — source: %s · model: %s.\n" +
		"  /login — connect an ExecAI account (our catalog of ~34 models + billing).",
	"ui.login.intro": "Hi! To log in you need to confirm the agent in your browser (like gh auth login).\n" +
		"If for some reason device-flow doesn't work — you can paste a JWT token (eyJ…) here and press Enter.\n\n" +
		"ℹ An ExecAI account is OPTIONAL: you can work with your own subscription without logging in.\n" +
		"  /connect kimi <key>   → /source kimi     (Kimi Code)\n" +
		"  /connect zai <key>    → /source zai      (Z.ai GLM)\n" +
		"  /connect openai <key> → /source openai   (OpenAI API)\n" +
		"  Also: anthropic, kimi-api, claude-cli, codex-cli, ollama — /connect shows everything.",
	"ui.login.staleToken":          "Old token is invalid on %s. Starting device-flow for a new login…",
	"ui.login.startFlow":           "Starting device-flow for a new login…",
	"ui.login.deviceFlowOpen":      "Open in your browser and confirm:\n\n  %s\n\nCode (in case you enter it manually): %s\n\nPress Enter / [y] to open the browser automatically. [n] to open it yourself.",
	"ui.boot.modelsFallbackFailed": "could not fetch models nor build a fallback list",
	"ui.welcome":                   "execai %s · %s/%s · %s · %s\nType a task. /model — models, /help — commands, /quit — exit.",

	// === Stream errors / token expiry ===
	"ui.stream.tokenExpiredHint": "→ Switch /source zai|ollama|anthropic, or /login to re-confirm.",
	"ui.stream.tokenExpiredFlow": "ExecAI token expired — starting device-flow. Confirm in your browser.",

	// === /compact ===
	"ui.compact.historyNote": "[History compacted earlier (%d messages): %s]",
	"ui.compact.done":        "📦 History compacted: %d messages → 1 summary (~%d chars)",
	"ui.compact.working":     "compacting history…",
	"ui.compact.tooShort":    "history is still short — nothing to compact (need >%d messages)",
	"ui.compact.truncated":   "…(truncated)",
	"ui.compact.promptSystem": "You are a context compressor for an AI agent. You are given a conversation transcript. " +
		"Return a BRIEF summary (≤500 words) that preserves:\n" +
		"  • key decisions and their reasons\n" +
		"  • important file paths and commands\n" +
		"  • tool-call results that may be useful later\n" +
		"  • errors and how they were resolved\n" +
		"Omit chit-chat and acknowledgements. Write in English, telegraphic style.",
	"ui.compact.promptUser": "Compress this conversation:\n\n%s",

	// === Autoloop ===
	"ui.autoloop.defaultPrompt": "continue",
	"ui.autoloop.wake":          "🌙 autoloop: waking up in %s (%s) → prompt: %q",

	// === /paste ===
	"ui.paste.empty":     "No pastes in this session. Ctrl+V a large chunk of text → marker.",
	"ui.paste.header":    "Pastes (Ctrl+V ≥200 chars or with \\n):\n",
	"ui.paste.showHint":  "\nShow: /paste show <N>",
	"ui.paste.notNumber": "not a number: %s",
	"ui.paste.notFound":  "no paste #%d",
	"ui.paste.usage":     "usage: /paste [list|show <N>]",

	// === /whoami ===
	"ui.whoami.notLoggedIn": "(not logged in — /login)",

	// === /classic & /mouse ===
	"ui.classic.on":  "✓ classic TUI ON — restart execai (/quit → execai). Alt-screen + pinned status bar, Shift+drag to copy.",
	"ui.classic.off": "✓ Ink-style (default) — restart execai. History in scrollback, native selection and scroll.",
	"ui.mouse.off":   "🖱  mouse capture OFF — the mouse selects text, the menu ignores clicks. Enable: /mouse on",
	"ui.mouse.on":    "🖱  mouse capture ON — wheel scrolls, clicks work on the menu. Select text: Shift+drag. Off: /mouse off",

	// === /effort ===
	"ui.effort.pickerHint": "effort picker: ←/→ select, Enter confirm, Esc cancel",
	"ui.effort.current":    "Effort now: %s (%d tokens)\nChange: /effort <off|low|medium|high|max>\n  off=0  low=1024  medium=4096  high=8192  max=32000\nWorks for Anthropic-compat sources (Z.ai, Kimi, Anthropic, ollama-cloud, claude-cli).",
	"ui.effort.set":        "✓ effort=%s (%d tokens)",

	// === /max-iterations ===
	"ui.maxIter.current": "Max iterations now: %d\nTool-use iteration limit per turn. When exhausted — soft stop, the user can say 'continue'.\nChange: /max-iterations <N>  (recommended 20-200; default 50)",
	"ui.maxIter.usage":   "/max-iterations <N>  where N is 1 to 500 (recommended 20-200)",

	// === /loop ===
	"ui.loop.status":      "🔁 loop: every %s — %q. /loop stop to stop",
	"ui.loop.inactive":    "loop is inactive. Usage: /loop <interval> <prompt>  (example: /loop 5m check the build status)",
	"ui.loop.notRunning":  "loop is not running anyway",
	"ui.loop.stopped":     "🔁 loop stopped",
	"ui.loop.usage":       "/loop <interval> <prompt>  (e.g.: /loop 5m check the build status)",
	"ui.loop.badInterval": "can't parse interval %s — need something like 30s, 5m, 1h",
	"ui.loop.started":     "🔁 loop started: every %s — %q\nStop: /loop stop",

	// === /log ===
	"ui.log.none":   "no log yet: %s",
	"ui.log.header": "📜 Last %d requests (%s):\n",

	// === /usage & /cd ===
	"ui.usage.fetchFailed": "couldn't fetch usage: %s",
	"ui.cd.current":        "cwd now: %s",

	// === Sessions ===
	"ui.session.new":         "new conversation",
	"ui.sessions.header":     "\nSaved conversations (newest first):\n",
	"ui.sessions.empty":      "(empty)\n",
	"ui.sessions.switchHint": "\nSwitch: /resume <number|id>",
	"ui.resume.notFound":     "conversation %q not found. /sessions — list",
	"ui.resume.loadFailed":   "couldn't load: %s",
	"ui.resume.resumed":      "resuming: %s",
	"ui.title.renamed":       "renamed: %s",

	// === /permissions ===
	"ui.perms.toolsEmpty": "  always allowed tools: (empty)\n",
	"ui.perms.exactCount": "  always allowed exact commands: %d entries\n",
	"ui.perms.resetHint":  "\nTo reset — delete the file manually or run: rm ~/.config/execai/permissions.json",

	// === /model ===
	"ui.model.notFound": "model %q not found",
	"ui.model.switched": "switched to %s/%s — %s (history preserved)",

	// === Approve ===
	"ui.approve.denied":       "denied",
	"ui.approve.allowedTool":  "allowed: all %s calls this session",
	"ui.approve.allowedExact": "allowed: this command this session",
	"ui.approve.navHint":      "← → or Tab — switch, Enter — select, Esc — deny",

	// === Tool-call summaries ===
	"ui.toolSummary.write": "Write %s  (%d bytes)",

	// === Plain REPL (chat.go) ===
	"plain.err.fetchModels":    "couldn't fetch the model list",
	"plain.err.emptyModels":    "server returned an empty model list",
	"plain.err.pickModelEmpty": "couldn't pick a model (empty list?)",
	"plain.err.pickModel":      "couldn't pick a model",
	"plain.commands":           "Commands: /model — pick a model, /clear — clear history, /quit — exit.",
	"plain.historyCleared":     "(history cleared)",
	"plain.modelSwitchHint":    "\nSwitch: /model <number> or /model <model_name>",
	"plain.modelNotFound":      "(model %q not found. /model — see the list)",
	"plain.modelSwitched":      "(switched to %s/%s — %s; history preserved)",
	"plain.errorPrefix":        "error:",
	"plain.modelsHeader":       "\nAvailable models (★ — primary, • — current):",

	// === Agent loop (internal/agent) ===
	"loop.iterationLimit": "⚠ Reached the limit of %d iterations — the task is not finished. Say “continue” to grant %d more, or rephrase.",

	// === Welcome screen (first launch) ===
	"welcome.text": `Hi! This is execai — a CLI agent for development.

What it can do:
  • read/write/edit files (Read, Write, Edit)
  • search (Grep, Glob, LS, Tree)
  • run shell commands (Bash) — read-only without asking, the rest with confirmation
  • make HTTP requests (WebFetch — no browser; a real browser will come separately)
  • keep a to-do list (TodoWrite)

Memory:
  • ./EXECAI.md           — project memory (repo context)
  • ~/.config/execai/EXECAI.md — your personal settings
Both files are loaded into the system prompt automatically every session.

Commands:
  /model               — list models
  /model <num|substring> — switch (history preserved)
  /clear               — clear history
  /help                — this message
  /quit                — exit

Tip: Enter — send, Shift+Enter — new line.`,

	// === WebSearch / WebFetch tools ===
	"tool.websearch.noLogin": "Web search is unavailable: it runs through the ExecAI gateway and needs an ExecAI account.\n" +
		"Without a login the local browser still works — use WebFetch to open any URL and follow the links it returns.\n" +
		"Run /login to enable search (and the ExecAI model catalog along with it).",
	"tool.websearch.sources": "Sources:",
	"tool.websearch.empty":   "Search returned nothing. Try rephrasing the query or open a specific page with WebFetch.",

	// === AskUser picker + субагенты ===
	"ask.title":             "The agent is asking:",
	"ask.hint":              "↑↓ choose · Enter confirm · 1-4 pick directly · Esc — let the agent decide",
	"ask.answered":          "Question: %s → %s",
	"ask.dismissed":         "left to the agent",
	"ask.dismissedForModel": "The user dismissed the question without choosing. Decide yourself, state the assumption you made, and continue.",
	"subagent.emptyResult":  "the subagent returned nothing",
}

func init() {
	i18n.Register("en", enMiscMessages)
}
