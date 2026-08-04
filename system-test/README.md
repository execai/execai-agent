# system-test — E2E для agent-vbai (CLI)

## Автоматизация (auto/)

Один вход — `./system-test/auto/run.sh [unit|smoke|pty|all]`:

- **unit** — `go test ./...` — unit-тесты во всех internal-пакетах.
  Быстро (<30 сек). Покрывает:
  - `internal/chat/subs_commands_test.go` — переключение source (BUG-1, BUG-2 regression, все 5 источников)
  - `internal/auth/agent_link_test.go` — device-flow polling с mock-бэком
    (happy path, транзиентные ошибки, timeout, ctx-cancel, тики UI)
- **smoke** — `go run ./cmd/syscheck` — реальные запросы во все подключенные подписки
- **pty** — PTY-based TUI-тест (TODO: bubbletea alt-screen плохо читается,
  файл под build-тегом `ptytest`)

Что запускать перед промоушеном R5 → prod:
```bash
./system-test/auto/run.sh all
```

## Отладка

При проблемах с device-flow login смотри `~/.config/execai/auth-poll.log`
— туда логируется что реально возвращает бэк (`status=pending`, `ERR: ...`).

## Ручные сценарии (scenarios/)

Ручные сценарии для проверки CLI агента целиком: подписки, source/model
switching, streaming, tools, autoloop, /loop, vision, memory. **Реальные
запросы к провайдерам, реальный биллинг ExecAI.** Никаких mock'ов.

## Философия (та же что в [[aiguide-vbai/system-test]])

Solo founder, время > деньги. Тесты долгие (~15-30 мин полный прогон) но
ловят то что юнит-тесты не словят:

- Переключение source/model в TUI — стейт-машина рвётся между разными подписками
- Стриминг (text/thinking deltas, tool calls) во всех 4-х source: ExecAI/Z.ai/Anthropic/claude-cli
- Tool permissions UI (Раз/Сессия/Команда/Всегда/Отклонить)
- Autoloop через schedule_wakeup
- /loop фиксированный
- vision (drag/Ctrl+V, multi-image)
- session continuity между запусками
- /thinking picker и Shift+Tab
- Status bar: src/provider/model/think синхронны

Стоимость прогона: ~10₽ (несколько запросов к каждому source).

## Запуск

Прогон руками **перед каждым промоушеном** R5 → execai-agent-prod. Если
сценарий упал — багу в `bugs/open/` и в этом README отметить.

```bash
cd /home/yz/velesbsd/execai/agent-vbai
# опционально — пересобрать локально из R5
go build -o /tmp/execai ./cmd/execai
/tmp/execai
```

Или прод-бинарь после промоушена:
```bash
curl -fsSL https://storage.yandexcloud.net/execai-agent-prod/execai/R5/latest/install.sh | bash
execai
```

## Test accounts

```
ExecAI:        alort5@yandex.com · YasonTS1   (тот что в /login)
Z.ai:          api-key Coding Plan (хранится в ~/.local/share/agent-vbai/zai-key)
Anthropic:     sk-ant-... из console.anthropic.com (~/.local/share/agent-vbai/anthropic-key)
Claude CLI:    локальный `claude` Pro/Max OAuth (claude --version → 2.1.173+)
```

## Сценарии

| ID    | Файл                                  | Что проверяет                            | ~Время |
|-------|---------------------------------------|------------------------------------------|--------|
| T01   | scenarios/01-source-switching.md      | Переключение источников и моделей        | 5 мин  |
| T02   | scenarios/02-streaming-all-sources.md | Стрим текста+thinking из 4 source        | 5 мин  |
| T03   | scenarios/03-tools-permissions.md     | Bash/Read/Write + approve modal          | 5 мин  |
| T04   | scenarios/04-autoloop-loop.md         | schedule_wakeup + /loop                  | 5 мин  |
| T05   | scenarios/05-vision-multi-image.md    | drag, Ctrl+V, multiple images            | 3 мин  |
| T06   | scenarios/06-session-continuity.md    | Перезапуск CLI продолжает беседу         | 2 мин  |
| T07   | scenarios/07-status-bar-sync.md       | src/provider/model в статус-баре         | 3 мин  |
| T08   | scenarios/08-project-and-serve.md     | Проекты + канал «веб → агент» (WA-лист)  | 15 мин |

## Bug tracker

`bugs/INDEX.md` — компактный список. Файлы `BUG-N-кратко.md` в `open/`
или `closed/`. Закрытие — `git mv open/BUG-N-...md closed/` + заметка в
коммите что чинит.

## Runs

`runs/YYYY-MM-DD-T0N-краткое-имя/` — артефакты прогона: скриншоты
(`screenshot -001.png ... -NNN.png`), copy логов из терминала, выводы
`requests.log`.

## Связь
- [[aiguide-vbai/system-test]] — оригинал на ветке `fatal`
- [[project-subscriptions-arch-zai-first]] — что тестируем
