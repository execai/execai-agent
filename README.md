**English** | [Русский](README.ru.md) | [Español](README.es.md) | [Deutsch](README.de.md) | [中文](README.zh.md)

🌐 **Website:** [execai.ru](https://execai.ru) · 💬 Web chat: [chat.execai.ru](https://chat.execai.ru)

---

# execai — a terminal AI agent

execai is a terminal-first AI coding agent that works with your favorite LLM providers (Claude, GPT-5, Kimi K3, GLM-5.2, and more) — bring your own subscription or use ours. A CLI agent in the spirit of Claude Code that actually runs on your machine: it reads files, executes commands, talks to kubernetes, and chats back. It supports **9 sources** — our ExecAI backend + your Z.ai / Kimi Code / Anthropic / OpenAI subscriptions + local Claude Code / OpenAI Codex CLI / Ollama (cloud or local). You can switch between them on the fly with a shared conversation history.

```
execai R5.112 · openai/gpt-5 · it@execai.ru · ~
Type a task. /model — models, /help — commands, /quit — exit.

› check free disk space on server dev01
```

![demo](scripts/demo.gif)

**Ink-style rendering** (default since R5.107): message history is committed to the terminal scrollback (native selection works and doesn't get reset on updates). Live streaming and input live in a dynamic area at the bottom. Just like Claude Code.

---

## Contents

1. [Installation](#installation)
2. [First run](#first-run)
3. [Talking to the agent](#talking-to-the-agent)
4. [Sources — ExecAI and subscriptions](#sources--execai-and-subscriptions)
5. [Models](#models)
6. [Images and files](#images-and-files)
7. [Effort — reasoning level](#effort--reasoning-level)
8. [Loop and Autoloop — repeat and wait](#loop-and-autoloop--repeat-and-wait)
9. [Agent memory (EXECAI/MEMORY.md)](#agent-memory)
10. [Spend / `/usage`](#spend--usage)
11. [Commands and hotkeys](#commands-and-hotkeys)
12. [Sessions and history](#sessions-and-history)
13. [Where the files live](#where-the-files-live)
14. [Troubleshooting](#troubleshooting)

---

## Installation

### Linux / macOS (Intel and Apple Silicon)

```bash
curl -fsSL https://storage.yandexcloud.net/execai-agent-prod/execai/R5/latest/install.sh | bash
```

The script will:
- detect your architecture (`linux-amd64`, `linux-arm64`, `darwin-amd64`, `darwin-arm64`),
- verify the SHA256,
- drop the binary into `/usr/local/bin/execai` (with passwordless sudo) or `~/.local/bin/execai`,
- remove stale execai copies from PATH and dead/`/tmp/execai*` entries from your rc files,
- on macOS, strip `com.apple.quarantine` automatically (Gatekeeper).

After install — in a **new terminal**, just type `execai`. In the current one — if you already had `execai` and bash cached the old path, `hash -r` will fix it.

### Windows 10/11 (amd64 and ARM64)

```powershell
iwr -useb https://storage.yandexcloud.net/execai-agent-prod/execai/R5/latest/install.ps1 | iex
```

Auto-detects architecture (amd64 / arm64 for Copilot+ PCs), asks for **UAC** once to add the folder to Defender's exclusions (otherwise antivirus can eat the binary), downloads `execai.exe` into `%LOCALAPPDATA%\execai\`, and registers it in User PATH.

> Run it from **Windows Terminal** (not the old `cmd.exe`/conhost) — the TUI (bubbletea) renders properly there.

After install, verify:
```bash
execai version
# execai R5.NN
```

### Pin a specific version

```bash
curl -fsSL https://storage.yandexcloud.net/execai-agent-prod/execai/R5/42/install.sh | bash
```

### Manual download

If you prefer to grab the binary yourself instead of running an installer, all releases are on GitHub with checksums:

**https://github.com/execai/execai-agent/releases/latest**

Pick the archive for your platform (`execai-{linux,darwin,windows}-{amd64,arm64}.{tar.gz,zip}`), verify against `SHA256SUMS`, extract, and put on PATH.

---

## First run

```bash
execai
```

With no arguments, the chat TUI opens.

**If you're not logged in yet** — the agent will show a browser confirmation link and open it for you automatically:

```
👉 Open in your browser and confirm this is your agent:
   https://chat.execai.ru/agents/connect/U7XQ9F4P

(waiting, polling every 3s… once you confirm, we continue)
```

In the browser:
1. If you're not logged in to ExecAI — sign in (Yandex / VK / email).
2. On the "Connect agent" page, the alias field is pre-filled with your hostname. Accept it or override (e.g. "yz-laptop"). Click "Confirm".

The agent will receive a JWT and continue right away. The token lives for 90 days and auto-renews on every connect.

### What is a persistent agent

Every agent is bound to a **stable host-id** on your machine:
- Linux — `/etc/machine-id`
- macOS — `IOPlatformUUID`
- Windows — `HKLM\SOFTWARE\Microsoft\Cryptography\MachineGuid`
- Fallback — `~/.config/execai/host_id` (UUID generated on first install)

This means: if you reinstall execai, delete `credentials.json`, and log in again — **the backend reuses the existing session**, agent_id stays the same. No duplicate entries in your device list.

You can view all your agents and unlink any of them at `apidev.velesbsd.com/settings/agents` (in UserMenu bottom-left → "Agents", terminal icon).

---

## Talking to the agent

Just describe your task in plain language:

```
› check how much space /var/log takes on server dev01
```

The agent will:
- figure out what you need,
- decide which tool to use (Bash, Read, Grep, …),
- show you the command before running it and ask for confirmation if it's dangerous,
- execute it, read the result, and answer.

### Confirming dangerous commands

Read-only things (`ls`, `cat`, `git status`, `kubectl get`, `grep`) run automatically. Commands that change state (`rm`, `>`, `chmod`, `kubectl delete`, `git push`) will prompt:

```
⚠  Confirm Bash execution
   kubectl delete pod foo-xxx
   
   [y] Once  [a] All Bash this session  [s] This command this session  [f] FOREVER  [n] Deny
```

| Key | Meaning |
|---|---|
| **y** | Allow ONCE |
| **a** | Allow ALL Bash invocations until execai restarts |
| **s** | Allow this exact command (arguments included) until restart |
| **f** | Allow FOREVER (persisted to `~/.config/execai/permissions.json`) |
| **n** or Esc | Deny — the agent gets "user refused" back |

Navigation: ← → or Tab to move focus, Enter to confirm, or the shortcuts `y/a/s/f/n`.

### Built-in tools

| Tool | What it does |
|---|---|
| **Bash** | Runs a shell command with streaming output (lines appear as they're produced) |
| **Read** | Reads a file with offset/limit, auto-detects binaries |
| **Write** | Creates/overwrites a file (with confirmation) |
| **Edit** | Precise string replacement in a file (with confirmation) |
| **Grep** | Regexp search across a file tree |
| **Glob** | Find files by pattern like `**/*.go` |
| **LS** | Directory listing |
| **Tree** | File tree up to depth N |
| **WebFetch** | HTTP GET of a page (no JS rendering) |
| **TodoWrite** | The agent's internal task planner |
| **schedule_wakeup** | AI schedules its own wakeup in N seconds (see [Autoloop](#loop-and-autoloop--repeat-and-wait)) |

---

## Sources — ExecAI and subscriptions

The agent can talk to **9 different sources**. The conversation history is shared — switch on the fly, don't lose context.

| Source | What it is | Billing |
|---|---|---|
| **execai** (default) | Our gateway → any of ~34 models | ExecAI plan |
| **zai** | Z.ai GLM Coding Plan direct | Your Z.ai subscription |
| **kimi** | Kimi Code Coding Plan (kimi.com/code) | Your Kimi Code subscription |
| **kimi-api** | Moonshot Platform pay-per-token | Pay-per-token Moonshot |
| **anthropic** | Direct Claude API (sk-ant-...) | Pay-per-token Anthropic |
| **openai** | Direct OpenAI API (sk-proj-...) | Pay-per-token OpenAI |
| **claude-cli** | Delegates to your local `claude` CLI | Your Claude Pro/Max/Team OAuth |
| **codex-cli** | Delegates to your local `codex` CLI | Your ChatGPT Plus/Pro/Team OAuth |
| **ollama** | Cloud (ollama.com) or local (`ollama serve`) | Your subscription / 0 ₽ locally |

### 1. ExecAI — default

After login the agent uses our billing. No extra setup.

```
src : ExecAI   provider : anthropic   model : claude-sonnet-4-6
```

### 2. Z.ai Coding Plan

Your own $3-60/mo subscription → you hit GLM-5.2 directly, we don't charge anything.

1. Grab a **Coding Plan** key (not a regular API-key!):
   - https://z.ai/manage-apikey/apikey-list → **Individual Coding Plan > Plan Overview**
   - Team: https://z.ai/manage-apikey/coding-plan/team/my-plan

   > ⚠️ A regular API-key is billed pay-per-token, NOT from your subscription.

2. `/connect zai sk-zai-XXXXX`
3. `/source zai` → primary model **GLM-5.2**

### 3. Kimi Code Coding Plan

A separate Moonshot AI product — their Claude Code equivalent with its own subscription. Their flagship **K3** is available on every plan except the lowest tiers.

1. Get a key at https://www.kimi.com/code/console (API keys section).
2. ```
   /connect kimi sk-XXXXX
   /source kimi
   ```
3. Your plan is auto-detected via `/coding/v1/models` — the status bar will show `kimi (K3)`, `kimi (K3 + HighSpeed)`, or `kimi (K2.7 Code)` depending on what's actually available.

Models: `k3` (primary), `kimi-for-coding` (K2.7 Code), `kimi-for-coding-highspeed`. `/usage` shows your **real quota**: the weekly limit plus rolling windows (5h) with progress bars.

### 4. Moonshot Platform (pay-per-token API)

A regular API key for per-token billing (not a subscription). If you want Moonshot models outside of Kimi Code:

1. Get a key at https://platform.moonshot.ai/console/api-keys.
2. ```
   /connect kimi-api sk-XXXXX
   /source kimi-api
   ```

The model catalog is pulled dynamically from your account's `/v1/models`. Primary → `kimi-latest` (auto-alias to the current flagship). The key is validated at `/connect` time — on 401/403 you get an immediate rejection with a hint about which key goes where.

### 5. Anthropic API

Direct key from https://console.anthropic.com/settings/keys — pay-per-token.

```
/connect anthropic sk-ant-XXXXX
/source anthropic
```
Models: claude-sonnet-4-6, claude-opus-4-8, claude-haiku-4-5.

### 6. OpenAI Platform (pay-per-token API)

Direct key from https://platform.openai.com/api-keys — pay-per-token.

```
/connect openai sk-proj-XXXXX
/source openai
```

The catalog is pulled from your account's `/v1/models`. Primary → **gpt-5** → o3 → gpt-4.1 → gpt-4o (priority order).

### 7. Claude Code CLI (Claude Pro/Max/Team OAuth)

If you already have `claude` (Claude Code) installed and a **Pro/Max/Team** subscription — use its quota via the local OAuth session, no separate key needed.

```
/connect claude-cli    # checks that `claude` is in PATH
/source claude-cli
```

Models (aliases): sonnet / opus / haiku. Model selection is also managed externally via `claude config set defaultModel <id>`.

⚠️ **Caveats:**
- execai-tools (Bash/Read/Write) do NOT work through claude-cli — it spins up its own tools with its own permissions
- History is passed as a plain-text prompt (no session-id)

### 8. OpenAI Codex CLI (ChatGPT Plus/Pro/Team OAuth)

The `claude-cli` equivalent for OpenAI. Uses your ChatGPT subscription quota (Plus/Pro/Team/Enterprise) via the local `codex` binary.

**Install codex** (Linux/macOS):
```bash
curl -fsSL https://chatgpt.com/codex/install.sh | sh
codex login    # OAuth in the browser with your ChatGPT account
```
Windows:
```powershell
powershell -ExecutionPolicy ByPass -c "irm https://chatgpt.com/codex/install.ps1 | iex"
```

**In execai:**
```
/connect codex-cli
/source codex-cli
```

Models: gpt-5, o3, o4-mini. Same caveats as claude-cli — execai-tools don't work, codex spins up its own.

### 9. Ollama — cloud or local

Two modes under one command:

**Cloud (ollama.com):**
```
/connect ollama <api-key>          # key from https://ollama.com/settings/keys
/source ollama
```
Models on their servers: **glm-5.2** (primary), qwen3-coder:480b, kimi-k2:1t, deepseek-v3.1:671b, gpt-oss:120b. Anthropic-compatible endpoint, effort is supported.

**Local (your own Ollama):**
```
ollama pull llama3.2               # externally, whatever you want to run
/connect ollama local              # localhost:11434
/connect ollama local http://192.168.1.10:11434   # your own URL
/source ollama
```
The catalog is pulled dynamically via `/api/tags`. 0 ₽. OpenAI-compatible endpoint.

### About switching

- **History is shared.** `/source zai` → `/source execai` → `/source ollama` — context is preserved.
- **Billing is isolated.** On an external subscription, ExecAI billing does NOT charge you.
- **`/subscriptions`** — what's connected, `/disconnect <provider>` — remove.
- **`/source` with no argument** — quick picker.

---

## Models

`/model` with no argument — list of what's available. `/model <id>` or `/model <num>` — switch.

Autocomplete is more convenient:
```
/model<Tab>   →  menu of all models with a filter
/model deeps  →  filter by substring
```

### What's in the catalog (by source)

| Source | Models |
|---|---|
| **execai** | ~34 models — Claude/GPT/DeepSeek/Kimi/MiniMax/Qwen/GLM through our backend |
| **zai** | glm-5.2 (primary), glm-5.2[1m], glm-4.7 |
| **kimi** | k3 (primary), kimi-for-coding (K2.7), kimi-for-coding-highspeed |
| **kimi-api** | kimi-latest (primary), kimi-k2-turbo-preview, moonshot-v1-* (dynamically from /v1/models) |
| **anthropic** | claude-sonnet-4-6, claude-opus-4-8, claude-haiku-4-5 |
| **openai** | gpt-5 (primary), o3, gpt-4.1, gpt-4o, o4-mini (dynamically from /v1/models) |
| **claude-cli** | sonnet / opus / haiku (aliases + pinned versions) |
| **codex-cli** | gpt-5, o3, o4-mini |
| **ollama cloud** | glm-5.2, qwen3-coder:480b, kimi-k2:1t, deepseek-v3.1:671b, gpt-oss:120b |
| **ollama local** | whatever you installed via `ollama pull` (dynamically from /api/tags) |

Primary auto-switches when you change source. Prices and descriptions live in `/model`.

---

## Images and files

### Images

Just **drag and drop** a PNG/JPG/GIF/WEBP into your terminal window, or **type the path in quotes**:

```
› '/home/yz/Pictures/Screenshot.png' what's in this image?
```

The agent will:
- find the path in your message (spaces and non-ASCII characters in the filename are fine, if the file is quoted),
- base64-encode it,
- send it as `image_url` to a vision model,
- get back a description.

**Vision models** (that can see): claude-sonnet-4-6, gpt-5.5, glm-5v-turbo, glm-4.5v. If you have a non-vision model selected — switch via `/model`.

### Files (text/code)

Just say the path — the agent will read it via the `Read` tool:

```
› take a look at internal/chat/tui.go and suggest a refactor
```

Large files are read in chunks. If the AI misses something important, ask more specifically ("read lines 100-200" or "find function X").

### Attaching a file from the clipboard

Terminals can't pass images from the clipboard (Ctrl+V) — that's a limitation of all TUIs. Drag & drop of files (the terminal pastes the path) works.

---

## Effort — reasoning level

Supported on Anthropic-compatible sources: **zai, kimi, anthropic, ollama-cloud, claude-cli**. The model "thinks" internally first, then answers. On hard tasks it gives dramatically better results, but it's slower and eats more quota.

### Levels

```
/effort                      # open the picker
```

Six positions: **off** (0) · **low** (1024) · **medium** (4096) · **high** (8192) · **xhigh** (16384) · **max** (32000) tokens.

Controls in the picker: **← →** to select, **Enter** to apply, **Esc** to cancel.

**Shift+Tab** cycles through effort levels without opening the picker (doesn't work in some terminals — fall back to `/effort` in that case).

Persisted to `~/.config/execai/config.json`, visible in the status bar as `effort : high`.

---

## Loop and Autoloop — repeat and wait

### `/loop` — a simple repeater

Run a prompt on a timer:

```
/loop 5m check the CI build status
```

Every 5 minutes the agent resends the prompt. Useful for polling — "wait until something happens".

```
/loop          # show current status
/loop stop     # stop
```

The status bar shows `🔁 loop: 5m`. Works only while the TUI is open.

### Autoloop — the AI decides when to wake up

Smarter: the AI is given a `schedule_wakeup` tool and decides on its own when to come back to a task.

Example:
```
› run `npm install`, wait for it to finish, then check the logs for errors
```

The AI:
1. Runs `npm install` (streaming into the chat)
2. Sees that install will take ~5 minutes
3. Calls `schedule_wakeup(delay_seconds=300, reason="waiting for npm install", prompt_on_wake="check logs and errors")`
4. Ends the current response
5. **5 minutes later the TUI wakes the agent up** with the prompt "check logs and errors"
6. The AI continues from where it left off — it sees the entire conversation history

In the UI it looks like:
```
🌙 autoloop: wake in 5m (waiting for npm install) → prompt: "check logs and errors"
... 5 minutes later ...
› 🌙 [autoloop] check logs and errors
● ...AI continues...
```

If the AI decides the task is done — it simply doesn't call `schedule_wakeup`, and autoloop stops on its own.

**When to use which:**
- `/loop` — a fixed interval, you already know how long to wait
- Autoloop — the AI figures it out, you just describe the task

---

## Agent memory

The agent remembers facts across sessions — like Claude Code, but simpler. It's automatically loaded into the system prompt (only the index, not everything at once) — the context stays lightweight.

### Where things go

**User memory** (cross-project, personal facts about you):
```
~/.config/execai/memory/
├── MEMORY.md              ← index (always in system prompt)
├── user_role.md           ← 1 file per fact
├── feedback_style.md
├── project_alpha.md
└── reference_jenkins.md
```

Each file — frontmatter + a short body:
```markdown
---
name: user-role
description: senior Go engineer, live in EU
metadata:
  type: user
---

- Write in Go, less often Python
- Prefer bubbletea for TUI
- Don't use sudo unless absolutely necessary
```

In `MEMORY.md` — one line referencing the file:
```
- [🎯 My stack](user_role.md) — Go, bubbletea, EU timezone
```

**Project memory** (specific to this repo):
```
<CWD>/EXECAI.md            ← single file
```

### 4 record types

| Type | What | Example |
|---|---|---|
| **user** | Role, skills, preferences | "Write in Go, tech lead" |
| **feedback** | How you want it to behave | "Don't do long summaries. Why: I don't read them" |
| **project** | Initiatives, deadlines, decisions | "R5 — new branch for subscriptions, deadline Q3" |
| **reference** | Pointers to external systems | "Jenkins: jenkins.velesbsd.com, dashboards in Grafana" |

### How to use it

Just tell the chat what to remember:
```
› remember that I write in Go and prefer bubbletea
```
The model will create `user_role.md` (or update the existing one) + add a line to `MEMORY.md`.

```
› project R5 — new branch for subscriptions/GUI, don't merge into R4
```
Creates `project_r5.md` in user memory. If the fact is about THIS repo — updates `EXECAI.md` in CWD.

The model does NOT clutter memory with episodic facts ("today I fixed a bug" — that belongs in git log). It only writes down durable things.

Works across **all sources** — the system prompt is the same.

---

## Spend / `/usage`

```
/usage
```

Shows different things depending on the active `/source`:

**ExecAI (default):** your plan + balance + 4 rate-limit windows (5h/day/week/month) with progress bars + the last 14 iterations (model, tokens, price in ₽).

**Kimi Code (`/source kimi`):** real quota from `api.kimi.com/coding/v1/usages` — plan by available models, weekly limit with reset time, rolling windows (5h etc.).

**Moonshot Platform (`/source kimi-api`), OpenAI (`/source openai`), Anthropic, Z.ai:** local token counters + a link to the provider's dashboard (real billing lives there).

---

## Commands and hotkeys

### Slash commands

| Command | What it does |
|---|---|
| `/help` | All commands |
| `/model [id]` | Change model |
| `/source [name]` | Change source (execai/zai/kimi/kimi-api/anthropic/openai/claude-cli/codex-cli/ollama) |
| `/connect <provider> [args...]` | Connect a subscription. No args — help |
| `/disconnect <provider>` | Disconnect a subscription |
| `/subscriptions` (or `/subs`) | List connections |
| `/usage` | Plan + spend (per-source specifics) |
| `/effort` | Reasoning-level picker (for Anthropic-compat sources) |
| `/max-iterations [N]` | Limit on tool-use iterations per turn. No arg shows current (default 40). On exhaustion — a soft stop |
| `/paste` | List of pastes (Ctrl+V of big chunks) — [Pasted #N — L lines, C chars] |
| `/paste show <N>` | Contents of paste #N |
| `/loop <interval> <prompt>` | Fixed-timer loop |
| `/loop stop` | Stop |
| `/log` | Last 20 LLM requests (see which model actually answered) |
| `/new` | New conversation (current one is saved) |
| `/sessions` | List of conversations |
| `/resume <num\|id>` | Open a saved conversation |
| `/compact` | Compact history into a summary via the LLM (for long conversations) |
| `/cd <path>` | Change working directory |
| `/clear` | Clear history (Ctrl+L) |
| `/whoami` | Who's logged in |
| `/config` | Show config |
| `/permissions` | Persistent tool permissions |
| `/classic on\|off` | Toggle classic TUI (alt-screen+mouse) instead of Ink-style. Requires restart |
| `/mouse on\|off` | Mouse capture (only relevant in classic TUI) |
| `/login` | Re-login |
| `/logout` | Log out |
| `/quit` | Exit (Ctrl+D) |

> Tip: type `/` and the autocomplete menu shows every command. For commands **with no arguments**, selecting from the menu executes immediately (single Enter). Commands that take a space-separated argument (`/model `, `/source `, `/cd `) wait for input.

### Hotkeys

| Key | Action |
|---|---|
| **Enter** | Send |
| **Shift+Enter** | New line (multiline) |
| **↑ / ↓** | History of your previous inputs (like bash) |
| **Tab** | Accept an autocomplete suggestion |
| **Shift+Tab** | Cycle effort level (or open the picker) |
| **PgUp / PgDn** | Scroll the chat |
| **Ctrl+R** | Fuzzy search through sessions |
| **Ctrl+C** | Cancel stream (or twice to exit) |
| **Ctrl+D** | Exit (twice to confirm) |
| **Ctrl+L** | Clear history |
| **Shift+drag** | Select text (if the mouse is captured — `/mouse off` to select normally) |
| **Mouse wheel** | Scroll the viewport |

---

## Sessions and history

Every conversation is auto-saved to `~/.config/execai/sessions/<uuid>.json` after each exchange. On restart:
- If the same folder had a conversation younger than 24h — it resumes
- Otherwise a new one starts

`/sessions` — list of all conversations. `/resume 3` — open the third one. `/resume <id>` — by ID.

`Ctrl+R` — fuzzy search: type any word → it searches both titles AND contents of every conversation.

`/compact` — if a conversation gets too long and the context overflows, the agent will compress the older part into a single summary while keeping the last 6 turns.

Input history (`↑/↓`) is also saved to `~/.config/execai/input_history` — it survives restarts.

---

## Where the files live

| OS | Path |
|---|---|
| Linux   | `~/.config/execai/` |
| macOS   | `~/Library/Application Support/execai/` |
| Windows | `%APPDATA%\execai\` |

| File | What |
|---|---|
| `config.json` | api_base, selected_model_id, thinking_budget (effort) |
| `credentials.json` | JWT (mode 0600) |
| `host_id` | Stable machine-id (only if /etc/machine-id etc. aren't available — fallback) |
| `subscriptions.json` | Connected subscriptions (keys in plaintext — store safely) |
| `permissions.json` | Persistent tool allow-list |
| `sessions/<uuid>.json` | Each conversation's history |
| `memory/MEMORY.md` | User memory index (see [Memory](#agent-memory)) |
| `memory/*.md` | Individual facts (user/feedback/project/reference) |
| `input_history` | Last 200 of your inputs |
| `requests.log` | LLM request log (for `/log`) |
| `auth-poll.log` | Device-flow login diagnostics (for debugging) |
| `models_cache.json` | Model catalog cache — used when the network is down so execai still starts offline |
| `installed_arch_sha` | Architecture SHA for the auto-update check |
| `last_remote_sha` | Stamp for version comparison |

### Memory — see the dedicated section

[Agent memory](#agent-memory) — user + project structure, auto-loaded into the system prompt every session.

---

## Troubleshooting

**`execai: command not found`** after install:
- In the current session: `exec bash -l` or open a new terminal
- Or manually: `export PATH="$HOME/.local/bin:$PATH"`

**"zai subscription is not connected"** on `/source zai`:
- First `/connect zai <key>`. Where to get the key — see the [Sources](#sources--execai-and-subscriptions) section

**429 "Insufficient balance"** on Z.ai:
- You're using a regular API-key, not a Coding Plan key. Grab a proper Coding Plan one: https://z.ai/manage-apikey/apikey-list (Individual Coding Plan section)

**404 "Not found"** during chat (ExecAI):
- The gateway might not know the endpoint. Report it to the developers — aicore-vbai may need to be redeployed.

**TUI won't open, blank screen**:
- On Windows use **Windows Terminal**, not the old conhost. On Linux — any modern one (gnome-terminal, kitty, alacritty).
- Defender: `Add-MpPreference -ExclusionPath "$env:LOCALAPPDATA\execai"` and reinstall.

**Encoding breaks** (`Ð�escribe` instead of `Describe`):
- Old bubbles version bug. Update execai: `curl -fsSL .../install.sh | bash`.

**Auto-update never arrives**:
- On startup the status bar at the bottom shows `✓ execai R5.X — latest version` or `🔔 New version available`.
- If it's not working — `EXECAI_UPDATE_CHANNEL=R5` overrides the channel.

**Images don't get sent** even though the path is there:
- Check the path is in quotes (single or double) if it contains spaces or non-ASCII: `'/path/to/Screenshot.png'`
- Or unquoted if the path has no spaces: `/path/to/foo.png`
- If the AI still says "no image" — check the model, it must be a vision one (sonnet/gpt-5/glm-5v etc.)

**Request hung**:
- `Ctrl+C` cancels the current request. History is preserved.

**"Reached N iterations limit"** in chat:
- This is NOT an error — it's a soft stop after the tool-use iteration limit (default 40).
- Just say "continue" — the agent will take another batch of the same size and pick up where it left off.
- If the task is autonomous and large — raise the limit: `/max-iterations 100` (range 1-500).
- Saved to `~/.config/execai/config.json`, applies to subsequent turns.

**"context deadline exceeded" / models won't load**:
- Since R5.67+ execai ALWAYS starts, even with no network.
- On the first run it fetches the catalog from the API and caches it in `~/.config/execai/models_cache.json`.
- When the network goes down — it uses the cache and shows `ℹ Using cached catalog`.
- If there's no cache either — a built-in fallback (Claude Sonnet 4.6) kicks in, the TUI opens, real requests may fail but the interface is alive.

---

## Support

- Bugs/feature requests: [github.com/velesbsdllc/agent-vbai/issues](https://github.com/velesbsdllc/agent-vbai/issues)
- ExecAI docs: https://chat.execai.ru/

---

**execai** — by ExecAI/VBAI. MIT-style license.
