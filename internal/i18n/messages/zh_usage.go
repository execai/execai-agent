// Package messages/zh_usage — /usage 命令输出的简体中文字符串。
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var zhUsageMessages = map[string]string{
	// === /usage 主区块（ExecAI 计费） ===
	"usage.header":       "📊 USAGE",
	"usage.plan":         "套餐：%s（%s）— %s",
	"usage.active":       "有效",
	"usage.notActive":    "未激活",
	"usage.until":        "至 %s",
	"usage.wallet":       "钱包：%s ₽",
	"usage.limits":       "限额：",
	"usage.remainingRub": "（剩余 %.0f ₽）",
	"usage.remainingFmt": "（剩余 %s）",
	"usage.noEvents":     "（无事件）",
	"usage.iterations":   "AI 迭代（最近 %d 次 · 共 %.2f ₽）：",

	// === 限额窗口标签 ===
	"usage.window.5h":    "5小时",
	"usage.window.day":   "天",
	"usage.window.week":  "周",
	"usage.window.month": "月",

	// === 重置倒计时（humanDuration） ===
	"usage.resetIn.sec":     "%d秒后",
	"usage.resetIn.min":     "%d分钟后",
	"usage.resetIn.hour":    "%d小时后",
	"usage.resetIn.hourMin": "%d小时%d分钟后",
	"usage.resetIn.day":     "%d天后",
	"usage.resets":          "%s重置",
	"usage.refreshing":      "正在重置",

	// === Kimi Code Coding Plan ===
	"usage.kimi.header":         "🔌 Kimi Code (kimi.com/code)",
	"usage.kimi.plan":           "套餐：%s（按可用模型判断）",
	"usage.kimi.models":         "模型：%s",
	"usage.kimi.rollingWindows": "滚动窗口：",
	"usage.kimi.manage":         "ⓘ 订阅管理：%s",

	// === 滚动窗口单位标签 ===
	"usage.unit.hour": "%d小时",
	"usage.unit.min":  "%d分钟",
	"usage.unit.day":  "%d天",
	"usage.window.n":  "窗口 #%d",

	// === Anthropic 兼容 source（本地 requests.log 计数器） ===
	"usage.source.header":        "🔌 %s 源",
	"usage.source.emptyLog":      "🔌 %s 源\n\n本地请求日志为空。实际配额：%s",
	"usage.source.noRequests":    "（本地日志中没有请求）",
	"usage.source.quotaAt":       "实际配额请查看 %s",
	"usage.source.totalRequests": "日志中请求总数：    %d",
	"usage.source.totalInput":    "input tokens 总计：  %s",
	"usage.source.totalOutput":   "output tokens 总计： %s",
	"usage.source.cacheTokens":   "Cache tokens：       %s",
	"usage.source.last24h":       "最近 24 小时：       %d 个请求 · %s input · %s output",
	"usage.source.byModel":       "按模型：",
	"usage.source.modelLine":     "%-22s  %4d 次 · ↓%s ↑%s",
	"usage.source.errors":        "  ⚠ %d 个错误",
	"usage.localCounterNote":     "ⓘ 这是本地计数器。实际计费和配额：",
}

func init() {
	i18n.Register("zh", zhUsageMessages)
}
