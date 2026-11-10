// Package messages/zh_sync — Chinese strings for sync between computers
// (/sync, execai sync). Merged into the "zh" catalog via i18n.Register.
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var zhSyncMessages = map[string]string{
	"hint.sync":        "在你的多台电脑之间同步记忆和设置",
	"sync.usage":       "/sync — 状态 · /sync on · off · now · password · forget · keys export · keys import",
	"sync.placeholder": "回答 — Enter，取消 — Esc",
	"sync.working":     "请稍候…",
	"sync.cancelled":   "已取消。",
	"sync.error":       "同步：出错：%v",
	"sync.failed":      "无法连接同步服务器：%v",

	// === gate ===
	"sync.gate.noPlan": "同步功能包含在 «Work» 和 «Max» 套餐中：%s",
	"sync.gate.login":  "要使用同步，请登录 ExecAI：运行 execai 并在浏览器中确认（聊天中：/login）。",

	// === status ===
	"sync.status.off":         "同步已关闭。开启：%[1]s on — 记忆、设置和服务商密钥会在你所有的电脑上保持一致（«Work» 和 «Max» 套餐）。",
	"sync.status.on":          "同步已开启 · 上次交换：%s",
	"sync.status.onNever":     "同步已开启 · 尚未交换",
	"sync.status.what":        "同步内容：%s — 全部在这台电脑上加密。",
	"sync.status.pausedPlan":  "已暂停：同步功能包含在 «Work» 和 «Max» 套餐中：%s",
	"sync.status.pausedLogin": "已暂停：请登录 ExecAI（聊天中：/login）。",
	"sync.status.pausedKey":   "已暂停：另一台电脑更换了同步密钥。请再次输入同步密码：%s on",
	"sync.status.unreadable":  "服务器上有 %d 个文件在这里无法打开 — 它们用另一个密钥加密。请输入同步密码：%s on",
	"sync.status.tooBig":      "文件太大，只保留在这里：%s",
	"sync.status.quota":       "服务器上的同步空间已用完 — 新文件不再发送。",
	"sync.status.lastError":   "上次错误：%s",
	"sync.status.commands":    "命令：%[1]s now · %[1]s off · %[1]s password · %[1]s forget · %[1]s keys export | import",

	"sync.what.memory":   "记忆（文件：%d）",
	"sync.what.projects": "项目（%d）",

	// === on ===
	"sync.on.newIntro":       "这是你第一台开启同步的电脑。记忆、设置和服务商密钥在这台电脑上加密：服务器只保存加密后的副本，我们看不到里面的内容。",
	"sync.on.newPassword":    "请设置同步密码（至少 %d 个字符）。在你的其他电脑上需要输入它。密码无法找回。",
	"sync.on.repeatPassword": "再次输入密码：",
	"sync.on.joinIntro":      "你的另一台电脑已经开启了同步。",
	"sync.on.enterPassword":  "请输入第一台电脑上设置的同步密码：",
	"sync.on.keyDiffers":     "这台电脑已经有自己的加密密钥，与服务器上的密钥不同。用一个密钥加密的记忆无法用另一个密钥读取。",
	"sync.on.chooseKey": "1 — 使用服务器上的密钥（你的密钥会保存在旁边）\n" +
		"2 — 保留你的密钥并替换服务器上的：服务器上的所有内容将由这台电脑覆盖，其他电脑需要再次输入密码",
	"sync.on.oldKeySaved": "原来的密钥已保存：%s",
	"sync.on.raced":       "在此期间，另一台电脑更换了服务器上的密钥。请重试：%s on",
	"sync.on.racedJoin":   "另一台电脑刚刚设置了同步密码 — 请输入它。",
	"sync.on.created":     "同步已开启。",
	"sync.on.joined":      "已获取密钥，同步已开启。",
	"sync.on.replaced":    "同步已开启；服务器上现在是这台电脑的数据。",
	"sync.on.done":        "同步已开启。",
	"sync.on.firstFailed": "首次交换失败：%v。10 分钟后会自动重试，或者现在：%s now",

	// === password ===
	"sync.password.wrong":    "密码不正确。",
	"sync.password.short":    "太短：至少需要 %d 个字符。",
	"sync.password.mismatch": "两次输入的密码不一致，请重新输入。",
	"sync.password.new":      "新的同步密码（至少 %d 个字符，无法找回）：",
	"sync.password.changed":  "密码已更改。其他电脑无需任何操作 — 只有添加新电脑时才需要密码。",

	// === now / off ===
	"sync.now.done":    "完成：收到 %d，发送 %d。",
	"sync.now.off":     "同步已关闭。开启：%s on",
	"sync.off.already": "同步本来就是关闭的。开启：%s on",
	"sync.off.done":    "这台电脑上的同步已关闭。服务器上的数据仍然保留；删除：%[1]s forget。重新开启：%[1]s on",

	// === forget ===
	"sync.forget.warn":      "将从服务器删除加密密钥、所有已同步的记忆、设置和服务商密钥以及电脑列表。你电脑上的内容都会保留。此操作无法撤销。",
	"sync.forget.confirm":   "要删除，请输入「%s」：",
	"sync.forget.word":      "删除",
	"sync.forget.cancelled": "未删除。",
	"sync.forget.done":      "已从服务器删除，文件：%d。你电脑上的内容都还在；其他电脑会在下次检查时关闭同步。重新开启：%s on",

	// === background ===
	"sync.bg.paused":     "同步已暂停 — 这台电脑上的记忆和设置不受影响。",
	"sync.bg.resumed":    "同步已恢复。",
	"sync.bg.wiped":      "同步数据已从服务器删除（由另一台电脑操作）— 这里的同步已关闭。重新开启：%s on",
	"sync.bg.keyChanged": "同步已暂停：另一台电脑更换了密钥。请再次输入同步密码：%s on",

	// === keys ===
	"sync.keys.nothing":  "没有可迁移的内容：这台电脑上没有连接任何服务商密钥。",
	"sync.keys.exported": "服务商密钥：%s。\n不经服务器、无需套餐的迁移。这是迁移代码 — 在另一台电脑上粘贴（%[3]s keys import）：\n\n%[2]s",
	"sync.keys.phrase": "代码对应的短语 — 请与代码分开、通过另一种方式发送：\n\n    %s\n\n" +
		"代码和短语合在一起就能打开你的密钥：不要给任何人看。",
	"sync.keys.importIntro": "从另一台电脑迁移服务商密钥（在那台电脑上：keys export）。",
	"sync.keys.enterCode":   "粘贴迁移代码：",
	"sync.keys.enterPhrase": "输入 %d 个单词的短语：",
	"sync.keys.badCode":     "这看起来不像迁移代码 — 请完整复制。",
	"sync.keys.badPhrase":   "短语不正确。",
	"sync.keys.added":       "已添加：%s",
	"sync.keys.updated":     "已更新（那台电脑上的密钥更新）：%s",
	"sync.keys.kept":        "未变：%s",
	"sync.keys.active":      "当前源未改变：%s。切换：/source <名称>",

	// === execai sync ===
	"sync.cmd.short":            "在你的多台电脑之间同步记忆和设置",
	"sync.cmd.long":             "代理的记忆、设置、权限和服务商密钥在你所有的电脑上保持一致。所有内容都在电脑上用同步密码加密；服务器只保存加密副本，我们看不到里面的内容。\n没有套餐时，可以直接迁移服务商密钥：execai sync keys export / import。\n\n同步功能包含在 «Work» 和 «Max» 套餐中。",
	"sync.cmd.status":           "同步状态（默认）",
	"sync.cmd.on":               "在这台电脑上开启同步",
	"sync.cmd.off":              "在这台电脑上关闭同步（服务器上的数据保留）",
	"sync.cmd.now":              "立即同步",
	"sync.cmd.password":         "更改同步密码",
	"sync.cmd.forget":           "从服务器删除所有内容",
	"sync.cmd.keys":             "不经服务器在电脑之间迁移服务商密钥",
	"sync.cmd.keysExport":       "显示密钥迁移代码及其短语",
	"sync.cmd.keysImport":       "通过代码和短语接收另一台电脑的密钥",
	"sync.cmd.needTerminal":     "需要终端：答案需要手动输入。",
	"sync.what.config":          "设置和服务商密钥",
	"sync.status.keysFallback":  "不开启同步时，也可以直接迁移服务商密钥 — 不经服务器、无需套餐：%s keys export",
	"sync.pulledSource":         "另一台电脑切换了当前源：%s",
	"sync.status.projects":      "项目：%d（%s）",
	"sync.project.off":          "开启同步后会同步项目文件（.execai/）：%s on",
	"sync.project.pulled":       "已从服务器收到项目文件（.execai/）。",
	"sync.project.pushed":       "项目文件（.execai/）已加密发送到服务器。",
	"sync.project.same":         "项目文件（.execai/）与服务器一致。",
	"sync.project.empty":        "项目里还没有 .execai/ 文件 — 出现后会自动同步。",
	"sync.project.backup":       "这里和另一台电脑都修改了项目 — 较新的副本胜出，另一份保存在旁边：%s",
	"project.published":         "已在网页聊天中创建项目「%s」。",
	"project.publishedNotBound": "已在网页聊天中创建项目「%s」，但未能绑定当前目录：%v",
	"hint.projectPublish":       "用当前目录创建网页聊天项目",
	"project.cmd.short":         "项目：电脑上的目录 ↔ 网页聊天中的项目",
	"project.cmd.publish":       "用当前目录创建网页聊天项目，绑定并发送 .execai/",
}

func init() { i18n.Register("zh", zhSyncMessages) }
