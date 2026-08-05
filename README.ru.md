[English](README.md) | **Русский** | [Español](README.es.md) | [Deutsch](README.de.md) | [中文](README.zh.md)

🌐 **Сайт:** [execai.ru](https://execai.ru) · 💬 Веб-чат: [chat.execai.ru](https://chat.execai.ru)

---

# execai — терминальный AI-агент

CLI-агент в духе Claude Code, который реально работает на твоей машине: читает файлы, запускает команды, ходит в kubernetes, отвечает в чате. Поддерживает **10 источников** — наш ExecAI + твои подписки Z.ai / Kimi Code / Anthropic / OpenAI + локальные Claude Code / OpenAI Codex CLI / Ollama (cloud или local). Между ними переключается на лету общей историей.

```
execai R5.136 · openai/gpt-5 · it@execai.ru · ~
Введите задачу. /model — модели, /help — команды, /quit — выход.

› проверь свободное место на сервере dev01
```

![demo](scripts/demo.gif)

> **ℹ Аккаунт ExecAI опционален.** Дефолтный источник `execai` использует наш бэкенд (регистрация на [execai.ru](https://execai.ru)), но CLI полностью работает **без входа в ExecAI**: подключи свою подписку прямо с логин-экрана — `/connect kimi <key>`, `/connect zai <key>`, `/connect openai <key>`, `/connect anthropic <key>` или локальные `claude-cli` / `codex-cli` / `ollama` — затем `/source <provider>` и работай.

**Ink-style рендеринг** (по умолчанию, R5.107+): история сообщений уходит в терминальный scrollback (native selection работает, не сбрасывается при обновлениях). Live-стрим и input — в динамической области внизу. Как Claude Code.

---

## Содержание

1. [Установка](#установка)
2. [Первый запуск](#первый-запуск)
3. [Как разговаривать с агентом](#как-разговаривать-с-агентом)
4. [Источники — ExecAI и подписки](#источники--execai-и-подписки)
5. [Модели](#модели)
6. [Картинки и файлы](#картинки-и-файлы)
7. [Effort — уровень рассуждения](#effort--уровень-рассуждения)
8. [Loop и Autoloop — повторение и ожидание](#loop-и-autoloop--повторение-и-ожидание)
9. [Память агента (EXECAI/MEMORY.md)](#память-агента)
10. [Проекты и фоновый режим — веб → агент](#проекты-и-фоновый-режим--веб--агент)
11. [Расходы / `/usage`](#расходы--usage)
12. [Команды и хоткеи](#команды-и-хоткеи)
13. [Сессии и история](#сессии-и-история)
14. [Где живут файлы](#где-живут-файлы)
15. [Если что-то сломалось](#если-что-то-сломалось)

---

## Установка

### Linux / macOS (Intel и Apple Silicon)

```bash
curl -fsSL https://storage.yandexcloud.net/execai-agent-prod/execai/stable/install.sh | bash
```

Для русских локалей скрипт сам качает бинарь с **зеркала Яндекса** (быстрее в РФ/СНГ). Вне России — GitHub Releases:

```bash
curl -fsSL https://raw.githubusercontent.com/execai/execai-agent/main/install.sh | bash
```

Скрипт:
- определит архитектуру (`linux-amd64`, `linux-arm64`, `darwin-amd64`, `darwin-arm64`),
- проверит SHA256,
- положит бинарь в `/usr/local/bin/execai` (passwordless sudo) или `~/.local/bin/execai`,
- уберёт старые копии execai из PATH и мёртвые/`/tmp/execai*` записи из rc-файлов,
- на macOS сам снимет `com.apple.quarantine` (Gatekeeper).

После установки — в **новом терминале** просто `execai`. В текущем — если раньше уже был `execai` и bash закешировал старый путь, поможет `hash -r`.

### Windows 10/11 (amd64 и ARM64)

```powershell
iwr -useb https://storage.yandexcloud.net/execai-agent-prod/execai/stable/install.ps1 | iex
```

Автоопределит архитектуру (amd64 / arm64 для Copilot+ PC), попросит **UAC** один раз чтобы добавить папку в исключения Defender'а (иначе антивирус может съесть бинарь), скачает `execai.exe` в `%LOCALAPPDATA%\execai\`, пропишет в User PATH.

> Запускай через **Windows Terminal** (не старый `cmd.exe`/conhost) — там TUI (bubbletea) отрендерится нормально.

После установки проверь:
```bash
execai version
# execai R5.NN
```

### Установить конкретную версию

```bash
curl -fsSL https://raw.githubusercontent.com/execai/execai-agent/main/install.sh | VERSION=5.136 bash
```

### Скачать вручную

Если предпочитаешь взять бинарь сам без установщика — все релизы с чексуммами на GitHub:

**https://github.com/execai/execai-agent/releases/latest**

Выбери архив под свою платформу (`execai-{linux,darwin,windows}-{amd64,arm64}.{tar.gz,zip}`), сверь с `SHA256SUMS`, распакуй, положи в PATH.

---

## Первый запуск

```bash
execai
```

Без аргументов открывается чат-TUI.

**Если ты ещё не залогинен** — агент покажет ссылку для подтверждения в браузере и автоматически откроет её:

```
👉 Открой в браузере и подтверди что это твой агент:
   https://chat.execai.ru/agents/connect/U7XQ9F4P

(жди, поллю каждые 3 сек… как подтвердишь — продолжим)
```

В браузере:
1. Если не залогинен в ExecAI — войди (Яндекс / VK / e-mail)
2. На странице "Подключение агента" — поле alias'а автоматически заполнено твоим hostname'ом. Можешь принять или переопределить (например "yz-laptop"). Нажми "Подтвердить".

Агент сразу получит JWT и продолжит. Токен живёт 90 дней, авто-продлевается при каждом коннекте.

### Что такое persistent agent

Каждый агент привязан к **стабильному host-id** твоей машины:
- Linux — `/etc/machine-id`
- macOS — `IOPlatformUUID`
- Windows — `HKLM\SOFTWARE\Microsoft\Cryptography\MachineGuid`
- Fallback — `~/.config/execai/host_id` (UUID генерится при первой установке)

Это значит: если ты переустановил execai, удалил `credentials.json` и залогинился заново — **бэк реюзит существующую сессию**, agent_id остаётся тот же. Не плодятся дубликаты в списке твоих устройств.

Посмотреть все свои агенты и отвязать любой — `chat.execai.ru/settings/agents` (в UserMenu слева внизу → «Агенты», иконка терминала).

---

## Как разговаривать с агентом

Просто пиши задачу обычным языком:

```
› проверь сколько места занимает /var/log на сервере dev01
```

Агент сам:
- разберёт что тебе нужно,
- решит какой tool использовать (Bash, Read, Grep, …),
- покажет команду до выполнения и спросит подтверждения если она опасная,
- выполнит, прочитает результат, ответит.

### Подтверждение опасных команд

Read-only вещи (`ls`, `cat`, `git status`, `kubectl get`, `grep`) выполняются автоматически. Команды что меняют состояние (`rm`, `>`, `chmod`, `kubectl delete`, `git push`) — спросят:

```
⚠  Подтвердите запуск Bash
   kubectl delete pod foo-xxx
   
   [y] Разово  [a] Весь Bash в сессии  [s] Эту команду в сессии  [f] НАВСЕГДА  [n] Отклонить
```

| Кнопка | Что |
|---|---|
| **y** | Разрешить ОДИН раз |
| **a** | Разрешить ВСЕ вызовы Bash до перезапуска execai |
| **s** | Разрешить именно эту команду (включая аргументы) до перезапуска |
| **f** | Разрешить НАВСЕГДА (запишется в `~/.config/execai/permissions.json`) |
| **n** или Esc | Отклонить — агент получит "пользователь отказал" |

Управление: ← → или Tab — переключить фокус, Enter — подтвердить, либо горячие клавиши `y/a/s/f/n`.

### Список встроенных инструментов

| Tool | Что делает |
|---|---|
| **Bash** | Выполняет shell-команду со streaming-выводом (строки появляются по мере появления) |
| **Read** | Читает файл с offset/limit, автодетект бинарных |
| **Write** | Создаёт/перезаписывает файл (со спросом) |
| **Edit** | Точная замена строки в файле (со спросом) |
| **Grep** | Поиск regexp по дереву файлов |
| **Glob** | Найти файлы по шаблону `**/*.go` |
| **LS** | Листинг директории |
| **Tree** | Дерево файлов до глубины N |
| **WebFetch** | Открывает страницу: HTML → читаемый текст + ссылки для перехода (без JS) |
| **WebSearch** | Поиск в интернете с ответом и списком источников (нужен аккаунт ExecAI) |
| **AskUser** | Задаёт вопрос с 2–4 вариантами, когда решение за вами |
| **Task** | Отдаёт самостоятельную подзадачу субагенту (только чтение) |
| **TodoWrite** | Внутренний планировщик задач самого агента |
| **schedule_wakeup** | AI сам планирует пробуждение через N сек (см. [Autoloop](#loop-и-autoloop--повторение-и-ожидание)) |

---

## Источники — ExecAI и подписки

Агент может ходить через **9 разных источников**. История беседы общая — переключаешься на лету, контекст не теряется.

| Источник | Что | Биллинг |
|---|---|---|
| **execai** (дефолт) | Наш gateway → любой из ~34 моделей | Тариф ExecAI |
| **zai** | Z.ai GLM Coding Plan напрямую | Твоя подписка Z.ai |
| **zai-api** | Z.ai open platform, оплата за токены — **ключ тот же, что у подписки**, но, в отличие от Coding Plan, без ограничений по списку инструментов | Оплата за токены Z.ai |
| **kimi** | Kimi Code Coding Plan (kimi.com/code) | Твоя подписка Kimi Code |
| **kimi-api** | Moonshot Platform pay-per-token | Pay-per-token Moonshot |
| **anthropic** | Прямой Claude API (sk-ant-...) | Pay-per-token Anthropic |
| **openai** | Прямой OpenAI API (sk-proj-...) | Pay-per-token OpenAI |
| **claude-cli** | Делегирование в локальный `claude` CLI | Твоя Claude Pro/Max/Team OAuth |
| **codex-cli** | Делегирование в локальный `codex` CLI | Твоя ChatGPT Plus/Pro/Team OAuth |
| **ollama** | Cloud (ollama.com) или local (`ollama serve`) | Твоя подписка / 0 ₽ локально |

### 1. ExecAI — дефолт

После логина агент использует наш биллинг. Никаких дополнительных настроек.

```
src : ExecAI   provider : anthropic   model : claude-sonnet-4-6
```

### 2. Z.ai Coding Plan

Своя подписка $3-60/мес → ходишь напрямую в GLM-5.2, мы ничего не списываем.

1. Возьми **Coding Plan** ключ (не обычный API-key!):
   - https://z.ai/manage-apikey/apikey-list → **Individual Coding Plan > Plan Overview**
   - Team: https://z.ai/manage-apikey/coding-plan/team/my-plan

   > ⚠️ Обычный API-key биллится pay-per-token, НЕ из подписки.

2. `/connect zai sk-zai-XXXXX`
3. `/source zai` → primary модель **GLM-5.2**

### 3. Kimi Code Coding Plan

Отдельный продукт Moonshot AI — их аналог Claude Code с собственной подпиской. Флагман **K3** доступен на всех тарифах кроме младших.

1. Возьми ключ на https://www.kimi.com/code/console (раздел API-ключи).
2. ```
   /connect kimi sk-XXXXX
   /source kimi
   ```
3. Тариф автоматически определяется через `/coding/v1/models` — в статус-баре покажется `kimi (K3)`, `kimi (K3 + HighSpeed)` или `kimi (K2.7 Code)` в зависимости от того что реально доступно.

Модели: `k3` (primary), `kimi-for-coding` (K2.7 Code), `kimi-for-coding-highspeed`. `/usage` показывает **реальную квоту**: недельный лимит + rolling-окна (5h) с прогресс-барами.

### 4. Moonshot Platform (pay-per-token API)

Обычный API-ключ для оплаты за токены (не подписка). Если нужны модели Moonshot вне Kimi Code:

1. Возьми ключ на https://platform.moonshot.ai/console/api-keys.
2. ```
   /connect kimi-api sk-XXXXX
   /source kimi-api
   ```

Каталог моделей подтягивается динамически из `/v1/models` твоего аккаунта. Primary → `kimi-latest` (auto-alias на актуальный флагман). Ключ валидируется при `/connect` — при 401/403 сразу отказ с подсказкой какой ключ куда.

### 5. Anthropic API

Прямой ключ с https://console.anthropic.com/settings/keys — pay-per-token.

```
/connect anthropic sk-ant-XXXXX
/source anthropic
```
Модели: claude-sonnet-4-6, claude-opus-4-8, claude-haiku-4-5.

### 6. OpenAI Platform (pay-per-token API)

Прямой ключ с https://platform.openai.com/api-keys — pay-per-token.

```
/connect openai sk-proj-XXXXX
/source openai
```

Каталог подтягивается из `/v1/models` твоего аккаунта. Primary → **gpt-5** → o3 → gpt-4.1 → gpt-4o (порядок приоритета).

### 7. Claude Code CLI (Claude Pro/Max/Team OAuth)

Если у тебя уже стоит `claude` (Claude Code) и подписка **Pro/Max/Team** — используй её квоту через локальную OAuth-сессию, без отдельного ключа.

```
/connect claude-cli    # проверит наличие `claude` в PATH
/source claude-cli
```

Модели (алиасы): sonnet / opus / haiku. Управление моделью тоже через `claude config set defaultModel <id>` снаружи.

⚠️ **Ограничения:**
- execai-tools (Bash/Read/Write) через claude-cli НЕ работают — он поднимает свои tools со своим разрешением
- История передаётся как plain-text промт (без session-id)

### 8. OpenAI Codex CLI (ChatGPT Plus/Pro/Team OAuth)

Аналог `claude-cli` для OpenAI. Использует квоту твоей ChatGPT-подписки (Plus/Pro/Team/Enterprise) через локальный `codex` binary.

**Установка codex** (Linux/macOS):
```bash
curl -fsSL https://chatgpt.com/codex/install.sh | sh
codex login    # OAuth в браузере под своим ChatGPT-аккаунтом
```
Windows:
```powershell
powershell -ExecutionPolicy ByPass -c "irm https://chatgpt.com/codex/install.ps1 | iex"
```

**В execai:**
```
/connect codex-cli
/source codex-cli
```

Модели: gpt-5, o3, o4-mini. Ограничения такие же как у claude-cli — execai-tools не работают, codex запускает свои.

### 9. Ollama — cloud или local

Два режима под одной командой:

**Cloud (ollama.com):**
```
/connect ollama <api-key>          # ключ с https://ollama.com/settings/keys
/source ollama
```
Модели на их серверах: **glm-5.2** (primary), qwen3-coder:480b, kimi-k2:1t, deepseek-v3.1:671b, gpt-oss:120b. Anthropic-совместимый endpoint, effort поддерживается.

**Local (свой Ollama):**
```
ollama pull llama3.2               # снаружи, что хочешь запустить
/connect ollama local              # localhost:11434
/connect ollama local http://192.168.1.10:11434   # свой URL
/source ollama
```
Каталог динамически подтягивается через `/api/tags`. 0 ₽. OpenAI-совместимый endpoint.

### Про переключение

- **История общая.** `/source zai` → `/source execai` → `/source ollama` — контекст сохраняется.
- **Биллинг изолирован.** На внешней подписке ExecAI-биллинг НЕ списывает.
- **`/subscriptions`** — что подключено, `/disconnect <provider>` — удалить.
- **`/source` без аргумента** — быстрый picker.

---

## Модели

`/model` без аргумента — список доступных. `/model <id>` или `/model <num>` — переключиться.

Удобнее через автокомплит:
```
/model<Tab>   →  меню всех моделей с фильтром
/model deeps  →  фильтр по подстроке
```

### Что в каталоге (по source)

| Source | Модели |
|---|---|
| **execai** | ~34 модели — Claude/GPT/DeepSeek/Kimi/MiniMax/Qwen/GLM через наш backend |
| **zai** | glm-5.2 (primary), glm-5.2[1m], glm-4.7 |
| **kimi** | k3 (primary), kimi-for-coding (K2.7), kimi-for-coding-highspeed |
| **kimi-api** | kimi-latest (primary), kimi-k2-turbo-preview, moonshot-v1-* (динамически из /v1/models) |
| **anthropic** | claude-sonnet-4-6, claude-opus-4-8, claude-haiku-4-5 |
| **openai** | gpt-5 (primary), o3, gpt-4.1, gpt-4o, o4-mini (динамически из /v1/models) |
| **claude-cli** | sonnet / opus / haiku (алиасы + pinned версии) |
| **codex-cli** | gpt-5, o3, o4-mini |
| **ollama cloud** | glm-5.2, qwen3-coder:480b, kimi-k2:1t, deepseek-v3.1:671b, gpt-oss:120b |
| **ollama local** | что установлено через `ollama pull` (динамически из /api/tags) |

Автопереключение primary при смене source. Цены/описания — в `/model`.

---

## Картинки и файлы

### Картинки

Просто **перетащи** PNG/JPG/GIF/WEBP в окно терминала или **впиши путь в кавычках**:

```
› '/home/user/Pictures/Снимок экрана.png' что тут на картинке?
```

Агент сам:
- найдёт путь в сообщении (поддерживает пробелы и кириллицу в имени, если файл в кавычках),
- закодирует base64,
- отправит как `image_url` в vision-модель,
- получит описание.

**Vision-модели** (умеют видеть): claude-sonnet-4-6, gpt-5.5, glm-5v-turbo, glm-4.5v. Если выбрана не-vision модель — переключись через `/model`.

### Файлы (текст/код)

Просто скажи путь — агент сам прочитает через `Read` tool:

```
› посмотри файл internal/chat/tui.go и предложи рефакторинг
```

Большие файлы читаются по частям. Если AI пропускает важное — попроси конкретнее ("прочитай строки 100-200" или "найди функцию X").

### Прикрепить файл из буфера

Картинки из буфера обмена (Ctrl+V) терминал передать не умеет — это ограничение всех TUI. Drag&drop файлов (терминал вставит путь) работает.

---

## Effort — уровень рассуждения

Поддерживается на Anthropic-совместимых источниках: **zai, kimi, anthropic, ollama-cloud, claude-cli**. Модель сначала "думает" внутри себя, потом отвечает. На сложных задачах даёт кардинально лучший результат, но медленнее и дороже квоты.

### Уровни

```
/effort                      # открыть picker
```

6 позиций: **off** (0) · **low** (1024) · **medium** (4096) · **high** (8192) · **xhigh** (16384) · **max** (32000) токенов.

Управление в picker'е: **← →** выбор, **Enter** применить, **Esc** отмена.

**Shift+Tab** циклит уровни без открытия picker (в некоторых терминалах не работает — тогда `/effort`).

Сохраняется в `~/.config/execai/config.json`, в status bar виден как `effort : high`.

---

## Loop и Autoloop — повторение и ожидание

### `/loop` — простой повторитель

Запустить промт по таймеру:

```
/loop 5m проверь статус CI билда
```

Каждые 5 минут агент сам перепосылает промт. Полезно для polling'а — "ждать пока что-то произойдёт".

```
/loop          # показать текущий статус
/loop stop     # остановить
```

В status bar появится `🔁 loop: 5m`. Работает только пока TUI открыт.

### Autoloop — AI сам решает когда проснуться

Более умно: AI получил инструмент `schedule_wakeup` и САМ решает когда вернуться к задаче.

Пример:
```
› запусти `npm install`, подожди пока закончится и проверь что в логах нет error
```

AI:
1. Запускает `npm install` (streaming в чате)
2. Видит что install займёт ~5 минут
3. Зовёт `schedule_wakeup(delay_seconds=300, reason="жду npm install", prompt_on_wake="проверь логи и ошибки")`
4. Заканчивает текущий ответ
5. **Через 5 минут TUI сам разбудит агента** с промтом "проверь логи и ошибки"
6. AI продолжает с того же места — видит всю историю беседы

В UI это выглядит как:
```
🌙 autoloop: пробуждение через 5m (жду npm install) → промт: "проверь логи и ошибки"
... 5 минут позже ...
› 🌙 [autoloop] проверь логи и ошибки
● ...AI продолжает...
```

Если AI решил что задача готова — просто не зовёт `schedule_wakeup`, autoloop останавливается сам.

**Когда использовать что:**
- `/loop` — фиксированный интервал, ты заранее знаешь сколько ждать
- Autoloop — AI сам разруливает, ты только формулируешь задачу

---

## Память агента

Агент помнит факты между сессиями — как у Claude Code, но проще. Автоматически подгружается в system prompt (только индекс, а не всё сразу) — контекст остаётся лёгким.

### Куда что писать

**User memory** (кросс-проектная, личное про тебя):
```
~/.config/execai/memory/
├── MEMORY.md              ← индекс (всегда в system prompt)
├── user_role.md           ← 1 файл на факт
├── feedback_style.md
├── project_alpha.md
└── reference_jenkins.md
```

Каждый файл — фронтматтер + короткое тело:
```markdown
---
name: user-role
description: senior Go engineer, live in EU
metadata:
  type: user
---

- Пишу на Go, реже Python
- Предпочитаю bubbletea для TUI
- Не использую sudo без крайней нужды
```

В `MEMORY.md` — одна строка-ссылка на файл:
```
- [🎯 Мой стек](user_role.md) — Go, bubbletea, EU timezone
```

**Project memory** (специфика этого репо):
```
<CWD>/EXECAI.md            ← одним файлом
```

### 4 типа записей

| Type | Что | Пример |
|---|---|---|
| **user** | Роль, скиллы, преференции | "Пишу на Go, тимлид" |
| **feedback** | Как хочешь чтобы работал | "Не делай длинных summary. Why: не читаю" |
| **project** | Инициативы, дедлайны, решения | "R5 — новая ветка под подписки, дедлайн Q3" |
| **reference** | Пойнтеры на внешние системы | "Jenkins: jenkins.mycompany.com, дашборды в Grafana" |

### Как этим пользоваться

Просто скажи в чате что запомнить:
```
› запомни что я на Go пишу и предпочитаю bubbletea
```
Модель создаст `user_role.md` (или обновит существующий) + добавит строку в `MEMORY.md`.

```
› проект R5 — новая ветка под подписки/GUI, не мержить в R4
```
Создаст `project_r5.md` в user memory. Если факт про ЭТОТ репо — обновит `EXECAI.md` в CWD.

Модель НЕ засоряет память эпизодическими фактами ("сегодня чинил bug" — это в git log). Пишет только устойчивое.

Работает на **всех source** — system prompt один и тот же.

---

## Проекты и фоновый режим — веб → агент

*С версии v6.17. Веб-часть требует инструмента «Агенты» в каталоге проекта на execai.ru — он включается постепенно.*

Твоя машина может быть **инструментом внутри проекта веб-чата**: один раз привяжи каталог к проекту, запусти фоновый слушатель — и проси модель в браузере делать что-то на твоей машине. Модель зовёт агента как любой другой инструмент проекта (ssh, git), задача выполняется **в каталоге проекта**, ответ возвращается в чат.

### Привязка каталога

```
/project              # твои проекты; ● — привязан к этому каталогу
/project bind <имя>   # привязать текущий каталог + добавить машину в проект
/project on|off       # тот же тумблер, что в карточке проекта в вебе
/project unbind       # снять привязку и убрать машину из проекта
```

Привязка делает два дела: запоминает «этот каталог на этой машине = тот проект» и добавляет в проект обычную запись инструмента — как ssh-профиль, — поэтому машина появляется в карточке проекта с тумблером вкл/выкл. В интерфейсе показываются алиасы машин; стабильный id машины переживает повторный login.

### `execai serve` — фоновый слушатель

```
execai serve
```

Слушает задачи из веб-чата и выполняет их. Что важно:

- Задача выполняется **в каталоге, привязанном к её проекту**, а не там, где запущен `serve`. Каталог пропал → внятная ошибка, а не работа не в том месте.
- **Что разрешено — твой `permissions.json`** (то, что ты отмечал «НАВСЕГДА» в TUI). Пустой файл → разрешено всё, с громким предупреждением при старте.
- **Всё вне списка агент спрашивает прямо в веб-чате** *(с v6.18)*: во время задачи появляется вопрос с той командой, которую агент собирается выполнить, и теми же вариантами, что в TUI — «Разово», «Весь инструмент в этой задаче», «Эту команду в этой задаче», «НАВСЕГДА», «Отклонить». «НАВСЕГДА» записывается в `permissions.json` на машине. Не ответил за ~2 минуты (или вкладка закрыта) — агент получает отказ: молчание никогда не расширяет права.
- `--read-only` — режим «смотреть, но не трогать»: без правок файлов и команд.
- Каждый вызов инструмента пишется в **журнал** `~/.config/execai/serve-audit.log` (ротация на 8 МБ) — видно, что агент делал ночью.
- Один демон на машину (pid-lock). `execai serve --status` — pid/аптайм/контур; `--stop` — мягкая остановка (даёт доделать текущую задачу); `--stop --force` — SIGKILL через 5 с, результат текущей задачи теряется и в чате будет таймаут.
- Агент оффлайн → чат честно получает «агент не отвечает» за ~12 с; задача остаётся в очереди и выполнится, как только `serve` запустится. Выдача подтверждается (ack) — задача, отданная в мёртвое соединение, не теряется.
- Закрыл терминал — процесс умрёт вместе с ним. Чтобы пережил:
  `setsid nohup execai serve > ~/.execai-serve.log 2>&1 &`

В этом режиме выключен AskUser (вопросы-уточнения модели; разрешения — см. выше — приходят в веб-чат), а предел итераций ниже (30 на задачу).

---

## Расходы / `/usage`

```
/usage
```

Показывает разное в зависимости от активного `/source`:

**ExecAI (default):** твой тариф + баланс + 4 окна-лимита (5h/день/неделя/месяц) с прогресс-барами + последние 14 итераций (модель, токены, цена ₽).

**Kimi Code (`/source kimi`):** реальная квота с `api.kimi.com/coding/v1/usages` — тариф по доступным моделям, недельный лимит с временем сброса, rolling-окна (5h и т.п.).

**Moonshot Platform (`/source kimi-api`), OpenAI (`/source openai`), Anthropic, Z.ai:** локальные счётчики токенов + ссылка на дашборд провайдера (реальный биллинг там).

---

## Команды и хоткеи

### Слэш-команды

| Команда | Что делает |
|---|---|
| `/help` | Все команды |
| `/model [id]` | Сменить модель |
| `/source [name]` | Сменить источник (execai/zai/kimi/kimi-api/anthropic/openai/claude-cli/codex-cli/ollama) |
| `/connect <provider> [args...]` | Подключить подписку. Без аргументов — help |
| `/disconnect <provider>` | Отключить подписку |
| `/subscriptions` (или `/subs`) | Список подключений |
| `/usage` | Тариф + расходы (специфично для каждого source) |
| `/effort` | Picker уровня рассуждения (для Anthropic-compat sources) |
| `/max-iterations [N]` | Лимит tool-use итераций за один ход. Без аргумента показать текущий (дефолт 40). При исчерпании — мягкая остановка |
| `/paste` | Список вставок (Ctrl+V больших кусков) — [Pasted #N — L lines, C chars] |
| `/paste show <N>` | Содержимое вставки #N |
| `/project [bind <имя>\|on\|off\|unbind]` | Привязка каталога к проекту веб-чата; on/off — тумблер проекта (с v6.17) |
| `/loop <interval> <prompt>` | Фиксированный таймер-loop |
| `/loop stop` | Остановить |
| `/log` | Последние 20 LLM-запросов (видно какая модель реально ответила) |
| `/new` | Новая беседа (текущая сохранится) |
| `/sessions` | Список бесед |
| `/resume <num\|id>` | Открыть сохранённую беседу |
| `/compact` | Сжать историю в summary через LLM (для длинных бесед) |
| `/cd <path>` | Сменить рабочую папку |
| `/clear` | Очистить историю (Ctrl+L) |
| `/whoami` | Кто залогинен |
| `/config` | Показать конфиг |
| `/permissions` | Persistent-разрешения tools |
| `/classic on\|off` | Переключить classic TUI (alt-screen+mouse) вместо Ink-style. Требует рестарт |
| `/mouse on\|off` | Захват мыши (актуально только в classic TUI) |
| `/login` | Перелогиниться |
| `/logout` | Выйти |
| `/quit` | Выход (Ctrl+D) |

> Подсказка: набери `/` и автокомплит-меню покажет все команды. Для команд **без аргумента** выбор из меню сразу выполняется (один Enter). Команды с пробелом-аргументом (`/model `, `/source `, `/cd `) ждут ввод.

### Хоткеи

| Клавиша | Что |
|---|---|
| **Enter** | Отправить |
| **Shift+Enter** | Новая строка (multiline) |
| **↑ / ↓** | История твоих предыдущих вводов (как bash) |
| **Tab** | Принять подсказку из автокомплита |
| **Shift+Tab** | Циклить effort-уровень (или открыть picker) |
| **PgUp / PgDn** | Скролл чата |
| **Ctrl+R** | Fuzzy-поиск по сессиям |
| **Ctrl+C** | Отменить стрим (или дважды для выхода) |
| **Ctrl+D** | Выход (дважды для подтверждения) |
| **Ctrl+L** | Очистить историю |
| **Shift+drag** | Выделить текст (если мышь захвачена — `/mouse off` чтобы как обычно) |
| **Колесо мыши** | Скролл viewport |

---

## Сессии и история

Каждая беседа автоматически сохраняется в `~/.config/execai/sessions/<uuid>.json` после каждого обмена. После рестарта execai:
- Если в той же папке была беседа моложе 24h — продолжается
- Иначе стартует новая

`/sessions` — список всех бесед. `/resume 3` — открыть третью. `/resume <id>` — по ID.

`Ctrl+R` — fuzzy-поиск: вводишь любое слово → ищется по заголовкам И содержимому всех бесед.

`/compact` — если беседа слишком длинная и контекст переполняется, агент сожмёт старую часть в один summary, сохранит последние 6 turn'ов.

История вводов (`↑/↓`) тоже сохраняется в `~/.config/execai/input_history` — переживает рестарт.

---

## Где живут файлы

| ОС | Путь |
|---|---|
| Linux   | `~/.config/execai/` |
| macOS   | `~/Library/Application Support/execai/` |
| Windows | `%APPDATA%\execai\` |

| Файл | Что |
|---|---|
| `config.json` | api_base, selected_model_id, thinking_budget (effort) |
| `credentials.json` | JWT (mode 0600) |
| `host_id` | Стабильный machine-id (только если /etc/machine-id и т.п. недоступны — fallback) |
| `subscriptions.json` | Подключенные подписки (ключи в plain — храни безопасно) |
| `permissions.json` | Persistent-allow-list tools |
| `sessions/<uuid>.json` | История каждой беседы |
| `memory/MEMORY.md` | Индекс user memory (см. [Память](#память-агента)) |
| `memory/*.md` | Отдельные факты (user/feedback/project/reference) |
| `input_history` | Последние 200 твоих вводов |
| `requests.log` | Лог LLM-запросов (для `/log`) |
| `auth-poll.log` | Диагностика device-flow login (для отладки) |
| `models_cache.json` | Кеш каталога моделей — используется при отвале сети чтобы execai стартовал оффлайн |
| `installed_arch_sha` | SHA архитектуры для auto-update check |
| `last_remote_sha` | Stamp для сравнения версий |

### Memory — см. отдельный раздел

[Память агента](#память-агента) — структура user + project, автоматически подгружается в system prompt на каждой сессии.

---

## Если что-то сломалось

**`execai: command not found`** после установки:
- В текущей сессии: `exec bash -l` или открой новый терминал
- Или вручную: `export PATH="$HOME/.local/bin:$PATH"`

**"подписка zai не подключена"** при `/source zai`:
- Сначала `/connect zai <key>`. Где взять ключ — см. раздел [Источник](#источник-execai-vs-твоя-подписка)

**429 "Insufficient balance"** на Z.ai:
- Ты используешь обычный API-key, не Coding Plan key. Возьми именно Coding Plan: https://z.ai/manage-apikey/apikey-list (раздел Individual Coding Plan)

**404 "Not found"** при чате (ExecAI):
- Возможно gateway не знает endpoint. Сообщи разработчикам — может потребоваться передеплоить aicore-vbai.

**TUI не открывается, пустой экран**:
- На Windows используй **Windows Terminal**, не старый conhost. На Linux — любой современный (gnome-terminal, kitty, alacritty).
- Защитник: `Add-MpPreference -ExclusionPath "$env:LOCALAPPDATA\execai"` и переустанови.

**Кодировка ломается** (`Ð�пиши` вместо `Опиши`):
- Это баг старой версии bubbles. Обнови execai: `curl -fsSL .../install.sh | bash`.

**Авто-обновление не приходит**:
- В status bar внизу при старте увидишь `✓ execai R5.X — последняя версия` либо `🔔 Доступна новая`.
- Если не работает — `EXECAI_UPDATE_CHANNEL=R5` переопределит канал.

**Не отправляются картинки** хотя есть путь:
- Проверь что путь в кавычках (одинарные/двойные) если в нём есть пробелы или кириллица: `'/path/to/Снимок экрана.png'`
- Или без кавычек если путь без пробелов: `/path/to/foo.png`
- Если AI всё равно говорит "нет картинки" — проверь модель, она должна быть vision (sonnet/gpt-5/glm-5v и т.п.)

**Загрузка зависла**:
- `Ctrl+C` отменит текущий запрос. История сохранится.

**"Достигнут лимит N итераций"** в чате:
- Это НЕ ошибка — мягкая остановка после лимита tool-use итераций (дефолт 40).
- Просто скажи «продолжай» — агент возьмёт ещё столько же и продолжит с того же места.
- Если задача автономная и большая — поставь лимит выше: `/max-iterations 100` (диапазон 1-500).
- Сохранится в `~/.config/execai/config.json`, применяется к последующим ходам.

**"context deadline exceeded" / модели не загружаются**:
- С R5.67+ execai стартует ВСЕГДА даже если сеть недоступна.
- При первом старте берёт каталог из API и кеширует в `~/.config/execai/models_cache.json`.
- При отвале сети — использует кеш и показывает `ℹ Использую кешированный каталог`.
- Если и кеша нет — встроенная заглушка (Claude Sonnet 4.6), TUI открывается, реальные запросы могут упасть но интерфейс живой.

---

## Поддержка

- Bugs/feature requests: [github.com/execai/execai-agent/issues](https://github.com/execai/execai-agent/issues)
- Документация ExecAI: https://chat.execai.ru/

---

**execai** — by ExecAI/VBAI. MIT-style license.
