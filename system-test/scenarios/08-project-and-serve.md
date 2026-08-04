# T08 — Проекты и канал «веб → агент» (WA-лист)

Система: агент как инструмент проекта + задачи из веб-чата.
Цепочка: каталог (integrations) → workspace_tools → tools-vbai (схема функции,
{alias}/enum) → aihandler (вызов с токеном юзера) → agents-vbai `/tasks/run` →
Redis wake → `execai serve` → `/tasks/<id>/result` → ответ модели.

**Как гонять:** DEV-контур (адрес шлюза — во внутренней памяти), учётка агента
из `~/.config/execai/credentials.json`. На машине —
демон `execai serve`. Скрипты прогона лежат в `runs/<дата>-T08-*/` рядом с
артефактами — прогон повторяется ими, не руками.

⚠️ Правило PULSE: ответы модели проверять по истории
(`POST /aichat-vbai/actions {action:get_conversation_messages}`), НЕ грепом
стрима — text_delta дробит текст.

Статусы: ✅ PASS · 🔴 FAIL · ⏭ SKIP · ⚠️ WARN. ⭐ = prod-gate.

| # | Сценарий | Ожидаем |
|---|---|---|
| WA1 ⭐ | CLI: `/project bind <ws>` в каталоге | привязка в agents-vbai + запись `workspace_tools` (tool_id=agent, profile=agent_id, enabled); повторный bind НЕ плодит дубли; `/project` показывает `[агент вкл]` |
| WA2 ⭐ | **Настоящий конвейер**: `POST /aichat-vbai/stream` с `tools:[{tool_id:"agent",alias:<agent_id>}]`, промт «спроси у агента версию ОС» | модель вызывает `run_on_agent`, демон исполняет, финальный ответ в ИСТОРИИ беседы содержит данные с машины; ровно 1 задача |
| WA3 ⭐ | Тумблер OFF в активном проекте → `/tasks/run` | отказ сразу («агент выключен…»), задача НЕ создана; после ON — работает |
| WA4 ⭐ | 2 проекта на машине, разные каталоги; задача «pwd» | выполняется в каталоге **активного** проекта (active-workspace), не в каталоге демона и не в первом попавшемся |
| WA5 ⭐ | Security: (a) agent, не привязанный к активному проекту → отказ; (b) пустая задача → 400; (c) без agent → 400; (d) `POST /tasks/<id>/result` от МАШИНЫ, которой задача не адресована → **403**; (e) бинд браузерной сессией без agent_id → 400 (юнит resolveAgentID) |
| WA6 ⭐ | Демон остановлен → `/tasks/run` | «агент не отвечает» ≤15с, задача pending; старт `serve` → подхват БЕЗ повторного run, результат в БД |
| WA7 | Деградация: integrations 500 → проектные задачи отложены (pending), личные идут; Redis down → short-poll fallback | покрыто юнитами (`TestAllowedByWorkspace_*`); живьём на общем DEV не гоняем — ломать разделяемую среду нельзя |
| WA8 | UI: чипы/список показывают **alias**, не uuid; запись без машины ≠ машина; пустой список = инструкция, не ошибка | vitest `agentNames` (passthrough ssh/git) + глазами владельца |
| WA9 | Долгая задача (> RunTimeout 110с) | run возвращает «не уложился…» с task_id; демон ДОДЕЛЫВАЕТ задачу, результат в БД появляется позже |
| WA11 ⭐ | Эксплуатация демона: `--status` без запуска → «не запущен» + код 1; запуск; `--status` → pid/время/контур + код 0; вторая копия → отказ с подсказкой; `--stop` → мягкая остановка, pid-файл убран; зависший процесс: `--stop` отказывается и подсказывает, `--stop --force` добивает через 5с и чистит pid-файл; `--force` без `--stop` → ошибка; протухший pid-файл (kill -9) НЕ блокирует запуск; журнал ротируется в `.1` | все пункты |
| WA10 | Две машины в проекте → alias-список | tools-vbai `_inject_alias_enum`: enum по машинам, default=первая; модель вызывает существующую машину, вызов исполняется |

**Prod-gate:** WA1–WA6 + WA11 зелёные. WA7 — юнитами. WA8/WA10 — до выпуска функции
из DEV.

**Регресс по impact-map** (гонять при изменении этих файлов):
- `integrations-vbai/app/routes/catalog.py`, `config.py` → смоук: available-tools
  отдаёт ssh/git с has_profiles; CRUD workspace_tools жив. Флаг
  `AGENT_TOOL_ENABLED` — юнитом в обе стороны.
- `execaiui` label-композиция (ChatLayout / ChatV2Layout / SettingsPanel /
  WelcomeScreen) → подписи ssh/git НЕ изменились (vitest agentNames
  passthrough) + глазами чипы в шапке.
- `agents-vbai/internal/toolset` → `go test ./internal/toolset/` (схема функции:
  endpoint-префикс, {alias}, обязательность параметров).

## Журнал прогонов
| Дата | Против | Результат | Артефакты |
|---|---|---|---|
| 2026-08-03 STAGE | agents-vbai **R6.16** · integrations R2.6 · aicore R5.14 | WA1/WA3/WA5/WA6 ✅ · **WA2 🟡 частично** · WA4/WA8/WA9/WA10 ⏭ — блокер: aicore R5.14 на стейдже не регистрирует маршруты (вызова нет в коде тега, фикс 30.07 не доехал) | `runs/2026-08-03-T08-web-agent-channel/report-stage.md` |
| 2026-08-03 DEV | agents-vbai **R6.15** (`68483a6`) · agent-vbai R6 `1b42c8b` · integrations R2.13 · execaiui R5.179 | **prod-gate WA1–WA6 ✅** (WA2/WA6 — со 2-го захода: найдены и закрыты баги №4 SSE-контракт и №5 потеря задачи зомби-poll'ом) · WA7 юнитами · WA8 программно · WA9/WA10 ✅ | `runs/2026-08-03-T08-web-agent-channel/` |
