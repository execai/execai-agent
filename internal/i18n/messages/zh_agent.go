// Package messages/zh_agent — Chinese strings of the agent loop and
// the model catalog that the person sees: a declined tool, the scope of a
// permission, working without the catalog. Merged into the "zh" catalog
// via i18n.Register.
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var zhAgentMessages = map[string]string{
	"agent.tool.declined":          "用户拒绝执行此工具。",
	"agent.tool.noApprovalChannel": "execai 在此无法为该调用请求确认",
	"agent.approve.scope":          "该许可将适用于：%s",
	"llm.offline.fallback":         "⚠ 离线运行 — 模型目录不可用，使用内置列表。原因：%v",
	"llm.offline.cache":            "ℹ 使用缓存的模型目录（已存在 %s）。网络不可用：%v",
	"llm.models.unauthorized":      "/billing-vbai/models_public 返回 401 — 登录已过期，请重新登录（聊天中：/login）",
	"llm.models.status":            "模型目录返回 %d：%s",
	"llm.models.notJSON":           "模型目录的响应不是列表：%s",
	"llm.models.none":              "既没有网络，也没有已保存的模型目录",
	"llm.err.unauthorized":         "%s 返回 401 — %s",
	"llm.err.status":               "服务器返回 %d：%s",
	"llm.err.server":               "服务器返回错误：%v",
	"llm.err.aichatUnauthorized":   "/aichat-vbai/stream 返回 401 — 令牌已过期，请运行 execai login",
	"llm.err.sse":                  "SSE 流中的服务器错误：%v",
	"llm.err.anthropicKey":         "Anthropic API 返回 401 — 密钥错误。请检查 /subscriptions",
	"llm.err.providerKey":          "%s 返回 401 — API 密钥错误。请检查 /subscriptions 和 /connect",
	"llm.err.providerStatus":       "%s 返回 %d：%s",
	"llm.billing.limit":            "⏱  额度已用完",
	"llm.billing.funds":            "💰 余额不足",
	"llm.billing.inactive":         "⚠  套餐未激活",
	"llm.billing.unavailable":      "由于计费原因，服务暂时不可用。",
	"llm.billing.hint":             "更换套餐：ExecAI 网页账户中的「套餐」部分（%s）",
	"llm.cli.claudeMissing":        "未找到 claude CLI（PATH、~/.local/bin、VS Code/Cursor 扩展）。安装：https://docs.claude.com/en/docs/claude-code/quickstart",
	"llm.cli.claudeNotFound":       "未找到 claude CLI",
	"llm.cli.claudeNoModels":       "claude 没有返回任何模型",
	"llm.cli.codexMissing":         "在 PATH 中未找到 codex CLI。安装：https://github.com/openai/codex\n安装后：codex login",
	"llm.cli.codexNotFound":        "在 PATH 中未找到 codex CLI",
	"llm.cli.codexNoModels":        "codex 没有返回任何模型",
	"llm.cli.noAnswer":             "CLI 未作答就退出了",
	"llm.cli.emptyResult":          "结果为空",
	"llm.cli.emptyAnswer":          "回答为空",
	"llm.cli.images":               "图片",
	"llm.cli.noToolResult":         "没有结果：claude 提前退出",
	"llm.ollama.down":              "Ollama %s 不可用",
	"llm.ollama.start":             "请启动：ollama serve",
	"llm.ollama.noModel":           "Ollama：模型 %q 未安装（ollama pull %s?）。%s",
	"llm.ollama.status":            "Ollama 返回 %d：%s",
	"llm.ollama.tagsStatus":        "Ollama /api/tags 返回 %d：%s",
	"llm.ollama.tagsParse":         "Ollama /api/tags：无法解析响应",
	"llm.ollama.noModels":          "Ollama 中没有模型。请安装一个：`ollama pull llama3.2`",
	"llm.ollama.localModel":        "本地 Ollama 模型",
	"agent.memory.readFailed":      "读取记忆",
	"agent.memory.exportDirFailed": "创建导出文件夹",
	"agent.memory.writeFailed":     "写入 %s",
	"agent.permissions.saveFailed": "无法保存权限：",
	"auth.notSignedIn":             "你尚未登录。请运行：execai login",
}

func init() { i18n.Register("zh", zhAgentMessages) }
