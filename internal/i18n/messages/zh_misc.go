// Package messages/zh_misc — misc 批次的简体中文字符串（tui.go、plain REPL、
// compact、agent loop、welcome）。
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var zhMiscMessages = map[string]string{
	// === Boot / login flow (tui.go) ===
	"ui.boot.noLogin": "ℹ 正在不使用 ExecAI 账号工作 — 来源：%s · 模型：%s。\n" +
		"  /login — 连接 ExecAI 账号（我们的目录约 34 个模型 + 计费）。",
	"ui.login.intro": "你好！要登录，需要在浏览器里确认这个 agent（类似 gh auth login）。\n" +
		"如果 device-flow 因某些原因不可用 — 可以把 JWT 令牌（eyJ…）粘贴到这里并按 Enter。\n\n" +
		"ℹ ExecAI 账号是可选的：可以不登录，直接用自己的订阅工作。\n" +
		"  /connect kimi <key>   → /source kimi     (Kimi Code)\n" +
		"  /connect zai <key>    → /source zai      (Z.ai GLM)\n" +
		"  /connect openai <key> → /source openai   (OpenAI API)\n" +
		"  还有：anthropic、kimi-api、claude-cli、codex-cli、ollama — /connect 会显示全部。",
	"ui.login.staleToken":          "旧令牌在 %s 上无效。正在启动 device-flow 重新登录…",
	"ui.login.startFlow":           "正在启动 device-flow 重新登录…",
	"ui.login.deviceFlowOpen":      "在浏览器中打开并确认：\n\n  %s\n\n验证码（如需手动输入）：%s\n\n按 Enter / [y] 自动打开浏览器。[n] 手动打开。",
	"ui.boot.modelsFallbackFailed": "既无法获取模型列表，也无法组装 fallback 列表",
	"ui.welcome":                   "execai %s · %s/%s · %s · %s\n请输入任务。/model — 模型，/help — 命令，/quit — 退出。",

	// === Stream errors / token expiry ===
	"ui.stream.tokenExpiredHint": "→ 切换 /source zai|ollama|anthropic，或用 /login 重新确认。",
	"ui.stream.tokenExpiredFlow": "ExecAI 令牌已过期 — 正在启动 device-flow。请在浏览器中确认。",

	// === /compact ===
	"ui.compact.historyNote": "[此前已压缩的历史（%d 条消息）：%s]",
	"ui.compact.done":        "📦 历史已压缩：%d 条消息 → 1 份摘要（约 %d 字符）",
	"ui.compact.working":     "正在压缩历史…",
	"ui.compact.tooShort":    "历史还太短 — 没什么可压缩的（需要 >%d 条消息）",
	"ui.compact.truncated":   "…（已截断）",
	"ui.compact.promptSystem": "你是 AI agent 的上下文压缩器。你会收到一段对话的 transcript。" +
		"请返回一份简短摘要（≤500 词），保留：\n" +
		"  • 关键决策及其原因\n" +
		"  • 重要的文件路径和命令\n" +
		"  • 之后可能有用的 tool-call 结果\n" +
		"  • 出现的错误及其解决方式\n" +
		"省略闲聊和确认。用中文书写，电报式风格。",
	"ui.compact.promptUser": "压缩这段对话：\n\n%s",

	// === Autoloop ===
	"ui.autoloop.defaultPrompt": "继续",
	"ui.autoloop.wake":          "🌙 autoloop：%s 后唤醒（%s）→ 提示词：%q",

	// === /paste ===
	"ui.paste.empty":     "本会话中还没有粘贴。Ctrl+V 粘贴大段文本 → 生成标记。",
	"ui.paste.header":    "粘贴（Ctrl+V ≥200 字符或含 \\n）：\n",
	"ui.paste.showHint":  "\n查看：/paste show <N>",
	"ui.paste.notNumber": "不是数字：%s",
	"ui.paste.notFound":  "没有 #%d 号粘贴",
	"ui.paste.usage":     "用法：/paste [list|show <N>]",

	// === /whoami ===
	"ui.whoami.notLoggedIn": "（未登录 — /login）",

	// === /classic & /mouse ===
	"ui.classic.on":  "✓ classic TUI 已开启 — 重启 execai（/quit → execai）。Alt-screen + 固定状态栏，Shift+拖动 复制。",
	"ui.classic.off": "✓ Ink 风格（默认）— 重启 execai。历史在 scrollback 中，原生选择和滚动。",
	"ui.mouse.off":   "🖱  鼠标捕获 OFF — 鼠标可选中文本，菜单不响应点击。开启：/mouse on",
	"ui.mouse.on":    "🖱  鼠标捕获 ON — 滚轮滚动，可点击菜单。选中文本：Shift+拖动。关闭：/mouse off",

	// === /effort ===
	"ui.effort.pickerHint": "effort 选择器：←/→ 选择，Enter 确认，Esc 取消",
	"ui.effort.current":    "当前 effort：%s（%d tokens）\n修改：/effort <off|low|medium|high|max>\n  off=0  low=1024  medium=4096  high=8192  max=32000\n适用于 Anthropic 兼容来源（Z.ai、Kimi、Anthropic、ollama-cloud、claude-cli）。",
	"ui.effort.set":        "✓ effort=%s（%d tokens）",

	// === /max-iterations ===
	"ui.maxIter.current": "当前 max iterations：%d\n每轮 tool-use 迭代上限。用尽时 — 软停止，用户可以说“继续”。\n修改：/max-iterations <N>（建议 20-200；默认 50）",
	"ui.maxIter.usage":   "/max-iterations <N>，N 取 1 到 500（建议 20-200）",

	// === /loop ===
	"ui.loop.status":      "🔁 loop：每 %s — %q。/loop stop 停止",
	"ui.loop.inactive":    "loop 未激活。用法：/loop <间隔> <提示词>（示例：/loop 5m 检查构建状态）",
	"ui.loop.notRunning":  "loop 本来就没在运行",
	"ui.loop.stopped":     "🔁 loop 已停止",
	"ui.loop.usage":       "/loop <间隔> <提示词>（例如：/loop 5m 检查构建状态）",
	"ui.loop.badInterval": "无法解析间隔 %s — 需要类似 30s、5m、1h 的格式",
	"ui.loop.started":     "🔁 loop 已启动：每 %s — %q\n停止：/loop stop",

	// === /log ===
	"ui.log.none":   "还没有日志：%s",
	"ui.log.header": "📜 最近 %d 次请求（%s）：\n",

	// === /usage & /cd ===
	"ui.usage.fetchFailed": "无法获取 usage：%s",
	"ui.cd.current":        "当前 cwd：%s",

	// === Sessions ===
	"ui.session.new":         "新会话",
	"ui.sessions.header":     "\n已保存的会话（最新在上）：\n",
	"ui.sessions.empty":      "（空）\n",
	"ui.sessions.switchHint": "\n切换：/resume <编号|id>",
	"ui.resume.notFound":     "找不到会话 %q。/sessions — 查看列表",
	"ui.resume.loadFailed":   "加载失败：%s",
	"ui.resume.resumed":      "继续会话：%s",
	"ui.title.renamed":       "已重命名：%s",

	// === /permissions ===
	"ui.perms.toolsEmpty": "  always allowed tools: （空）\n",
	"ui.perms.exactCount": "  always allowed exact commands: %d 条记录\n",
	"ui.perms.resetHint":  "\n要重置 — 手动删除该文件或运行：rm ~/.config/execai/permissions.json",

	// === /model ===
	"ui.model.notFound": "找不到模型 %q",
	"ui.model.switched": "已切换到 %s/%s — %s（历史已保留）",

	// === Approve ===
	"ui.approve.denied":       "已拒绝",
	"ui.approve.allowedTool":  "已允许：本会话中 %s 的所有调用",
	"ui.approve.allowedExact": "已允许：本会话中的这条命令",
	"ui.approve.navHint":      "← → 或 Tab — 切换，Enter — 选择，Esc — 拒绝",

	// === Tool-call summaries ===
	"ui.toolSummary.write": "Write %s （%d 字节）",

	// === Plain REPL (chat.go) ===
	"plain.err.fetchModels":    "无法获取模型列表",
	"plain.err.emptyModels":    "服务器返回了空的模型列表",
	"plain.err.pickModelEmpty": "无法选择模型（列表为空？）",
	"plain.err.pickModel":      "无法选择模型",
	"plain.commands":           "命令：/model — 选择模型，/clear — 清空历史，/quit — 退出。",
	"plain.historyCleared":     "（历史已清空）",
	"plain.modelSwitchHint":    "\n切换：/model <编号> 或 /model <model_name>",
	"plain.modelNotFound":      "（找不到模型 %q。/model — 查看列表）",
	"plain.modelSwitched":      "（已切换到 %s/%s — %s；历史已保留）",
	"plain.errorPrefix":        "错误：",
	"plain.modelsHeader":       "\n可用模型（★ — primary，• — current）：",

	// === Agent loop (internal/agent) ===
	"loop.iterationLimit": "⚠ 已达到 %d 次迭代上限 — 任务尚未完成。说“继续”再给 %d 次，或者换个说法。",

	// === Welcome screen (first launch) ===
	"welcome.text": `你好！这是 execai — 面向开发的 CLI agent。

能做什么：
  • 读/写/编辑文件（Read、Write、Edit）
  • 搜索（Grep、Glob、LS、Tree）
  • 执行 shell 命令（Bash）— 只读操作无需确认，其余需要确认
  • 发起 HTTP 请求（WebFetch — 无浏览器；真正的浏览器会另行推出）
  • 维护 to-do 列表（TodoWrite）

记忆：
  • ./EXECAI.md           — 项目记忆（仓库上下文）
  • ~/.config/execai/EXECAI.md — 你的个人设置
两个文件每次会话都会自动加载进 system prompt。

命令：
  /model               — 模型列表
  /model <num|子串>     — 切换（历史保留）
  /clear               — 清空历史
  /help                — 本条消息
  /quit                — 退出

提示：Enter — 发送，Shift+Enter — 换行。`,

	// === WebSearch / WebFetch tools ===
	"tool.websearch.noLogin": "网络搜索不可用：搜索通过 ExecAI 网关进行，需要 ExecAI 账号。\n" +
		"未登录时仍可使用本地浏览器——用 WebFetch 打开任意网址，并沿着返回的链接继续访问。\n" +
		"执行 /login 即可启用搜索（同时获得 ExecAI 模型目录）。",
	"tool.websearch.sources": "来源：",
	"tool.websearch.empty":   "搜索没有返回结果。请换一种说法，或用 WebFetch 直接打开具体页面。",

	// === AskUser picker + субагенты ===
	"ask.title":             "智能体提问：",
	"ask.hint":              "↑↓ 选择 · Enter 确认 · 1-4 直接选 · Esc — 交给智能体决定",
	"ask.answered":          "问题：%s → %s",
	"ask.dismissed":         "交由智能体决定",
	"ask.dismissedForModel": "用户关闭了问题且未做选择。请自行决定，说明所采用的假设，然后继续。",
	"subagent.emptyResult":  "子智能体没有返回结果",

	// === /key — ключ шифрования памяти ===
	"hint.key":  "记忆加密密钥（查看 / 导出 / 导入 / 创建）",
	"key.usage": "/key — 查看状态 · /key new — 创建 · /key export — 显示私钥 · /key import <密钥> — 安装来自另一台机器的密钥",
	"key.absent": "尚未创建加密密钥。\n" +
		"它会在首次同步时自动创建，也可以现在用 /key new 创建。\n" +
		"位置：%s",
	"key.present": "加密密钥已就位。\n" +
		"公钥（可以公开；别人用它为你加密）：\n" +
		"  %s\n" +
		"文件：%s",
	"key.created": "加密密钥已创建。\n" +
		"公钥：%s\n" +
		"文件：%s",
	"key.alreadyExists": "密钥已存在（公钥：%s）。未做替换——那会导致用它加密的一切都无法访问。",
	"key.noRecoveryWarning": "⚠ 没有找回机制。我们没有你的密钥，将来也不会有。\n" +
		"一旦丢失，已同步的记忆将无法解读，只能从零重建。\n" +
		"\n" +
		"现在就保存一份副本：/key export，并放进密码管理器。\n" +
		"要在另一台机器上使用同一份记忆，请在那里执行 /key import <密钥>。",
	"key.exportWarning": "⚠ 下面是你的私钥。持有它的人就能读取你已同步的记忆。不要粘贴到聊天、issue 或截图中。",
	"key.exportHint":    "公钥部分（这个可以公开）：%s",
	"key.imported":      "密钥已安装。公钥：%s",
	"key.importFailed": "无法安装该密钥：%v\n" +
		"\n" +
		"如果你想替换已有密钥，请先删除它，但要确认已有副本，\n" +
		"否则用旧密钥加密的一切都将无法解读：%s",
	"key.invalid": "这看起来不是私钥（%v）。期望格式：AGE-SECRET-KEY-1…",
	"key.error":   "密钥操作失败：%v",

	// === /memory — импорт и экспорт памяти ===
	"hint.memory":               "智能体记忆：从其他智能体导入、导出",
	"hint.memoryFind":           "先搜自己的记忆，若为空则搜本机其他智能体的",
	"memory.usage":              "/memory — 查看状态 · /memory import — 导入此处其他智能体的记忆 · /memory export [目录] — 以 markdown 导出记忆 · /memory find <关键词> — 先搜自己的记忆，再搜其他智能体的",
	"memory.findUsage":          "要搜索什么？例如：/memory find 定价",
	"memory.findMine":           "🧠 在自己的记忆中找到 %d 条，关键词「%s」：",
	"memory.findForeign":        "📁 自己的记忆里没有，但本机另一个智能体有 %d 条，关键词「%s」：",
	"memory.findNothing":        "自己的记忆和其他智能体的记忆里都没有找到：「%s」",
	"memory.findImportQuestion": "把它收进我的记忆吗？（%d 个文件）",
	"memory.status":             "记忆：%d 条，位于 %s",
	"memory.foundNearby":        "在此目录发现其他智能体的记忆文件：%d 个",
	"memory.importHint":         "\n导入它们：/memory import",
	"memory.nothingToImport":    "在 %s 未发现其他智能体的记忆。\n已查找：CLAUDE.md、.claude/、AGENTS.md、.cursorrules、.cursor/rules/、.github/copilot-instructions.md、EXECAI.md",
	"memory.importQuestion":     "要将 %d 个文件导入智能体记忆吗？",
	"memory.importYes":          "导入",
	"memory.importYesDesc":      "内容将成为记忆的一部分，而记忆之后会同步到服务器（用你的密钥加密）",
	"memory.importNo":           "取消",
	"memory.importNoDesc":       "不读取、不复制任何内容",
	"memory.importCancelled":    "已取消导入，未复制任何内容。",
	"memory.imported":           "已导入：%d",
	"memory.exported":           "已导出 %d 个文件到 %s",
	"memory.error":              "记忆操作失败：%v",

	// === /project — привязка каталога к проекту ===
	"hint.project":              "把当前目录绑定到网页聊天中的项目",
	"project.usage":             "/project — 项目列表 · bind <名称> — 绑定当前目录 · unbind — 解除绑定 · on/off — 在项目中启用/停用代理",
	"project.listHeader":        "你的项目（● 已绑定到当前目录，○ 绑定在别处）：",
	"project.listHint":          "绑定：/project bind <名称>",
	"project.none":              "还没有项目——请在网页聊天中创建。",
	"project.defaultTag":        "[默认]",
	"project.bound":             "项目「%s」已绑定到 %s",
	"project.unbound":           "已解除 %s 的绑定",
	"project.notBound":          "没有项目绑定到 %s",
	"project.notFound":          "未找到项目「%s」。可用：%s",
	"project.needLogin":         "需要 ExecAI 账号 —— /login",
	"project.error":             "项目操作失败：%v",
	"project.serveQuestion":     "启动后台监听器以接收网页聊天的任务？",
	"project.serveYes":          "启动",
	"project.serveYesDesc":      "工具遵循你的 permissions.json；关闭终端后仍继续运行",
	"project.serveReadOnly":     "只读",
	"project.serveReadOnlyDesc": "可以查看，但不能改文件、不能跑命令",
	"project.serveNo":           "暂不",
	"project.serveNoDesc":       "稍后手动执行：execai serve",
	"project.serveStarted":      "▶ 监听器已启动（pid %d）。状态：execai serve --status · 停止：--stop",
	"project.serveLog":          "  输出：%s",
	"project.serveSkipped":      "未启动监听器 —— 网页任务会排队等待。启动：execai serve",
	"project.serveFailed":       "无法启动监听器：%v",
	"project.serveAlready":      "▶ 监听器已在运行（pid %d）",
	"project.agentOn":           "[代理已启用]",
	"project.agentOff":          "[代理已停用]",
	"project.enabled":           "代理「%s」已在项目「%s」中启用 —— 它会接收该项目的任务",
	"project.disabled":          "代理「%s」已在项目「%s」中停用 —— 它不会接收该项目的任务",
	"project.notAgent":          "当前会话不是代理，无内容可添加。请在目标机器上使用 /login 登录。",
	"project.notInProject":      "代理不在项目「%s」中 —— 请先执行 /project bind",
	"project.boundNoTool":       "目录 %[2]s 已绑定到项目「%[1]s」，但将代理加入项目失败：%[3]v",
}

func init() {
	i18n.Register("zh", zhMiscMessages)
}
