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
	"fleet.result.timeout":   "子智能体未能在 %s 内完成（可用 EXECAI_SUBAGENT_TIMEOUT 修改上限，0 为不限）",
	"fleet.result.cancelled": "子智能体随其父智能体一起停止",
	"fleet.result.stopped":   "子智能体已被用户停止",
	"fleet.result.limit":     "（在 %d 步上限处停止——结果可能不完整；要从停下的地方继续，用 TaskExtend，id 为 %s）",
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
	"fleet.tui.listHint":        "/agents open <n> — 记录 · /agents stop <n> — 停止 · /agents more <n> [步数] — 增加步数 · /agents limit [N] — 新子智能体的步数",
	"fleet.tui.openHeader":      "智能体「%s」· %s · %s",
	"fleet.tui.openEmpty":       "（记录暂为空）",
	"fleet.tui.notFound":        "没有智能体 %s — 见 /agents",
	"fleet.tui.stopping":        "正在停止智能体「%s」",
	"fleet.tui.notRunning":      "智能体「%s」已经结束",
	"fleet.tui.usage":           "用法：/agents · /agents open <n> · /agents stop <n> · /agents more <n> [步数] · /agents limit [N]",
	"fleet.tui.autoReply":       "🤖 后台智能体已完成——结果已交给智能体",
	"fleet.tui.cardDone":        "%s 智能体「%s」— %s",
	"fleet.tui.approveAgent":    "🤖 由智能体「%s」请求",
	"fleet.tui.stoppedChildren": "已停止的子智能体：%d",
	"status.agents":             "智能体",
	"hint.agents":               "子智能体：列表 · open <n> · stop <n> · more <n> · limit <N>",
	"fleet.extend.raised":       "智能体「%s」增加了 %d 步——现在第 %d 步，共 %d 步",
	"fleet.extend.resumed":      "智能体「%s」从第 %d 步继续：再给 %d 步（上限 %d）。结果会单独发来，或通过 TaskWait（id %s）获取。",
	"fleet.extend.ceiling":      "智能体「%s」已是最大上限——%d 步",
	"fleet.extend.finished":     "智能体「%s」已经结束（%s），并非停在步数上限——无需延长；需要更多工作请新建 Task",
	"fleet.extend.prompt":       "你又获得了 %d 步。从停下的地方继续，完成任务，并在最后一条消息中给出完整结果。",
	"fleet.tui.steps":           "第 %d/%d 步 · 剩 %d",
	"fleet.tui.atLimit":         "已到步数上限",
	"fleet.tui.moreHint":        "步数不多了——/agents more %s",
	"fleet.tui.limitShow":       "新子智能体的步数：%d（默认 %d，允许 1–%d）。修改：/agents limit <N>",
	"fleet.tui.limitSet":        "新子智能体将获得 %d 步",
	"fleet.tui.limitBad":        "需要 1 到 %d 之间的数字",

	"fleet.ide.notFound": "没有这个智能体：%s",
	"fleet.ide.stopping": "正在停止智能体「%s」",
}

func init() { i18n.Register("zh", zhFleetMessages) }
