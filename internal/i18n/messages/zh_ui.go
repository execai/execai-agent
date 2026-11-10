// Package messages/zh_ui — Chinese strings for execai ui (the agent panel in
// a browser). Merged into the "zh" catalog via i18n.Register.
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var zhUIMessages = map[string]string{
	"ui.cmd.short": "在浏览器中打开智能体面板（仅限本机）",
	"ui.cmd.long": "与 VS Code 和 ExecAI Studio 中相同的聊天面板，在 127.0.0.1 的浏览器标签页中打开。\n\n" +
		"地址中带有一次性密钥：首次访问时换成 cookie，之后该链接不再打开任何内容。" +
		"只有本机可以连接。来自网站的任务和套餐检查与编辑器面板相同。停止：Ctrl+C。",
	"ui.flag.cwd":       "项目目录（默认：当前目录）",
	"ui.flag.port":      "127.0.0.1 上的端口（默认：空闲端口）",
	"ui.flag.noOpen":    "不打开浏览器，只打印地址",
	"ui.badCwd":         "不是目录：%s",
	"ui.address":        "ExecAI 面板：%s",
	"ui.hint":           "链接只能使用一次——打开它的浏览器在 execai ui 运行期间保持访问。停止：Ctrl+C。",
	"ui.openFailed":     "无法打开浏览器（%v）——请手动打开链接。",
	"ui.stopped":        "面板已停止。",
	"ui.http.linkUsed":  "此链接已被使用。请在首次打开它的浏览器中打开面板，或重新启动 execai ui 以获取新链接。",
	"ui.http.noSession": "请打开 execai ui 在终端中打印的链接。",
}

func init() { i18n.Register("zh", zhUIMessages) }
