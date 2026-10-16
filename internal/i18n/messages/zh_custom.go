// Package messages/zh_custom — Chinese strings for live model lists and the
// custom provider (/connect custom). Merged into the "zh" catalog.
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var zhCustomMessages = map[string]string{
	"catalog.cliDefault":               "默认",
	"catalog.cliDefaultDesc":           "不传 --model：使用 CLI 自身设置中的模型",
	"subs.connect.modelsCount":         "提供商返回的模型：%d 个 — 用 /model 选择。",
	"subs.connect.noModels":            "提供商没有返回模型列表。请自行指定：/model <id> — 会为此来源记住。",
	"subs.connect.ollamaLocalFailed":   "✗ %s 上的 Ollama 没有响应（HTTP %d）。是否已启动？`ollama serve`",
	"subs.connect.ollamaCloudRejected": "✗ ollama.com 拒绝了密钥（HTTP %d）。密钥：https://ollama.com/settings/keys",
	"ui.model.listEmpty":               "此来源尚未返回任何模型。请指定：/model <id>",
	"ui.model.typed":                   "模型 %q 不在提供商的列表中 — 按输入使用，并为此来源记住。",
	"hint.connect.custom":              "自定义提供商：任意 OpenAI 或 Anthropic 兼容地址",
	"subs.connectHint.custom":          "自定义提供商：协议、地址、密钥 — 分步填写",
	"subs.hint.custom":                 "自定义（%s）· %s",
	"subs.custom.usage": "用法：\n" +
		"  /connect custom                                — 分步填写\n" +
		"  /connect custom <名称> <openai|anthropic> <base_url> [api_key] [--models a,b] [--header \"名称: 值\"]\n" +
		"示例：\n" +
		"  /connect custom \"LM Studio\" openai http://localhost:1234/v1\n" +
		"  /connect custom deepseek openai https://api.deepseek.com sk-xxx\n" +
		"  /connect custom corp anthropic https://llm.corp.local sk-xxx --header \"X-Tenant: dev\"",

	"custom.wiz.intro":              "连接自定义提供商 — 任何提供 OpenAI 或 Anthropic 风格 API 的服务器。Esc 取消。",
	"custom.wiz.name":               "第 1/4 步。叫什么名字？例如：LM Studio、vLLM、deepseek、corp-gateway（使用拉丁字母 — 名称会成为 /source 的 id）。",
	"custom.wiz.idIs":               "id：%s（之后用 /source <id> 切换）",
	"custom.wiz.api":                "第 2/4 步。接口类型：\n  1 — OpenAI 兼容（POST …/chat/completions）：LM Studio、vLLM、llama.cpp、LiteLLM、DeepSeek、Groq、Together、Mistral…\n  2 — Anthropic 兼容（POST …/v1/messages）：LiteLLM、Claude 网关、MiniMax、DeepSeek…\n输入 1 或 2。",
	"custom.wiz.urlOpenAI":          "第 3/4 步。服务器地址（base URL），例如 http://localhost:1234/v1、http://gpu-box:8000/v1、https://api.deepseek.com",
	"custom.wiz.urlAnthropic":       "第 3/4 步。服务器地址（base URL），不含 /v1/messages，例如 https://llm.corp.local、https://api.deepseek.com/anthropic",
	"custom.wiz.key":                "第 4/4 步。API 密钥 — 没有就直接回车（本地服务器通常没有）。仅保存在本机，不会写入输入历史。",
	"custom.wiz.noKey":              "（无密钥）",
	"custom.wiz.keyRejected":        "✗ 服务器拒绝了密钥（HTTP %d）。请重新输入，或按 Esc 取消。",
	"custom.wiz.models":             "服务器没有返回模型列表（GET …/models：HTTP %d）。请输入以逗号分隔的模型 ID，或回车跳过，之后用 /model <id> 指定。",
	"custom.wiz.cancelled":          "已取消连接自定义提供商。",
	"custom.wiz.placeholder.name":   "提供商名称，例如 LM Studio",
	"custom.wiz.placeholder.api":    "1 — OpenAI 兼容，2 — Anthropic 兼容",
	"custom.wiz.placeholder.url":    "base URL，例如 http://localhost:1234/v1",
	"custom.wiz.placeholder.key":    "API 密钥，没有就回车",
	"custom.wiz.placeholder.models": "以逗号分隔的模型 ID，或回车",
	"custom.keyRejected":            "✗ %[2]s 拒绝了密钥（HTTP %[1]d）。",
	"custom.connected":              "✓ 已连接：%s（id %s，%s 协议，%s）",
	"custom.connectedModels":        "模型：%d 个，当前：%s — 用 /model 选择。",
	"custom.connectedHint":          "之后切回：/source %[1]s · 删除：/disconnect %[1]s",
}

func init() { i18n.Register("zh", zhCustomMessages) }
