# Переезд с другого агента за минуту

Пришли из Claude Code, Cursor, Codex CLI, Windsurf, Gemini CLI или GitHub
Copilot? Инструкции, которые вы писали для них, execai подхватит сам —
заново объяснять проект не нужно.

## 1. Установка

Linux / macOS:

```bash
curl -fsSL https://raw.githubusercontent.com/execai/execai-agent/main/install.sh | bash
```

Зеркало в РФ (обычно быстрее):

```bash
curl -fsSL https://storage.yandexcloud.net/execai-agent-prod/execai/stable/install.sh | MIRROR=yandex bash
```

Windows 10/11 (PowerShell, лучше в Windows Terminal):

```powershell
iwr -useb https://storage.yandexcloud.net/execai-agent-prod/execai/stable/install.ps1 | iex
```

Затем в новом терминале откройте проект:

```bash
cd ~/work/my-project
execai
```

Первый запуск предложит вход: покажет ссылку подтверждения и откроет её в
браузере. `execai login --token <jwt>` — вход для скриптов. Вход и подписка
ExecAI нужны только для моделей ExecAI (любой план, первые 3 дня за 1 ₽).
Со своими ключами и локальными моделями можно работать без аккаунта: прямо
на экране входа наберите `/connect <провайдер> <ключ>` и
`/source <провайдер>` (или `/source ollama`).

## 2. Что подхватывается

| Откуда | В проекте | Вне проекта |
|---|---|---|
| Claude Code | `CLAUDE.md`, `CLAUDE.local.md`, `.claude/CLAUDE.md`, `.claude/memory/*.md` | `~/.claude/CLAUDE.md`; память проекта `~/.claude/projects/<проект>/memory/*.md` |
| Cursor | `.cursorrules`, `.cursor/rules/*.mdc` и `*.md` | — |
| Codex CLI | `AGENTS.md` | `~/.codex/AGENTS.md`, `~/.codex/instructions.md` (или `$CODEX_HOME`) |
| Windsurf | `.windsurfrules`, `.windsurf/rules/*.md` | `~/.codeium/windsurf/memories/global_rules.md` |
| Gemini CLI | `GEMINI.md` | `~/.gemini/GEMINI.md` |
| GitHub Copilot | `.github/copilot-instructions.md`, `.github/instructions/*.md` | — |

Папки читаются на один уровень, без вложенных. Каждый файл становится
отдельной записью в памяти execai:

| ОС | Память execai |
|---|---|
| Linux | `~/.config/execai/memory/` |
| macOS | `~/Library/Application Support/execai/memory/` |
| Windows | `%APPDATA%\execai\memory\` |

В записи сохраняется, откуда она взята, а строка в индексе `MEMORY.md`
говорит агенту, о чём файл и из какого он проекта. Исходные файлы не
меняются. Повторный импорт того же файла ничего не дублирует.

Не берутся:

- пустые файлы и файлы больше 256 КБ;
- файлы, где есть что-то похожее на ключ, токен или пароль. Такой файл
  пропускается с номером строки — уберите секрет и повторите.

## 3. Как импортировать

**Само, при первом запуске.** Если в каталоге проекта нашлись такие файлы,
а у execai для этого проекта ещё нет памяти (`.execai/EXECAI.md` или `EXECAI.md` в корне), агент один раз
покажет список с путями и спросит. Enter — импортировать, Esc или «Нет» —
ничего не трогать. Ответ запоминается для каталога
(`~/.config/execai/projects.json`), повторно вопрос не появится. В домашнем
каталоге и там, где нашлись только глобальные файлы, агент не спрашивает.

**В чате:**

```
/memory import          показать находки и импортировать после подтверждения
/memory find <запрос>   искать в своей памяти, а если пусто — в памяти Claude Code на этой машине
/memory export [папка]  выгрузить память execai обычными markdown-файлами
```

**Из терминала или CI:**

```bash
execai import --dry-run              # что и куда ляжет; ничего не пишет
execai import --from claude,cursor   # только эти источники
execai import -y                     # без вопроса
```

`--from`: `claude`, `cursor`, `codex`, `windsurf`, `gemini`, `copilot` или
`all` (по умолчанию). В терминале команда спрашивает `[Y/n]`; без терминала
(CI) вопросов не задаёт.

Пример `--dry-run`:

```
Найдено файлов других агентов: 4. Память execai: /home/me/.config/execai/memory
  + .cursorrules — Cursor → imported_app_cursorrules.md
      Отвечай кратко.
  + CLAUDE.md — Claude Code → imported_app_claude.md
      Проект на Go. Тесты: make test
  + ~/.codex/AGENTS.md — Codex CLI (глобальные инструкции) → imported_codex_global.md
      Комментарии в коде — по-английски
  ~ ~/.codex/instructions.md — Codex CLI (глобальные инструкции, старый формат): похоже на ключ, токен или пароль (строка 1) — уберите секрет из файла и повторите
Не переносятся: ключи провайдеров (подключите заново: /connect), разрешения, MCP-серверы.
Пробный прогон: ничего не записано. Импортировать: execai import
```

Импортированные записи попадают в контекст строкой индекса, целиком агент
читает их, когда они нужны. Если инструкции проекта должны быть в контексте
всегда и целиком, положите их в `EXECAI.md` в корне проекта — он читается в
каждом ходе. Например, `ln -s CLAUDE.md EXECAI.md` или просто копия.

## 4. Что не переносится

**Ключи и вход провайдеров.** execai не читает чужие файлы авторизации,
настройки и переменные окружения. Свои ключи подключаются в чате заново —
вход в ExecAI для этого не нужен, подписка тоже: она нужна только для
моделей ExecAI.

```
/connect zai <ключ>
/connect kimi <ключ>
/connect openai <ключ>
/connect anthropic <ключ>
/connect openrouter <ключ>
/connect custom          свой OpenAI- или Anthropic-совместимый адрес, по шагам
/source <имя>            переключиться на подключённый источник
```

Подписку Claude или ChatGPT можно использовать через установленные `claude`
и `codex`: `/connect claude-cli`, `/connect codex-cli` — они работают со
своим собственным входом, подписка ExecAI для них не нужна. Локальные
модели — `/connect ollama local`. Ключи остаются на машине
(`subscriptions.json` в каталоге конфига execai).

**Разрешения.** Списки разрешённых команд других агентов не переносятся. В
execai кнопка «Всегда» при подтверждении запоминает разрешение в
`permissions.json`, общий уровень строгости — `/security light|deep|paranoid`.

**MCP-серверы.** execai пока не поддерживает MCP, конфиги MCP других агентов
не читаются. Если вы работаете через `/source claude-cli`, серверы,
настроенные в самом Claude Code, использует claude; на их вызовы execai
спрашивает разрешение.

**Остальное.** История сессий, логи, `settings.json`, команды, хуки и
субагенты Claude Code (`.claude/commands`, `.claude/agents`) не читаются.
Правила из настроек Cursor (User Rules) хранятся не в файлах — перенесите их
вручную, например в `EXECAI.md`.

## 5. Проверить на пустом каталоге

```bash
mkdir -p /tmp/try-execai && cd /tmp/try-execai
printf 'Проект на Go. Тесты: make test\n' > CLAUDE.md
printf 'Отвечай кратко.\n' > .cursorrules
execai import --dry-run
```
