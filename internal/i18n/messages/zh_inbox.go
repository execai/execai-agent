// Package messages/zh_inbox — 「网页 → 智能体」通道的中文字符串：
// 由 execai serve 或编辑器面板接收的网站任务。
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var zhInboxMessages = map[string]string{
	"inbox.err.parse": "无法解析服务器响应",

	"inbox.lock.busy":         "收件箱锁不断易主，请重试",
	"inbox.lock.heldByIDE":    "网站任务已由一个编辑器窗口接收（pid %d，文件夹 %s）。请在该窗口关闭 ExecAI 面板，或从该窗口运行任务。",
	"inbox.lock.heldByServe":  "网站任务已由后台进程 execai serve 接收（pid %d）。",
	"inbox.lock.signalFailed": "无法向进程 %d 发送信号：%v",
	"inbox.lock.stopTimeout":  "execai serve（pid %d）在 %s 内未退出——可能正在完成任务。请重试，或运行：execai serve --stop --force",

	"inbox.plan.refused":   "从网站运行任务包含在 Work 和 Max 套餐中：%s",
	"inbox.plan.serveWarn": "⚠ 你的套餐不包含从网站运行任务——每个任务都会被拒绝：",

	"inbox.result.interrupted": "（已中断：%s）",
	"inbox.result.noText":      "（智能体已完成，但没有给出文本回答）",

	"inbox.task.bindingsFailed": "无法获取项目绑定：%v",
	"inbox.task.dirMissing":     "未找到项目目录：%s",
	"inbox.task.noDir":          "该项目未设置目录——在此绑定：execai → /project bind",
	"inbox.task.notBound":       "本机未绑定到该任务的项目——运行：execai → /project bind",
	"inbox.task.empty":          "空任务",
	"inbox.task.stopped":        "已在编辑器中停止",
	"inbox.task.timeout":        "任务未在 %s 内完成",
	"inbox.task.editorClosed":   "任务完成前编辑器已关闭",

	"inbox.serve.refused":     "✗ 任务 %s 被拒绝：套餐不包含网站任务",
	"inbox.serve.statusIDE":   "未运行：网站任务由一个编辑器窗口接收（pid %d，文件夹 %s）",
	"inbox.serve.stopIDE":     "网站任务由编辑器窗口接收（pid %d，文件夹 %s），而不是 execai serve——请关闭该窗口",
	"inbox.ide.otherProject":  "另一个项目（%s）的任务正在等待——请在编辑器中打开它或运行 execai serve",
	"inbox.ide.anotherWindow": "网站任务由另一个编辑器窗口接收：%s",
	"inbox.ide.taken":         "现在由此窗口接收网站任务",
	"inbox.ide.bindWhich":     "哪个项目？请传入其 id（list_workspaces 会列出）",
}

func init() { i18n.Register("zh", zhInboxMessages) }
