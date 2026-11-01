// Package messages/zh_fleet — 子智能体与“谁在做什么”列表的中文文本（E101）。
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var zhFleetMessages = map[string]string{
	"fleet.ask.prefix":      "🤖 智能体「%s」请求：",
	"fleet.card.write":      "可修改",
	"fleet.card.background": "后台",

	"fleet.state.queued":    "排队中",
	"fleet.state.running":   "工作中",
	"fleet.state.waiting":   "等待你的回答",
	"fleet.state.done":      "已完成",
	"fleet.state.failed":    "出错",
	"fleet.state.cancelled": "已停止",

	"fleet.started":          "子智能体 %s（「%s」）已在后台启动。完成后结果会作为单独消息送达；如需现在等待，请用 id %s 调用 TaskWait。",
	"fleet.result.timeout":   "子智能体未能在 %s 内完成",
	"fleet.result.cancelled": "子智能体随其父智能体一起停止",
	"fleet.result.stopped":   "子智能体已被用户停止",
	"fleet.result.limit":     "（在 %d 步上限处停止——结果可能不完整）",
	"fleet.wait.none":        "没有需要等待的后台子智能体",
	"fleet.wait.unknown":     "没有子智能体 %s",
	"fleet.note.header":      "后台子智能体已完成：",
	"fleet.note.item":        "—「%s」（%s），%s：",
	"fleet.err.model":        "此处无法选择模型 %q——子智能体使用 %s 的当前模型",
	"fleet.err.cwdOutside":   "cwd %s 超出了父智能体目录 %s",
	"fleet.err.cwdMissing":   "cwd %s 不是目录",

	"fleet.tui.header":          "🤖 智能体",
	"fleet.tui.more":            "… 还有 %d 个 — /agents",
	"fleet.tui.listHeader":      "子智能体（最新在下）：",
	"fleet.tui.listEmpty":       "还没有子智能体。智能体通过 Task 工具启动它们。",
	"fleet.tui.listHint":        "/agents open <n> — 记录 · /agents stop <n> — 停止",
	"fleet.tui.openHeader":      "智能体「%s」· %s · %s",
	"fleet.tui.openEmpty":       "（记录暂为空）",
	"fleet.tui.notFound":        "没有智能体 %s — 见 /agents",
	"fleet.tui.stopping":        "正在停止智能体「%s」",
	"fleet.tui.notRunning":      "智能体「%s」已经结束",
	"fleet.tui.usage":           "用法：/agents · /agents open <n> · /agents stop <n>",
	"fleet.tui.autoReply":       "🤖 后台智能体已完成——结果已交给智能体",
	"fleet.tui.cardDone":        "%s 智能体「%s」— %s",
	"fleet.tui.approveAgent":    "🤖 由智能体「%s」请求",
	"fleet.tui.stoppedChildren": "已停止的子智能体：%d",
	"status.agents":             "智能体",
	"hint.agents":               "子智能体：列表 · open <n> · stop <n>",

	"fleet.ide.notFound": "没有这个智能体：%s",
	"fleet.ide.stopping": "正在停止智能体「%s」",
}

func init() { i18n.Register("zh", zhFleetMessages) }
