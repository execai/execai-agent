// Package messages/zh_machine — 入门引导（/start）、本机卡片（/machine）和手机链接（/phone）的中文文本。
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var zhMachineMessages = map[string]string{
	"hint.start":        "入门：五个步骤以及已完成的部分",
	"hint.machine":      "本机：模型、同步、可以从哪里调用",
	"hint.machineCheck": "检查模型来源是否有响应",
	"hint.phone":        "二维码：用手机调用这台机器",

	// === /start ===
	"start.title":    "入门",
	"start.progress": "已完成 %d / %d",
	"start.allDone":  "一切就绪：这台机器已完全配置好。",
	"start.footer": "本机：/machine · 所有命令：/help · 再次查看此列表：/start\n" +
		"Enter — 发送，Shift+Enter — 换行。",

	"start.login.title": "登录 ExecAI",
	"start.login.why":   "账号把这台机器与网页聊天、你的手机和你的其他电脑连接起来。",
	"start.login.cmd":   "输入：/login — 然后在浏览器中确认。",

	"start.models.title": "连接模型",
	"start.models.why":   "你自己的服务商密钥或 ExecAI 套餐 — 记忆和手机调用在两种方式下都能使用。",
	"start.models.cmd":   "输入：/connect <服务商> — 使用自己的密钥，/plan — ExecAI 套餐，/machine check — 检查来源是否有响应。",

	"start.sync.title":    "开启同步",
	"start.sync.why":      "让你所有的电脑拥有相同的设置和记忆。",
	"start.sync.what":     "会同步（在这台电脑上用你的同步密码加密）：包含服务商密钥的配置、你的个人记忆、已绑定项目的文件。",
	"start.sync.notWhat":  "留在本机：api_base（这台机器的服务器地址）和登录信息。",
	"start.sync.password": "同步密码只有你知道：我们不保存它，也无法找回。",
	"start.sync.cmd":      "输入：/sync on（「Work」和「Max」套餐）。",

	"start.project.title": "把文件夹绑定到网页项目",
	"start.project.why":   "文件夹会成为网页聊天中的项目：来自网页的任务在其中执行，项目记忆是共享的。",
	"start.project.cmd":   "输入：/project publish — 用这个文件夹创建项目，/project bind <名称> — 绑定到已有项目。",

	"start.phone.title": "用手机调用智能体",
	"start.phone.why":   "从网页聊天或手机给这台机器派任务 — 即使它使用的是你自己的密钥。",
	"start.phone.cmd":   "输入：/phone — 显示手机用的二维码。要接收任务，请运行 execai serve。",

	// === /machine ===
	"machine.title":               "本机",
	"machine.label.machine":       "机器",
	"machine.label.account":       "账号",
	"machine.label.models":        "模型",
	"machine.label.sync":          "同步",
	"machine.label.presence":      "可访问性",
	"machine.identity":            "%s · %s · 智能体 %s",
	"machine.account.in":          "%s · 套餐：%s",
	"machine.account.noPlan":      "%s · 没有 ExecAI 套餐",
	"machine.account.planPending": "%s · 正在确认套餐",
	"machine.account.out":         "未登录 — /login",

	"machine.source.alive":     "有响应（检查于 %s）",
	"machine.source.down":      "无响应（检查于 %s）",
	"machine.source.unchecked": "尚未检查 — /machine check",
	"machine.source.needLogin": "需要登录 — /login",
	"machine.source.noPlan":    "没有有效套餐 — /plan",
	"machine.source.active":    "使用中",

	"machine.sync.off":      "已关闭 — /sync on",
	"machine.sync.on":       "已开启 · 最近上传：%s",
	"machine.sync.never":    "已开启 · 尚未上传",
	"machine.sync.paused":   "已暂停 — 详情：/sync",
	"machine.sync.inside":   "包含：%s",
	"machine.sync.config":   "配置和服务商密钥",
	"machine.sync.memory":   "个人记忆（文件：%d）",
	"machine.sync.projects": "项目：%d",
	"machine.sync.empty":    "暂无内容",
	"machine.sync.mode":     "模式：%s",
	"machine.mode.all":      "所有项目",
	"machine.mode.selected": "仅已绑定的文件夹",
	"machine.mode.none":     "不含项目",

	"machine.presence.serve":      "接收任务：是 — execai serve（pid %d）",
	"machine.presence.ide":        "接收任务：是 — 编辑器窗口（pid %d）",
	"machine.presence.noListener": "接收任务：否 — 请运行 execai serve",
	"machine.presence.reachable":  "从网页和手机调用：可以 — /phone",
	"machine.presence.blocked":    "从网页和手机调用：暂不可以 — %s",
	"machine.block.login":         "请登录（/login）",
	"machine.block.plan":          "需要「Work」或「Max」套餐",
	"machine.block.planPending":   "正在确认套餐",
	"machine.block.listener":      "请运行 execai serve",

	"machine.footer":   "重新检查来源：/machine check · 入门：/start",
	"machine.checking": "正在检查来源…",

	// === /phone ===
	"phone.title":     "用手机调用这台机器",
	"phone.scan":      "用手机摄像头扫描二维码，或打开链接：",
	"phone.what":      "该页面会打开网页聊天，并已选好这台机器来执行任务。",
	"phone.needLogin": "登录后会显示手机链接：/login",
	"phone.listener":  "要接收任务，这台机器需要运行 execai serve（在单独的窗口中）。",
	"phone.plan":      "来自网页和手机的任务包含在「Work」和「Max」套餐中：%s",
	"phone.qrFailed":  "无法绘制二维码：%v",
}

func init() { i18n.Register("zh", zhMachineMessages) }
