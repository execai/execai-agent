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

	// === /key — ключ шифрования памяти ===
	"hint.key":  "memory encryption key (show / export / import / new)",
	"key.usage": "/key — show status · /key new — create · /key export — show the private key · /key import <key> — install a key from another machine",
	"key.absent": "No encryption key yet.\n" +
		"It will be created automatically on the first sync, or now with /key new.\n" +
		"Location: %s",
	"key.present": "Encryption key is in place.\n" +
		"Public key (safe to share, others encrypt FOR you with it):\n" +
		"  %s\n" +
		"File: %s",
	"key.created": "Encryption key created.\n" +
		"Public key: %s\n" +
		"File: %s",
	"key.alreadyExists": "A key already exists (public: %s). It was NOT replaced — that would lose access to everything encrypted with it.",
	"key.noRecoveryWarning": "⚠ There is no recovery. We do not have your key and never will.\n" +
		"Lose it and the synced memory becomes unreadable — it can only be rebuilt from scratch.\n" +
		"\n" +
		"Save a copy now: /key export, then put it in a password manager.\n" +
		"To use the same memory on another machine, run /key import <key> there.",
	"key.exportWarning": "⚠ Below is your PRIVATE key. Anyone holding it can read your synced memory. Do not paste it into chats, issues or screenshots.",
	"key.exportHint":    "Public part (this one is safe to share): %s",
	"key.imported":      "Key installed. Public: %s",
	"key.importFailed": "Could not install the key: %v\n" +
		"\n" +
		"If you meant to replace an existing key, delete it first — but make sure you have\n" +
		"a copy, otherwise everything encrypted with the old key becomes unreadable: %s",
	"key.invalid": "That does not look like a private key (%v). Expected format: AGE-SECRET-KEY-1…",
	"key.error":   "Key operation failed: %v",

	// === /memory — импорт и экспорт памяти ===
	"hint.memory":               "agent memory: import from other agents, export",
	"hint.memoryFind":           "search my memory, and if empty — other agents on this machine",
	"memory.usage":              "/memory — status · /memory import — pull in memory from other agents found here · /memory export [dir] — dump memory as markdown · /memory find <query> — search my memory, then other agents’",
	"memory.findUsage":          "Search for what? Example: /memory find pricing",
	"memory.findMine":           "🧠 Found in my own memory (%d) for \"%s\":",
	"memory.findForeign":        "📁 Nothing in my own memory, but another agent on this machine has it (%d) for \"%s\":",
	"memory.findNothing":        "Nothing found — not in my memory, not in another agent memory: \"%s\"",
	"memory.findImportQuestion": "Take this into my memory? (%d file(s))",
	"memory.status":             "Memory: %d entries in %s",
	"memory.foundNearby":        "Found %d file(s) of other agents' memory in this directory:",
	"memory.importHint":         "\nRun /memory import to pull them in.",
	"memory.nothingToImport":    "No memory files of other agents found in %s.\nLooked for: CLAUDE.md, .claude/, AGENTS.md, .cursorrules, .cursor/rules/, .github/copilot-instructions.md, EXECAI.md",
	"memory.importQuestion":     "Import %d file(s) into the agent's memory?",
	"memory.importYes":          "Import",
	"memory.importYesDesc":      "Contents become part of memory — and memory is later synced to the server (encrypted with your key)",
	"memory.importNo":           "Cancel",
	"memory.importNoDesc":       "Nothing is read or copied",
	"memory.importCancelled":    "Import cancelled — nothing was copied.",
	"memory.imported":           "Imported: %d",
	"memory.exported":           "Exported %d file(s) to %s",
	"memory.error":              "Memory operation failed: %v",

	// === /project — привязка каталога к проекту ===
	"hint.project":              "bind this directory to a project from the web chat",
	"project.usage":             "/project — list projects · bind <name> — bind this directory · unbind — unbind · on/off — enable/disable the agent in the project",
	"project.listHeader":        "Your projects (● bound to this directory, ○ bound elsewhere):",
	"project.listHint":          "Bind: /project bind <name>",
	"project.none":              "You have no projects yet — create one in the web chat.",
	"project.defaultTag":        "[default]",
	"project.bound":             "Project «%s» bound to %s",
	"project.unbound":           "Binding removed from %s",
	"project.notBound":          "No project is bound to %s",
	"project.notFound":          "Project «%s» not found. Available: %s",
	"project.needLogin":         "an ExecAI account is required — /login",
	"project.error":             "Project operation failed: %v",
	"project.serveQuestion":     "Start the background listener to accept tasks from the web chat?",
	"project.serveYes":          "Start",
	"project.serveYesDesc":      "tools follow your permissions.json; survives closing the terminal",
	"project.serveReadOnly":     "Read-only",
	"project.serveReadOnlyDesc": "may look, may not change files or run commands",
	"project.serveNo":           "Not now",
	"project.serveNoDesc":       "later by hand: execai serve",
	"project.serveStarted":      "▶ Listener started (pid %d). Status: execai serve --status · stop: --stop",
	"project.serveLog":          "  output: %s",
	"project.serveSkipped":      "Listener not started — web tasks will wait in the queue. Start it: execai serve",
	"project.serveFailed":       "Could not start the listener: %v",
	"project.serveAlready":      "▶ Listener is already running (pid %d)",
	"project.agentOn":           "[agent on]",
	"project.agentOff":          "[agent OFF]",
	"project.enabled":           "Agent «%s» enabled in project «%s» — it will pick up tasks from there",
	"project.disabled":          "Agent «%s» disabled in project «%s» — it will not pick up tasks from there",
	"project.notAgent":          "This session is not an agent, so there is nothing to add. Log in with /login on the machine itself.",
	"project.notInProject":      "The agent is not part of project «%s» — run /project bind first",
	"project.boundNoTool":       "Directory %[2]s is bound to project «%[1]s», but adding the agent to the project failed: %[3]v",
}

func init() {
	i18n.Register("en", enMiscMessages)
}
