// Package messages/ru_import — Russian strings for importing other agents'
// instructions and memory (/memory import, the first-run offer, execai
// import). Merged into the "ru" catalog.
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var ruImportMessages = map[string]string{
	"import.kind.memory":        "память",
	"import.kind.rules":         "правила",
	"import.kind.personal":      "личные настройки",
	"import.kind.global":        "глобальные инструкции",
	"import.kind.globalLegacy":  "глобальные инструкции, старый формат",
	"import.kind.agentsStd":     "общий стандарт: Codex CLI и другие",
	"import.kind.pathRules":     "инструкции по путям",
	"import.kind.projectMemory": "проектная память",

	"import.skip.secret":  "похоже на ключ, токен или пароль (строка %d) — уберите секрет из файла и повторите",
	"import.skip.already": "уже импортировано (%s)",
	"import.skip.exists":  "в памяти уже есть %s — не перезаписываю",
	"import.skip.read":    "не прочитать: %v",
	"import.skip.write":   "не записать: %v",

	"import.badFrom":        "неизвестный источник %q для --from; можно: %s или all",
	"import.fm.description": "Импортировано из %s: %s",
	"import.indexLine":      "- [📥 %s](%s) — %s · импортировано из %s: %s",

	"import.offer.question": "Найдены настройки других агентов: %s. Импортировать их в память execai?\nСпрашиваю один раз для этого проекта.\n\n%s",
	"import.offer.more":     "  … и ещё %d",
	"import.offer.yes":      "Импортировать",
	"import.offer.yesDesc":  "копии лягут в память execai, исходные файлы не меняются",
	"import.offer.no":       "Нет",
	"import.offer.noDesc":   "ничего не копирую и больше не спрашиваю здесь",
	"import.offer.declined": "Хорошо, ничего не трогаю. Импорт в любой момент: /memory import или execai import --dry-run",

	"import.cmd.short": "Перенести инструкции и память других агентов (Claude Code, Cursor, Codex CLI…)",
	"import.cmd.long": "Ищет инструкции и память других агентов в текущем каталоге и в домашнем:\n" +
		"  Claude Code — CLAUDE.md, CLAUDE.local.md, .claude/CLAUDE.md, ~/.claude/CLAUDE.md,\n" +
		"                память проекта в ~/.claude/projects/<проект>/memory/\n" +
		"  Cursor      — .cursorrules, .cursor/rules/\n" +
		"  Codex CLI   — AGENTS.md, ~/.codex/AGENTS.md, ~/.codex/instructions.md ($CODEX_HOME учитывается)\n" +
		"  Windsurf    — .windsurfrules, .windsurf/rules/, ~/.codeium/windsurf/memories/global_rules.md\n" +
		"  Gemini CLI  — GEMINI.md, ~/.gemini/GEMINI.md\n" +
		"  Copilot     — .github/copilot-instructions.md, .github/instructions/\n\n" +
		"и копирует их в память execai. Исходные файлы не меняются, уже импортированное\n" +
		"повторно не копируется, файлы с чем-то похожим на ключ или пароль пропускаются.\n" +
		"Настройки, сессии, логи и ключи других агентов не читаются.\n\n" +
		"В терминале спрашивает подтверждение; без терминала (CI) вопросов не задаёт.\n\n" +
		"Примеры:\n" +
		"  execai import --dry-run\n" +
		"  execai import --from claude,cursor\n" +
		"  execai import -y",
	"import.cmd.flagDryRun":     "только показать, что и куда будет импортировано; ничего не записывать",
	"import.cmd.flagFrom":       "откуда брать: claude, cursor, codex, windsurf, gemini, copilot или all (можно через запятую)",
	"import.cmd.flagYes":        "не спрашивать подтверждение",
	"import.cmd.nothing":        "Ни в %s, ни в домашнем каталоге инструкций других агентов не найдено.",
	"import.cmd.found":          "Найдено файлов других агентов: %d. Память execai: %s",
	"import.cmd.notTransferred": "Не переносятся: ключи провайдеров (подключите заново: /connect), разрешения, MCP-серверы.",
	"import.cmd.dryRunDone":     "Пробный прогон: ничего не записано. Импортировать: execai import",
	"import.cmd.nothingNew":     "Нового для импорта нет.",
	"import.cmd.confirm":        "Импортировать %d файл(ов)? [Y/n] ",
	"import.cmd.cancelled":      "Отменено — ничего не скопировано.",
	"import.cmd.done":           "Импортировано: %d",
}

func init() { i18n.Register("ru", ruImportMessages) }
