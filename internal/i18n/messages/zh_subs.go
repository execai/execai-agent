// Package messages/zh_subs — 订阅/来源命令的简体中文文案
// (/connect、/disconnect、/source、/subscriptions)。
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var zhSubsMessages = map[string]string{
	// === /disconnect ===
	"subs.noSubs":              "暂无订阅",
	"subs.disconnect.notFound": "未找到订阅 %q",
	"subs.disconnect.none":     "没有已连接的订阅。",
	"subs.disconnect.header":   "/disconnect <provider>。已连接：\n",
	"subs.saveError":           "保存失败：%s",
	"subs.disconnected":        "✓ %s 已断开",

	// === /source（列表）===
	"subs.useDeprecated":     "ℹ /use — 已弃用，请使用 /source\n\n",
	"subs.source.listHeader": "可用来源 (/source <name>)：\n",
	"subs.source.listExecai": "  • execai  — 我们的计费（默认）\n",
	"subs.source.listItem":   "  • %-8s — 订阅 %s\n",
	"subs.source.listFooter": "\n提示：输入 '/source ' 并按 Tab 打开菜单。",

	// === /source <name> 失败时的指引 ===
	"subs.howto.zai": "\n\n如何连接 Z.ai：\n" +
		"  1. 在 https://z.ai/manage-apikey/apikey-list 获取密钥\n" +
		"     （Individual Coding Plan → Plan Overview 栏目）\n" +
		"  2. /connect zai sk-zai-XXXXXX\n" +
		"  3. /source zai",
	"subs.howto.anthropic": "\n\n如何连接 Anthropic：\n" +
		"  1. 在 https://console.anthropic.com/settings/keys 获取 API key\n" +
		"     （格式 sk-ant-...，按 token 计费）\n" +
		"  2. /connect anthropic sk-ant-XXXXXX\n" +
		"  3. /source anthropic",
	"subs.sourceSwitched": "✓ 来源：%s · 模型：%s",

	// === /connect（无参数）===
	"subs.connect.usageShort": "用法：/connect <provider> <api_key>\n" +
		"支持：zai (Z.ai Coding Plan)\n" +
		"提示：'/connect ' + Tab 打开提供商菜单。",

	// === /subscriptions ===
	"subs.list.empty": "没有已连接的订阅。\n" +
		"连接：/connect zai (Z.ai Coding Plan)\n" +
		"当前来源：ExecAI（我们的计费）",
	"subs.list.header":         "已连接的订阅（当前激活：%s）：\n",
	"subs.list.switchHint":     "\n切换：/source <provider>  （或 /source execai → 我们的计费）\n",
	"subs.list.disconnectHint": "断开：/disconnect <provider>",

	// === 提供商描述（/source 菜单）===
	"subs.provider.execai":     "我们的计费（默认）",
	"subs.provider.zai":        "Z.ai GLM Coding Plan",
	"subs.provider.kimi":       "Kimi Code Coding Plan（K3/K2.7，kimi.com/code 订阅）",
	"subs.provider.kimiapi":    "Moonshot Platform（按 token 计费，platform.moonshot.ai）",
	"subs.provider.anthropic":  "Anthropic API（sk-ant-…）",
	"subs.provider.openai":     "OpenAI Platform（platform.openai.com 的 sk-… 密钥，按 token 计费）",
	"subs.provider.openrouter": "OpenRouter（openrouter.ai 的 sk-or-… — 一个密钥调用所有厂商，按量计费）",
	"subs.provider.codexcli":   "本地 OpenAI Codex CLI（ChatGPT Plus/Pro 配额）",
	"subs.provider.claudecli":  "本地 Claude Code（Pro/Max 订阅配额）",
	"subs.provider.ollama":     "本地 Ollama runner（localhost:11434）",

	// === 菜单提示 ===
	"subs.hint.connected":     "已连接",
	"subs.hint.connectedPlan": "已连接 · %s",
	"subs.hint.notConnected":  "未连接 — /connect %s",
	"subs.hint.remove":        "删除订阅",

	// === /connect 菜单提示 ===
	"subs.connectHint.zai":        "Z.ai GLM Coding Plan（Coding Plan API key）",
	"subs.connectHint.kimi":       "Kimi Code Coding Plan（K3/K2.7，密钥来自 kimi.com/code/console）",
	"subs.connectHint.kimiapi":    "Moonshot Platform 按 token 计费（密钥来自 platform.moonshot.ai）",
	"subs.connectHint.anthropic":  "Claude API（sk-ant-...，来自 console.anthropic.com）",
	"subs.connectHint.openai":     "OpenAI Platform 按 token 计费（sk-…，来自 platform.openai.com）",
	"subs.connectHint.openrouter": "OpenRouter 按量计费：一个密钥用 Claude、GPT、Gemini、DeepSeek",
	"subs.connectHint.codexcli":   "本地 OpenAI Codex CLI（无需密钥，需已安装 `codex`）",
	"subs.connectHint.claudecli":  "本地 Claude Code（无需密钥，需已安装 `claude`）",
	"subs.connectHint.ollama":     "本地 Ollama runner（localhost:11434，无需密钥）",

	// === /connect 流程 ===
	"subs.connect.usage": "用法：/connect <provider> <api_key> [base_url]\n" +
		"支持：zai (Z.ai Coding Plan)\n" +
		"示例：  /connect zai sk-zai-XXXXX\n" +
		"        /connect zai sk-zai-XXXXX https://open.bigmodel.cn/api/paas/v4  （CN 区域）",
	"subs.connect.claudecliOK": "✓ claude-cli 已连接（通过 Claude Code OAuth 使用你的 Pro/Max 订阅配额）。\n" +
		"切换：/source claude-cli\n" +
		"\n" +
		"限制：\n" +
		"  • execai-tools（Bash/Read/Write）不可用 — claude CLI 运行它自己的 tools。\n" +
		"  • 模型管理 — 需在外部执行 `claude config set defaultModel <id>`。\n" +
		"  • 历史记录以纯文本 prompt 传递（无 session-id）。",
	"subs.connect.codexcliOK": "✓ codex-cli 已连接（通过 OpenAI Codex OAuth 使用你的 ChatGPT Plus/Pro 订阅配额）。\n" +
		"切换：/source codex-cli\n" +
		"\n" +
		"要求：\n" +
		"  • PATH 中已安装 `codex`（github.com/openai/codex）。\n" +
		"  • 已用你的 ChatGPT 账号执行 `codex login`。\n" +
		"\n" +
		"限制：\n" +
		"  • execai-tools 不可用 — codex 运行它自己的 tools。\n" +
		"  • 无流式输出 — codex exec 返回最终文本。",
	"subs.connect.ollamaHelp": "Ollama — 两种连接模式：\n" +
		"\n" +
		"🌩  CLOUD（ollama.com）：\n" +
		"   模型 glm-5.2、qwen3-coder-30b 等运行在其服务器上。\n" +
		"   Anthropic 兼容 endpoint。需要 API 密钥：https://ollama.com/settings/keys\n" +
		"   用法：  /connect ollama <api-key>\n" +
		"\n" +
		"🏠  LOCAL（你自己的 Ollama）：\n" +
		"   模型通过 `ollama pull <name>` 获取，在本地运行，免费。\n" +
		"   OpenAI 兼容 endpoint。无需密钥。\n" +
		"   用法：  /connect ollama local\n" +
		"          /connect ollama local http://192.168.1.10:11434  （自定义 URL）",
	"subs.connect.ollamaLocalOK": "✓ ollama (local) 已连接：%s\n模型（%d）：  %s\n\n切换：/source ollama",
	"subs.connect.ollamaCloudOK": "✓ ollama (cloud) 已连接：%s\n" +
		"模型：glm-5.2、qwen3-coder-30b、kimi-k2 等（见 https://ollama.com/library）\n" +
		"\n切换：/source ollama\n" +
		"更换模型：  /model",
	"subs.connect.unsupported": "暂不支持提供商 %q。可用：zai、anthropic、openai、kimi（Kimi Code Coding Plan）、kimi-api（Moonshot Platform 按 token 计费）、claude-cli、codex-cli、ollama",

	"subs.connect.example.zai":        "示例：/connect zai sk-zai-XXXXX",
	"subs.connect.example.anthropic":  "示例：/connect anthropic sk-ant-XXXXX  （密钥来自 https://console.anthropic.com/settings/keys）",
	"subs.connect.example.kimi":       "示例：/connect kimi sk-XXXXX  （Kimi Code Coding Plan，来自 https://www.kimi.com/code/console）",
	"subs.connect.example.kimiapi":    "示例：/connect kimi-api sk-XXXXX  （Moonshot Platform 按 token 计费，来自 https://platform.moonshot.ai/console/api-keys）",
	"subs.connect.example.openai":     "示例：/connect openai sk-proj-XXXXX  （密钥来自 https://platform.openai.com/api-keys）",
	"subs.connect.example.openrouter": "示例：/connect openrouter sk-or-v1-XXXXX （密钥来自 https://openrouter.ai/keys）",
	"subs.connect.keyRequired":        "必须提供 API key。%s",

	"subs.connect.kimiApiRejected": "✗ 密钥被 api.moonshot.ai 拒绝（HTTP %d）。\n" +
		"你可能把 Kimi Code 密钥当成了 Moonshot Platform 密钥。\n\n" +
		"两种密钥不同：\n" +
		"  • Kimi Code（订阅）：       /connect kimi <key>  — 来自 https://www.kimi.com/code/console\n" +
		"  • Moonshot 按 token 计费：  /connect kimi-api <key> — 来自 https://platform.moonshot.ai/console/api-keys",
	"subs.connect.openaiRejected": "✗ 密钥被 api.openai.com 拒绝（HTTP %d）。\n" +
		"请检查：密钥是否完整复制、是否仍然有效（未被吊销）。\n" +
		"获取密钥：https://platform.openai.com/api-keys",
	"subs.connect.availableModels": "\n  可用模型：%s",
	"subs.connected.via":           "✓ %s 已通过 %s 连接。%s\n切换：  /source %s",
	"subs.connected":               "✓ %s 已连接。切换：  /source %s",

	// === Z.ai open platform (zai-api) + ToS-предупреждение по Coding Plan ===
	"subs.provider.zaiapi":        "Z.ai 开放平台（按量计费，api.z.ai）",
	"subs.connectHint.zaiapi":     "Z.ai 按量计费（密钥来自 z.ai/manage-apikey）——不限制工具",
	"subs.connect.example.zaiapi": "示例：/connect zai-api XXXXX （Z.ai 开放平台密钥，见 https://z.ai/manage-apikey/apikey-list）",
	"subs.connect.zaiApiRejected": "✗ api.z.ai 拒绝了该请求（HTTP %d）。\n" +
		"Z.ai 的 Coding Plan 与开放平台共用同一把密钥，\n" +
		"因此这几乎不是密钥填错，而是按量计费余额为零：\n" +
		"订阅与开放平台是分开计费的。\n" +
		"\n" +
		"充值：https://z.ai/manage-apikey/apikey-list，或继续通过 /connect zai\n" +
		"使用订阅（参见关于其条款的提示）。",
	"subs.connect.zaiToSWarning": "⚠ 关于 GLM Coding Plan 的使用条款。\n" +
		"Z.ai 将该订阅限定在一份封闭的工具清单内（Claude Code、Cursor、Cline、Roo Code、\n" +
		"OpenCode、Pi、Crush、Goose 等），execai 不在其中。\n" +
		"按其政策，可能被限流、冻结账号，累计三次违规后封禁。承担风险的是你的账号，\n" +
		"不是我们的，请自行斟酌。\n" +
		"\n" +
		"无限制的替代方案 —— Z.ai 开放平台，按量计费：\n" +
		"  /connect zai-api <key>   — 同样的 GLM 模型，没有工具清单限制。",
	"subs.connect.zaiKeyReused":       "复用 Coding Plan 的密钥，两者是同一把",
	"subs.connect.openrouterRejected": "✗ openrouter.ai 拒绝了该密钥（HTTP %d）。\n  需要来自 https://openrouter.ai/keys 的 sk-or-… 密钥。",
}

func init() { i18n.Register("zh", zhSubsMessages) }
