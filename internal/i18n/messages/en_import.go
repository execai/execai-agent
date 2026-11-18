// Package messages/en_import — English strings for importing other agents'
// instructions and memory (/memory import, the first-run offer, execai
// import). Merged into the "en" catalog.
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var enImportMessages = map[string]string{
	"import.kind.memory":        "memory",
	"import.kind.rules":         "rules",
	"import.kind.personal":      "personal",
	"import.kind.global":        "global instructions",
	"import.kind.globalLegacy":  "global instructions, legacy",
	"import.kind.agentsStd":     "shared standard: Codex CLI and others",
	"import.kind.pathRules":     "path-specific instructions",
	"import.kind.projectMemory": "project memory",

	"import.skip.secret":  "looks like a key, token or password (line %d) — remove the secret from the file and retry",
	"import.skip.already": "already imported (%s)",
	"import.skip.exists":  "%s already exists in memory — not overwriting",
	"import.skip.read":    "cannot read: %v",
	"import.skip.write":   "cannot write: %v",

	"import.badFrom":        "unknown --from source %q; use %s or all",
	"import.fm.description": "Imported from %s: %s",
	"import.indexLine":      "- [📥 %s](%s) — %s · imported from %s: %s",

	"import.offer.question": "Found settings of other agents: %s. Import them into execai's memory?\nAsked once for this project.\n\n%s",
	"import.offer.more":     "  … and %d more",
	"import.offer.yes":      "Import",
	"import.offer.yesDesc":  "copies go into execai's memory, the original files stay as they are",
	"import.offer.no":       "No",
	"import.offer.noDesc":   "copy nothing and don't ask again here",
	"import.offer.declined": "OK, nothing touched. Import any time: /memory import or execai import --dry-run",

	"import.cmd.short": "Bring over instructions and memory of other agents (Claude Code, Cursor, Codex CLI…)",
	"import.cmd.long": "Looks for other agents' instructions and memory in the current directory and the home one:\n" +
		"  Claude Code — CLAUDE.md, CLAUDE.local.md, .claude/CLAUDE.md, ~/.claude/CLAUDE.md,\n" +
		"                project memory in ~/.claude/projects/<project>/memory/\n" +
		"  Cursor      — .cursorrules, .cursor/rules/\n" +
		"  Codex CLI   — AGENTS.md, ~/.codex/AGENTS.md, ~/.codex/instructions.md ($CODEX_HOME respected)\n" +
		"  Windsurf    — .windsurfrules, .windsurf/rules/, ~/.codeium/windsurf/memories/global_rules.md\n" +
		"  Gemini CLI  — GEMINI.md, ~/.gemini/GEMINI.md\n" +
		"  Copilot     — .github/copilot-instructions.md, .github/instructions/\n\n" +
		"and copies them into execai's memory. The original files are not changed, what was\n" +
		"imported before is not copied again, files with anything like a key or password are skipped.\n" +
		"Other agents' settings, sessions, logs and keys are never read.\n\n" +
		"In a terminal it asks for confirmation; without one (CI) it asks nothing.\n\n" +
		"Examples:\n" +
		"  execai import --dry-run\n" +
		"  execai import --from claude,cursor\n" +
		"  execai import -y",
	"import.cmd.flagDryRun":     "only show what would be imported and where; write nothing",
	"import.cmd.flagFrom":       "where from: claude, cursor, codex, windsurf, gemini, copilot or all (comma-separated is fine)",
	"import.cmd.flagYes":        "do not ask for confirmation",
	"import.cmd.nothing":        "No instructions of other agents found in %s or in the home directory.",
	"import.cmd.found":          "Files of other agents found: %d. execai memory: %s",
	"import.cmd.notTransferred": "Not carried over: provider keys (connect again: /connect), permissions, MCP servers.",
	"import.cmd.dryRunDone":     "Dry run: nothing written. To import: execai import",
	"import.cmd.nothingNew":     "Nothing new to import.",
	"import.cmd.confirm":        "Import %d file(s)? [Y/n] ",
	"import.cmd.cancelled":      "Cancelled — nothing copied.",
	"import.cmd.done":           "Imported: %d",
}

func init() { i18n.Register("en", enImportMessages) }
