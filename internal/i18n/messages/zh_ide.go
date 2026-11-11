// Package messages/zh_ide — Chinese strings the agent sends to the editor
// panel (execai ide): the permission buttons and the list of sources to
// connect.
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var zhIDEMessages = map[string]string{
	// The descriptions under the permission buttons; the buttons themselves
	// are approve.* (shared with the terminal chat).
	"ide.approve.onceDesc":    "只允许这一次调用",
	"ide.approve.sessionDesc": "直到此聊天结束",
	"ide.approve.exactDesc":   "完全相同的调用，直到此聊天结束",
	"ide.approve.foreverDesc": "记住此权限 — 会写入 permissions.json",
	"ide.approve.denyDesc":    "不执行；智能体会收到拒绝",

	"ide.connect.withKey":    "%s（密钥）",
	"ide.connect.noKey":      "%s（无需密钥）",
	"ide.connect.zaiAPI":     "Z.ai 平台，按 token 计费（密钥）",
	"ide.connect.openrouter": "OpenRouter — 一个密钥使用所有厂商（密钥）",
	"ide.connect.ollama":     "Ollama — 本地或云端（云端需要密钥）",
	"ide.connect.custom":     "自定义提供商 — 任何兼容 OpenAI 或 Anthropic 的地址",

	// Notices and errors the panel shows (execai ide), the security levels
	// and the errors of the sign-in and connect paths from other packages.
	"ide.err.effort":            "effort: off|low|medium|high|max 或数字",
	"ide.err.positive":          "需要一个正数",
	"ide.maxIter.set":           "每轮工具迭代上限：%d",
	"ide.login.startFailed":     "无法开始登录",
	"ide.login.saveFailed":      "无法保存令牌",
	"ide.login.expired":         "确认码已过期 — 请重试",
	"ide.login.timeout":         "登录未及时确认",
	"ide.connect.keyRejectedBy": "%s 拒绝了密钥（HTTP %d）",
	"ide.connect.noModels":      "已连接：%s（%s）— 服务器没有返回模型列表（HTTP %d）；请重新连接并填写模型 ID",
	"ide.connect.ollamaKey":     "ollama.com 未接受密钥（HTTP %d）",
	"ide.connect.ollamaDown":    "%s 上的 Ollama 没有响应（HTTP %d）— 它在运行吗？",
	"ide.connect.unknown":       "未知来源：%s",
	"ide.connect.needKey":       "需要提供商密钥",
	"ide.connect.keyRejected":   "密钥被拒绝（HTTP %d）— 请确认它是 %s 的密钥",
	"ide.connect.done":          "已连接：%s",
	"ide.connect.models":        " — 可用模型：%d",
	"ide.connect.unverified":    "（密钥已保存；将在第一次请求时校验）",
	"ide.turn.queued":           "本轮已排队：其他聊天仍在回答",
	"ide.err.chdir":             "无法进入目录 %s",
	"ide.err.noModel":           "无法选择模型 — 请在 execai 中检查 /model",
	"ide.err.noModelShort":      "没有模型",
	"ide.err.badMessage":        "无法解析消息：%s",
	"ide.err.noChat":            "没有这个聊天：%s",
	"ide.subs.unreadable":       "无法读取 subscriptions.json（%s）— 正在使用基础来源；订阅没有丢失，但需要修复该文件",
	"ide.err.noSuchModel":       "当前来源没有这个模型：%s",
	"ide.err.unknownLevel":      "未知级别：%s",
	"ide.security.set":          "安全级别：%s",
	"ide.logout.done":           "已退出 ExecAI（订阅仍保留）",
	"ide.disconnect.done":       "已断开：%s",
	"ide.resume.none":           "没有可继续的 — 这是项目中的第一个聊天",
	"ide.resume.failed":         "无法打开最近的聊天：%s",
	"ide.chat.openFailed":       "无法打开聊天：%s",
	"ide.wake.none":             "没有安排唤醒",
	"ide.err.unknownCommand":    "未知命令: %s",
	"ide.err.unknownType":       "未知类型：%s",
	"ide.chat.busy":             "这个聊天仍在回答 — 请等待本轮结束或停止它",
	"security.level.light":      "轻度 — 不打扰",
	"security.level.deep":       "标准 — 项目内静默，项目外先询问",
	"security.level.paranoid":   "严格 — 一切都询问",
	"connect.err.api":           "不支持接口类型 %q：openai（chat/completions）或 anthropic（messages）",
	"connect.err.noURL":         "需要服务器地址，例如 http://localhost:1234/v1",
	"connect.err.badURL":        "地址 %q 不像 http(s) URL",
	"subs.err.nameLatin":        "名称需要包含拉丁字母或数字（/source 的 id 由它生成）",
	"subs.err.nameLong":         "名称超过 40 个字符",
	"subs.err.builtin":          "%q 是内置来源 — 请换一个名称",
	"subs.err.notLoaded":        "订阅未加载",
	"subs.err.notConnected":     "订阅 %q 未连接 — 请先 /connect %s",
	"auth.err.linkStart":        "agent-link/start 返回 %d：%s",
	"auth.err.linkPoll":         "agent-link/poll 返回 %d：%s",
	"auth.err.pollFailed":       "轮询连续失败 10 次",
	"auth.err.linkExpired":      "链接已过期 — 请重新运行 /login",
}

func init() { i18n.Register("zh", zhIDEMessages) }
