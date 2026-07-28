[English](README.md) | [Русский](README.ru.md) | [Español](README.es.md) | [Deutsch](README.de.md) | **中文**

---

# execai — 终端 AI 编程助手

execai — 终端优先的 AI 编程助手，与你喜欢的 LLM 提供方（Claude、GPT-5、Kimi K3、GLM-5.2 等）无缝协作 — 用你自己的订阅，或者用我们的。这是一个 Claude Code 风格的 CLI agent，真正跑在你自己的机器上：读文件、执行命令、与 kubernetes 对话，然后把结果聊给你听。它支持 **9 个源** — 我们的 ExecAI 后端 + 你自己的 Z.ai / Kimi Code / Anthropic / OpenAI 订阅 + 本地的 Claude Code / OpenAI Codex CLI / Ollama（云端或本地）。你可以随时在它们之间切换，会话历史保持共享。

```
execai R5.112 · openai/gpt-5 · alort5@yandex.com · ~
Type a task. /model — models, /help — commands, /quit — exit.

› check free disk space on server dev01
```

**Ink-style 渲染**（自 R5.107 起默认开启）：消息历史直接写入终端 scrollback（原生文本选择可用，且不会在更新时被清空）。流式输出和输入区域位于底部的动态区域。就像 Claude Code 一样。

---

## 目录

1. [安装](#安装)
2. [首次运行](#首次运行)
3. [与 agent 对话](#与-agent-对话)
4. [源 — ExecAI 与订阅](#源--execai-与订阅)
5. [模型](#模型)
6. [图片和文件](#图片和文件)
7. [Effort — 推理强度](#effort--推理强度)
8. [Loop 和 Autoloop — 重复与等待](#loop-和-autoloop--重复与等待)
9. [Agent 记忆（EXECAI/MEMORY.md）](#agent-记忆)
10. [消费 / `/usage`](#消费--usage)
11. [命令与快捷键](#命令与快捷键)
12. [会话与历史](#会话与历史)
13. [文件位置](#文件位置)
14. [故障排查](#故障排查)

---

## 安装

### Linux / macOS（Intel 与 Apple Silicon）

```bash
curl -fsSL https://storage.yandexcloud.net/execai-agent-prod/execai/R5/latest/install.sh | bash
```

脚本会：
- 检测你的架构（`linux-amd64`、`linux-arm64`、`darwin-amd64`、`darwin-arm64`），
- 校验 SHA256，
- 将二进制文件放到 `/usr/local/bin/execai`（使用免密 sudo）或 `~/.local/bin/execai`，
- 清理 PATH 中过时的 execai 副本，以及 rc 文件中失效的 `/tmp/execai*` 条目，
- 在 macOS 上，自动去除 `com.apple.quarantine`（Gatekeeper）。

安装完成后 — 在**新终端**里直接输入 `execai`。在当前终端里 — 如果你之前装过 `execai` 且 bash 缓存了旧路径，`hash -r` 就能解决。

### Windows 10/11（amd64 和 ARM64）

```powershell
iwr -useb https://storage.yandexcloud.net/execai-agent-prod/execai/R5/latest/install.ps1 | iex
```

自动识别架构（amd64 / arm64，适配 Copilot+ PC），请求一次 **UAC** 权限，将安装目录加入 Defender 排除列表（否则杀毒软件可能会误删二进制文件），把 `execai.exe` 下载到 `%LOCALAPPDATA%\execai\`，并注册到 User PATH。

> 请从 **Windows Terminal** 运行（不要用老的 `cmd.exe`/conhost）— 这样 TUI（bubbletea）才能正确渲染。

安装完成后，验证一下：
```bash
execai version
# execai R5.NN
```

### 固定到特定版本

```bash
curl -fsSL https://storage.yandexcloud.net/execai-agent-prod/execai/R5/42/install.sh | bash
```

---

## 首次运行

```bash
execai
```

不带任何参数时，会打开聊天 TUI。

**如果你还没登录** — agent 会显示一个浏览器确认链接，并自动为你打开：

```
👉 Open in your browser and confirm this is your agent:
   https://chat.execai.ru/agents/connect/U7XQ9F4P

(waiting, polling every 3s… once you confirm, we continue)
```

在浏览器里：
1. 如果你还没登录 ExecAI — 先登录（Yandex / VK / 邮箱）。
2. 在"Connect agent"页面，alias 字段已预填为你的主机名。可以接受，也可以修改（比如 "yz-laptop"）。点击"Confirm"。

Agent 会立刻收到 JWT 并继续运行。Token 有效期 90 天，每次 connect 时会自动续期。

### 什么是持久 agent

每个 agent 都绑定到你机器上一个**稳定的 host-id**：
- Linux — `/etc/machine-id`
- macOS — `IOPlatformUUID`
- Windows — `HKLM\SOFTWARE\Microsoft\Cryptography\MachineGuid`
- 回退方案 — `~/.config/execai/host_id`（首次安装时生成的 UUID）

这意味着：如果你重装 execai、删掉 `credentials.json`、再次登录 — **后端会复用已有的会话**，agent_id 保持不变。你的设备列表里不会出现重复项。

你可以在 `apidev.velesbsd.com/settings/agents`（左下角 UserMenu → "Agents"，终端图标）查看所有 agent 并解绑任意一个。

---

## 与 agent 对话

用自然语言描述你的任务即可：

```
› check how much space /var/log takes on server dev01
```

Agent 会：
- 弄清楚你需要什么，
- 决定使用哪个工具（Bash、Read、Grep 等），
- 在运行前展示命令，如果有危险会请求确认，
- 执行命令、读取结果、回答你。

### 危险命令的确认

只读操作（`ls`、`cat`、`git status`、`kubectl get`、`grep`）会自动运行。会改变状态的命令（`rm`、`>`、`chmod`、`kubectl delete`、`git push`）会弹出提示：

```
⚠  Confirm Bash execution
   kubectl delete pod foo-xxx
   
   [y] Once  [a] All Bash this session  [s] This command this session  [f] FOREVER  [n] Deny
```

| 按键 | 含义 |
|---|---|
| **y** | 仅本次允许 |
| **a** | 直到 execai 重启前，允许所有 Bash 调用 |
| **s** | 直到重启前，允许这条完全一样的命令（包括参数） |
| **f** | 永久允许（持久化到 `~/.config/execai/permissions.json`） |
| **n** 或 Esc | 拒绝 — agent 会收到 "user refused" |

导航：← → 或 Tab 移动焦点，Enter 确认，或直接用快捷键 `y/a/s/f/n`。

### 内置工具

| 工具 | 作用 |
|---|---|
| **Bash** | 执行 shell 命令，支持流式输出（行一出现就显示） |
| **Read** | 读取文件，支持 offset/limit，自动识别二进制 |
| **Write** | 创建/覆盖文件（需要确认） |
| **Edit** | 在文件中精确替换字符串（需要确认） |
| **Grep** | 在文件树中做正则搜索 |
| **Glob** | 按 `**/*.go` 这类模式查找文件 |
| **LS** | 列出目录 |
| **Tree** | 按深度 N 显示文件树 |
| **WebFetch** | HTTP GET 抓取页面（不渲染 JS） |
| **TodoWrite** | Agent 内部的任务规划器 |
| **schedule_wakeup** | AI 自己安排 N 秒后唤醒（详见 [Autoloop](#loop-和-autoloop--重复与等待)） |

---

## 源 — ExecAI 与订阅

Agent 可以对接 **9 个不同的源**。会话历史是共享的 — 随意切换，不会丢失上下文。

| 源 | 是什么 | 计费 |
|---|---|---|
| **execai**（默认） | 我们的网关 → 约 34 个模型任选 | ExecAI 套餐 |
| **zai** | 直连 Z.ai GLM Coding Plan | 你的 Z.ai 订阅 |
| **kimi** | Kimi Code Coding Plan（kimi.com/code） | 你的 Kimi Code 订阅 |
| **kimi-api** | Moonshot Platform 按 token 付费 | Moonshot 按 token 付费 |
| **anthropic** | 直连 Claude API（sk-ant-...） | Anthropic 按 token 付费 |
| **openai** | 直连 OpenAI API（sk-proj-...） | OpenAI 按 token 付费 |
| **claude-cli** | 委派给你本地的 `claude` CLI | 你的 Claude Pro/Max/Team OAuth |
| **codex-cli** | 委派给你本地的 `codex` CLI | 你的 ChatGPT Plus/Pro/Team OAuth |
| **ollama** | 云端（ollama.com）或本地（`ollama serve`） | 你的订阅 / 本地 0 ₽ |

### 1. ExecAI — 默认

登录后 agent 会使用我们的计费。无需额外配置。

```
src : ExecAI   provider : anthropic   model : claude-sonnet-4-6
```

### 2. Z.ai Coding Plan

你自己的 $3-60/月订阅 → 你直接访问 GLM-5.2，我们不收任何费用。

1. 获取 **Coding Plan** key（不是普通的 API-key！）：
   - https://z.ai/manage-apikey/apikey-list → **Individual Coding Plan > Plan Overview**
   - 团队版：https://z.ai/manage-apikey/coding-plan/team/my-plan

   > ⚠️ 普通的 API-key 会按 token 计费，**不会**从你的订阅里扣。

2. `/connect zai sk-zai-XXXXX`
3. `/source zai` → 主模型 **GLM-5.2**

### 3. Kimi Code Coding Plan

Moonshot AI 的另一款独立产品 — 相当于他们家的 Claude Code，有独立订阅。他们的旗舰 **K3** 除了最低档位以外的所有套餐都能用。

1. 在 https://www.kimi.com/code/console 拿到 key（API keys 部分）。
2. ```
   /connect kimi sk-XXXXX
   /source kimi
   ```
3. 你的套餐会通过 `/coding/v1/models` 自动检测 — 状态栏会根据实际可用情况显示 `kimi (K3)`、`kimi (K3 + HighSpeed)` 或 `kimi (K2.7 Code)`。

模型：`k3`（主模型）、`kimi-for-coding`（K2.7 Code）、`kimi-for-coding-highspeed`。`/usage` 显示你的**真实配额**：周限制 + 滚动窗口（5 小时）+ 进度条。

### 4. Moonshot Platform（按 token 付费的 API）

普通的按 token 计费 API key（不是订阅）。如果你想在 Kimi Code 之外使用 Moonshot 模型：

1. 在 https://platform.moonshot.ai/console/api-keys 获取 key。
2. ```
   /connect kimi-api sk-XXXXX
   /source kimi-api
   ```

模型目录会动态从你账户的 `/v1/models` 拉取。主模型 → `kimi-latest`（自动别名为当前旗舰）。Key 会在 `/connect` 时校验 — 遇到 401/403 会立刻拒绝，并提示你哪个 key 该用在哪里。

### 5. Anthropic API

从 https://console.anthropic.com/settings/keys 拿到的直连 key — 按 token 付费。

```
/connect anthropic sk-ant-XXXXX
/source anthropic
```
模型：claude-sonnet-4-6、claude-opus-4-8、claude-haiku-4-5。

### 6. OpenAI Platform（按 token 付费的 API）

从 https://platform.openai.com/api-keys 拿到的直连 key — 按 token 付费。

```
/connect openai sk-proj-XXXXX
/source openai
```

模型目录会从你账户的 `/v1/models` 拉取。主模型 → **gpt-5** → o3 → gpt-4.1 → gpt-4o（按优先级排序）。

### 7. Claude Code CLI（Claude Pro/Max/Team OAuth）

如果你已经装好了 `claude`（Claude Code），并且拥有 **Pro/Max/Team** 订阅 — 可以通过本地 OAuth 会话直接用它的配额，不需要额外的 key。

```
/connect claude-cli    # 检查 `claude` 是否在 PATH 中
/source claude-cli
```

模型（别名）：sonnet / opus / haiku。模型选择也可以在外部通过 `claude config set defaultModel <id>` 管理。

⚠️ **注意事项：**
- execai 的工具（Bash/Read/Write）在 claude-cli 下**不能用** — 它会启动自己的工具，用它自己的权限
- 历史以纯文本 prompt 的形式传入（没有 session-id）

### 8. OpenAI Codex CLI（ChatGPT Plus/Pro/Team OAuth）

`claude-cli` 在 OpenAI 端的等价物。通过本地 `codex` 二进制文件使用你的 ChatGPT 订阅配额（Plus/Pro/Team/Enterprise）。

**安装 codex**（Linux/macOS）：
```bash
curl -fsSL https://chatgpt.com/codex/install.sh | sh
codex login    # 在浏览器里用 ChatGPT 账户完成 OAuth
```
Windows：
```powershell
powershell -ExecutionPolicy ByPass -c "irm https://chatgpt.com/codex/install.ps1 | iex"
```

**在 execai 中：**
```
/connect codex-cli
/source codex-cli
```

模型：gpt-5、o3、o4-mini。注意事项和 claude-cli 一样 — execai 的工具用不了，codex 会启动它自己的。

### 9. Ollama — 云端或本地

一条命令，两种模式：

**云端（ollama.com）：**
```
/connect ollama <api-key>          # key 在 https://ollama.com/settings/keys 获取
/source ollama
```
他们服务器上的模型：**glm-5.2**（主模型）、qwen3-coder:480b、kimi-k2:1t、deepseek-v3.1:671b、gpt-oss:120b。Anthropic 兼容端点，支持 effort。

**本地（你自己的 Ollama）：**
```
ollama pull llama3.2               # 外部命令，你想跑什么就 pull 什么
/connect ollama local              # localhost:11434
/connect ollama local http://192.168.1.10:11434   # 你自己的 URL
/source ollama
```
模型目录通过 `/api/tags` 动态拉取。0 ₽。OpenAI 兼容端点。

### 关于切换

- **历史共享。** `/source zai` → `/source execai` → `/source ollama` — 上下文一直都在。
- **计费相互隔离。** 使用外部订阅时，ExecAI 计费**不会**扣你的钱。
- **`/subscriptions`** — 查看已连接的订阅，`/disconnect <provider>` — 移除。
- **`/source` 不带参数** — 快速选择器。

---

## 模型

`/model` 不带参数 — 列出所有可用模型。`/model <id>` 或 `/model <num>` — 切换。

用自动补全更方便：
```
/model<Tab>   →  所有模型菜单，带过滤
/model deeps  →  按子串过滤
```

### 目录里都有什么（按源）

| 源 | 模型 |
|---|---|
| **execai** | 约 34 个模型 — 通过我们的后端接入 Claude/GPT/DeepSeek/Kimi/MiniMax/Qwen/GLM |
| **zai** | glm-5.2（主模型）、glm-5.2[1m]、glm-4.7 |
| **kimi** | k3（主模型）、kimi-for-coding（K2.7）、kimi-for-coding-highspeed |
| **kimi-api** | kimi-latest（主模型）、kimi-k2-turbo-preview、moonshot-v1-*（从 /v1/models 动态获取） |
| **anthropic** | claude-sonnet-4-6、claude-opus-4-8、claude-haiku-4-5 |
| **openai** | gpt-5（主模型）、o3、gpt-4.1、gpt-4o、o4-mini（从 /v1/models 动态获取） |
| **claude-cli** | sonnet / opus / haiku（别名 + 固定版本） |
| **codex-cli** | gpt-5、o3、o4-mini |
| **ollama cloud** | glm-5.2、qwen3-coder:480b、kimi-k2:1t、deepseek-v3.1:671b、gpt-oss:120b |
| **ollama local** | 你通过 `ollama pull` 安装的任何模型（从 /api/tags 动态获取） |

切换源时主模型会自动切换。价格和说明都在 `/model` 里。

---

## 图片和文件

### 图片

直接把 PNG/JPG/GIF/WEBP **拖拽**到终端窗口里，或者**用引号写路径**：

```
› '/home/yz/Pictures/Screenshot.png' what's in this image?
```

Agent 会：
- 在你的消息里找到路径（文件名里带空格和非 ASCII 字符也没事，只要用引号括起来），
- base64 编码，
- 作为 `image_url` 发送给 vision 模型，
- 拿回描述。

**Vision 模型**（能看图的）：claude-sonnet-4-6、gpt-5.5、glm-5v-turbo、glm-4.5v。如果你当前选的不是 vision 模型 — 通过 `/model` 切换。

### 文件（文本/代码）

只要提到路径 — agent 会通过 `Read` 工具读取：

```
› take a look at internal/chat/tui.go and suggest a refactor
```

大文件会分块读取。如果 AI 漏掉了重要信息，问得更具体点（"读 100-200 行"或"找函数 X"）。

### 从剪贴板附加文件

终端无法从剪贴板传递图片（Ctrl+V）— 这是所有 TUI 的通用限制。拖拽文件是可以的（终端会粘贴路径）。

---

## Effort — 推理强度

在 Anthropic 兼容的源上支持：**zai、kimi、anthropic、ollama-cloud、claude-cli**。模型会先在内部"思考"再作答。在难题上效果会显著更好，但速度更慢、消耗更多配额。

### 等级

```
/effort                      # 打开选择器
```

六档：**off**（0）· **low**（1024）· **medium**（4096）· **high**（8192）· **xhigh**（16384）· **max**（32000）token。

选择器里的操作：**← →** 选择，**Enter** 应用，**Esc** 取消。

**Shift+Tab** 在不打开选择器的情况下循环切换 effort 等级（部分终端不支持 — 遇到这种情况回退用 `/effort`）。

设置持久化到 `~/.config/execai/config.json`，状态栏显示为 `effort : high`。

---

## Loop 和 Autoloop — 重复与等待

### `/loop` — 简单重复器

按定时器运行一个 prompt：

```
/loop 5m check the CI build status
```

每 5 分钟 agent 会重发这个 prompt。适合轮询 — "等某件事发生"。

```
/loop          # 显示当前状态
/loop stop     # 停止
```

状态栏会显示 `🔁 loop: 5m`。只在 TUI 打开时生效。

### Autoloop — AI 自己决定何时唤醒

更聪明的方式：给 AI 一个 `schedule_wakeup` 工具，让它自己决定何时回到某个任务。

例如：
```
› run `npm install`, wait for it to finish, then check the logs for errors
```

AI 会：
1. 运行 `npm install`（流式输出到聊天里）
2. 看到 install 大约需要 5 分钟
3. 调用 `schedule_wakeup(delay_seconds=300, reason="waiting for npm install", prompt_on_wake="check logs and errors")`
4. 结束当前响应
5. **5 分钟后 TUI 用 "check logs and errors" 唤醒 agent**
6. AI 从中断的地方继续 — 它能看到完整的会话历史

在 UI 里看起来是这样：
```
🌙 autoloop: wake in 5m (waiting for npm install) → prompt: "check logs and errors"
... 5 minutes later ...
› 🌙 [autoloop] check logs and errors
● ...AI continues...
```

如果 AI 判断任务已经完成 — 它就不再调用 `schedule_wakeup`，autoloop 自己就停了。

**什么时候用哪个：**
- `/loop` — 固定间隔，你已经知道要等多久
- Autoloop — 让 AI 自己判断，你只需要描述任务

---

## Agent 记忆

Agent 会跨会话记住事实 — 类似 Claude Code，但更简单。它会自动加载到 system prompt 里（只加载索引，不是全部内容）— 上下文保持轻量。

### 文件放在哪里

**用户记忆**（跨项目、关于你个人的事实）：
```
~/.config/execai/memory/
├── MEMORY.md              ← 索引（始终在 system prompt 里）
├── user_role.md           ← 每条事实 1 个文件
├── feedback_style.md
├── project_alpha.md
└── reference_jenkins.md
```

每个文件 — frontmatter + 简短正文：
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

在 `MEMORY.md` 里 — 一行引用对应文件：
```
- [🎯 My stack](user_role.md) — Go, bubbletea, EU timezone
```

**项目记忆**（当前 repo 专属）：
```
<CWD>/EXECAI.md            ← 单文件
```

### 4 种记录类型

| 类型 | 内容 | 例子 |
|---|---|---|
| **user** | 角色、技能、偏好 | "Write in Go, tech lead" |
| **feedback** | 你希望它怎么表现 | "Don't do long summaries. Why: I don't read them" |
| **project** | 计划、截止日期、决策 | "R5 — new branch for subscriptions, deadline Q3" |
| **reference** | 指向外部系统的指针 | "Jenkins: jenkins.velesbsd.com, dashboards in Grafana" |

### 怎么用

直接在聊天里告诉它要记住什么：
```
› remember that I write in Go and prefer bubbletea
```
模型会创建 `user_role.md`（或更新已有的）+ 在 `MEMORY.md` 里加一行。

```
› project R5 — new branch for subscriptions/GUI, don't merge into R4
```
在用户记忆里创建 `project_r5.md`。如果这条事实是关于**当前 repo**的 — 则更新 CWD 里的 `EXECAI.md`。

模型**不会**用琐碎的事件性事实塞满记忆（"今天我修了个 bug" — 那属于 git log）。它只记录持久性的东西。

**所有源**都能用 — system prompt 是一样的。

---

## 消费 / `/usage`

```
/usage
```

根据当前的 `/source` 显示不同内容：

**ExecAI（默认）：** 你的套餐 + 余额 + 4 个限流窗口（5h/day/week/month）带进度条 + 最近 14 次迭代（模型、token、₽ 价格）。

**Kimi Code（`/source kimi`）：** 从 `api.kimi.com/coding/v1/usages` 获取的真实配额 — 按可用模型的套餐、周限制及重置时间、滚动窗口（5 小时等）。

**Moonshot Platform（`/source kimi-api`）、OpenAI（`/source openai`）、Anthropic、Z.ai：** 本地 token 计数器 + 到提供方仪表盘的链接（真实计费在那边）。

---

## 命令与快捷键

### 斜杠命令

| 命令 | 作用 |
|---|---|
| `/help` | 所有命令 |
| `/model [id]` | 切换模型 |
| `/source [name]` | 切换源（execai/zai/kimi/kimi-api/anthropic/openai/claude-cli/codex-cli/ollama） |
| `/connect <provider> [args...]` | 连接一个订阅。不带参数 — 显示帮助 |
| `/disconnect <provider>` | 断开一个订阅 |
| `/subscriptions`（或 `/subs`） | 列出所有连接 |
| `/usage` | 套餐 + 消费（按源不同） |
| `/effort` | 推理强度选择器（用于 Anthropic 兼容的源） |
| `/max-iterations [N]` | 每轮 tool-use 迭代上限。不带参数显示当前值（默认 40）。到达上限时 — 软停止 |
| `/paste` | 粘贴列表（Ctrl+V 粘贴的大段内容） — [Pasted #N — L lines, C chars] |
| `/paste show <N>` | 查看粘贴 #N 的内容 |
| `/loop <interval> <prompt>` | 固定定时循环 |
| `/loop stop` | 停止 |
| `/log` | 最近 20 次 LLM 请求（看看实际是哪个模型在回答） |
| `/new` | 新会话（当前会话会被保存） |
| `/sessions` | 列出所有会话 |
| `/resume <num\|id>` | 打开已保存的会话 |
| `/compact` | 通过 LLM 把历史压缩为摘要（用于长会话） |
| `/cd <path>` | 切换工作目录 |
| `/clear` | 清除历史（Ctrl+L） |
| `/whoami` | 当前登录的是谁 |
| `/config` | 查看配置 |
| `/permissions` | 持久化的工具权限 |
| `/classic on\|off` | 切换到经典 TUI（alt-screen+mouse），而不是 Ink-style。需要重启 |
| `/mouse on\|off` | 鼠标捕获（仅在经典 TUI 下有意义） |
| `/login` | 重新登录 |
| `/logout` | 退出登录 |
| `/quit` | 退出（Ctrl+D） |

> 提示：输入 `/` 后自动补全菜单会展示所有命令。**不带参数**的命令，从菜单里选中即立即执行（单次 Enter）。带空格分隔参数的命令（`/model `、`/source `、`/cd `）会等待输入。

### 快捷键

| 按键 | 动作 |
|---|---|
| **Enter** | 发送 |
| **Shift+Enter** | 新行（多行输入） |
| **↑ / ↓** | 输入历史（类似 bash） |
| **Tab** | 接受自动补全建议 |
| **Shift+Tab** | 循环切换 effort 等级（或打开选择器） |
| **PgUp / PgDn** | 滚动聊天 |
| **Ctrl+R** | 模糊搜索会话 |
| **Ctrl+C** | 取消流式输出（或按两次退出） |
| **Ctrl+D** | 退出（按两次确认） |
| **Ctrl+L** | 清除历史 |
| **Shift+drag** | 选中文本（如果鼠标被捕获 — 用 `/mouse off` 恢复正常选中） |
| **鼠标滚轮** | 滚动视口 |

---

## 会话与历史

每次交互后，会话会自动保存到 `~/.config/execai/sessions/<uuid>.json`。重启时：
- 如果同一目录下有 24 小时以内的会话 — 会自动恢复
- 否则开新会话

`/sessions` — 列出所有会话。`/resume 3` — 打开第 3 个。`/resume <id>` — 按 ID 打开。

`Ctrl+R` — 模糊搜索：输入任意关键词 → 会搜索所有会话的**标题和内容**。

`/compact` — 如果会话太长导致上下文溢出，agent 会把较老的部分压缩成一段摘要，同时保留最后 6 轮。

输入历史（`↑/↓`）也会保存到 `~/.config/execai/input_history` — 重启后依然存在。

---

## 文件位置

| 操作系统 | 路径 |
|---|---|
| Linux   | `~/.config/execai/` |
| macOS   | `~/Library/Application Support/execai/` |
| Windows | `%APPDATA%\execai\` |

| 文件 | 内容 |
|---|---|
| `config.json` | api_base、selected_model_id、thinking_budget（effort） |
| `credentials.json` | JWT（权限 0600） |
| `host_id` | 稳定的 machine-id（仅在 /etc/machine-id 等不可用时作为回退） |
| `subscriptions.json` | 已连接的订阅（key 是明文 — 请妥善保管） |
| `permissions.json` | 持久化的工具白名单 |
| `sessions/<uuid>.json` | 每个会话的历史 |
| `memory/MEMORY.md` | 用户记忆索引（见 [记忆](#agent-记忆)） |
| `memory/*.md` | 单条事实（user/feedback/project/reference） |
| `input_history` | 你最近 200 条输入 |
| `requests.log` | LLM 请求日志（供 `/log` 使用） |
| `auth-poll.log` | Device-flow 登录诊断日志（用于调试） |
| `models_cache.json` | 模型目录缓存 — 断网时依然能让 execai 离线启动 |
| `installed_arch_sha` | 自动更新检查用的架构 SHA |
| `last_remote_sha` | 版本比较时间戳 |

### 记忆 — 见专门章节

[Agent 记忆](#agent-记忆) — 用户 + 项目结构，每次会话自动加载到 system prompt。

---

## 故障排查

**安装后 `execai: command not found`**：
- 当前会话里：`exec bash -l` 或者开个新终端
- 或者手动：`export PATH="$HOME/.local/bin:$PATH"`

**`/source zai` 时提示 "zai subscription is not connected"**：
- 先 `/connect zai <key>`。key 在哪拿 — 见 [源](#源--execai-与订阅) 章节

**Z.ai 上出现 429 "Insufficient balance"**：
- 你用的是普通 API-key，不是 Coding Plan key。去这里拿正确的 Coding Plan key：https://z.ai/manage-apikey/apikey-list（Individual Coding Plan 部分）

**聊天中出现 404 "Not found"（ExecAI）**：
- 网关可能不认识这个端点。反馈给开发者 — 可能需要重新部署 aicore-vbai。

**TUI 打不开、黑屏**：
- Windows 上用 **Windows Terminal**，不要用老的 conhost。Linux 上 — 任意现代终端（gnome-terminal、kitty、alacritty）。
- Defender：`Add-MpPreference -ExclusionPath "$env:LOCALAPPDATA\execai"`，然后重装。

**编码错乱**（比如 `Ð�escribe` 而不是 `Describe`）：
- 老版本 bubbles 的 bug。更新 execai：`curl -fsSL .../install.sh | bash`。

**自动更新一直不到**：
- 启动时底部状态栏会显示 `✓ execai R5.X — latest version` 或 `🔔 New version available`。
- 如果不工作 — `EXECAI_UPDATE_CHANNEL=R5` 可以覆盖 channel。

**图片明明写了路径却没发出去**：
- 路径里有空格或非 ASCII 字符时，检查是否用引号（单引号或双引号）括起来：`'/path/to/Screenshot.png'`
- 或者路径没有空格时不用引号：`/path/to/foo.png`
- 如果 AI 还说 "没图片" — 检查模型，必须是 vision 模型（sonnet/gpt-5/glm-5v 等）

**请求卡住**：
- `Ctrl+C` 取消当前请求。历史会保留。

**聊天里出现 "Reached N iterations limit"**：
- 这**不是**错误 — 是达到 tool-use 迭代上限（默认 40）后的软停止。
- 直接说 "continue" — agent 会再取一批同样大小的迭代，从上次停下的地方继续。
- 如果任务是自主型且很大 — 提高上限：`/max-iterations 100`（范围 1-500）。
- 会保存到 `~/.config/execai/config.json`，对之后的对话生效。

**"context deadline exceeded" / 模型加载不出来**：
- 自 R5.67 起，execai **始终**能启动，哪怕没有网络。
- 首次运行时会从 API 拉取目录并缓存到 `~/.config/execai/models_cache.json`。
- 断网时 — 使用缓存并显示 `ℹ Using cached catalog`。
- 如果连缓存都没有 — 会启用内置回退（Claude Sonnet 4.6），TUI 能打开，真实请求可能失败但界面是活的。

---

## 支持

- Bug/功能请求：[github.com/velesbsdllc/agent-vbai/issues](https://github.com/velesbsdllc/agent-vbai/issues)
- ExecAI 文档：https://chat.execai.ru/

---

**execai** — by ExecAI/VBAI。类 MIT 许可证。
