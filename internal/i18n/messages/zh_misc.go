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
	"ui.maxIter.current": "当前 max iterations：%d\n每轮 tool-use 迭代上限。用尽时 — 软停止，用户可以说“继续”。\n修改：/max-iterations <N>（建议 20-200；默认 40）",
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
}

func init() {
	i18n.Register("zh", zhMiscMessages)
}
