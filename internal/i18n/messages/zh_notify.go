// Package messages/zh_notify — Chinese strings of the terminal notifications
// (/notifications, E105).
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var zhNotifyMessages = map[string]string{
	"hint.notifications":     "本轮结束或智能体等待时提醒：system / bell / off / test",
	"hint.notificationsTest": "立即发送一条测试通知",

	"notify.turnDone":   "ExecAI：本轮已完成 — %s",
	"notify.approve":    "ExecAI 正在等待确认：%s",
	"notify.ask":        "ExecAI 有一个问题",
	"notify.approveAny": "ExecAI 正在等待确认",
	"notify.test":       "ExecAI：测试通知",

	// Background work (/loop, wake-ups): its end, an error, the step limit.
	"notify.bgDone":    "ExecAI：后台工作已完成 — %s",
	"notify.bgStopped": "ExecAI：后台工作已停止 — %s",
	"notify.bgError":   "ExecAI：后台一轮出错结束 — %s",
	"notify.bgLimit":   "ExecAI：后台一轮达到步数上限 — %s",

	"notify.title":         "通知 — 当你在其他窗口时，本轮结束或智能体等待你：",
	"notify.desc.system":   "桌面通知（终端自带通知时用终端的）和响铃",
	"notify.desc.bell":     "只通过终端：终端通知和响铃 — 不启动任何程序，可通过 SSH 使用",
	"notify.desc.off":      "不通知",
	"notify.here":          "在这台电脑上：%s",
	"notify.bell":          "响铃",
	"notify.focus.known":   "终端会报告焦点：只有在其窗口处于后台时才会通知。",
	"notify.focus.unknown": "终端没有报告焦点：距离你最后一次按键 30 秒以上的一轮，或 15 秒无人回答的问题，仍会通知。",
	"notify.howto":         "更改：/notifications system | bell | off · 试一下：/notifications test\n此设置只保留在这台电脑上 — 同步不会带走它。",
	"notify.set":           "通知：%s — %s",
	"notify.unknown":       "未知模式「%s」— 可用 system、bell 或 off。",
	"notify.offNow":        "通知已关闭 — /notifications system 可开启。",
	"notify.testSent":      "测试通知已发送：%s",
}

func init() { i18n.Register("zh", zhNotifyMessages) }
