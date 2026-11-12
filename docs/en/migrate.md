# Moving from another agent in a minute

Coming from Claude Code, Cursor, Codex CLI, Windsurf, Gemini CLI or GitHub
Copilot? execai picks up the instructions you wrote for them — no need to
explain the project again. The full guide in Russian:
[docs/ru/migrate.md](../ru/migrate.md).

## Install

```bash
# Linux / macOS
curl -fsSL https://raw.githubusercontent.com/execai/execai-agent/main/install.sh | bash
# Yandex mirror (faster in Russia/CIS)
curl -fsSL https://storage.yandexcloud.net/execai-agent-prod/execai/stable/install.sh | MIRROR=yandex bash
```

```powershell
# Windows 10/11
iwr -useb https://storage.yandexcloud.net/execai-agent-prod/execai/stable/install.ps1 | iex
```

Then run `execai` in your project: the first start offers sign-in (a
confirmation link opened in the browser); `execai login --token <jwt>` is for
scripts. Sign-in and an ExecAI plan are needed only for the ExecAI models (any
plan, the first 3 days cost 1 ₽). Your own keys and local models work without
an account: type `/connect <provider> <key>` and `/source <provider>` (or
`/source ollama`) right on the sign-in screen.

## What is picked up

| From | In the project | Outside the project |
|---|---|---|
| Claude Code | `CLAUDE.md`, `CLAUDE.local.md`, `.claude/CLAUDE.md`, `.claude/memory/*.md` | `~/.claude/CLAUDE.md`; project memory `~/.claude/projects/<project>/memory/*.md` |
| Cursor | `.cursorrules`, `.cursor/rules/*.mdc` and `*.md` | — |
| Codex CLI | `AGENTS.md` | `~/.codex/AGENTS.md`, `~/.codex/instructions.md` (or `$CODEX_HOME`) |
| Windsurf | `.windsurfrules`, `.windsurf/rules/*.md` | `~/.codeium/windsurf/memories/global_rules.md` |
| Gemini CLI | `GEMINI.md` | `~/.gemini/GEMINI.md` |
| GitHub Copilot | `.github/copilot-instructions.md`, `.github/instructions/*.md` | — |

Each file becomes its own record in execai's memory (`~/.config/execai/memory/`
on Linux) with its origin recorded; the originals are not changed and
re-importing does not duplicate. Empty files, files over 256 KB and files
containing anything that looks like a key, token or password are skipped.

## How

- **Automatically on first start**: in a project with such files and no
  execai project memory (`.execai/EXECAI.md` or a root `EXECAI.md`) yet, the
  agent lists them and asks once.
  The answer is remembered per directory (`projects.json`); declining touches
  nothing.
- **In the chat**: `/memory import`, `/memory find <query>`,
  `/memory export [dir]`.
- **From a shell or CI**:
  `execai import --dry-run`, `execai import --from claude,cursor`,
  `execai import -y`. `--from` takes `claude`, `cursor`, `codex`, `windsurf`,
  `gemini`, `copilot` or `all`. It asks `[Y/n]` in a terminal and nothing in CI.

## Not carried over

- **Provider keys and logins** — connect them again (no ExecAI sign-in
  needed): `/connect zai|kimi|openai|anthropic|openrouter <key>`,
  `/connect custom`, then `/source <name>`. Claude or ChatGPT subscriptions
  work through the installed CLIs: `/connect claude-cli`, `/connect codex-cli`;
  local models — `/connect ollama local`. Your own sources need no ExecAI
  plan; the plan is only for the ExecAI models.
- **Permissions** — use the “Always” button and `/security light|deep|paranoid`.
- **MCP servers** — execai does not support MCP yet; other agents' MCP configs
  are not read.
- Session history, logs, `settings.json`, Claude Code commands/hooks/subagents,
  Cursor User Rules (kept in Cursor's settings, not in files).
