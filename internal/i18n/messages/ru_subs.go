// Package messages/ru_subs — Russian strings for subscription/source commands
// (/connect, /disconnect, /source, /subscriptions). Values are the original
// hardcoded strings from internal/chat/subs_commands.go, byte-for-byte.
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var ruSubsMessages = map[string]string{
	// === /disconnect ===
	"subs.noSubs":              "подписок нет",
	"subs.disconnect.notFound": "подписка %q не найдена",
	"subs.disconnect.none":     "Нет подключенных подписок.",
	"subs.disconnect.header":   "/disconnect <provider>. Подключены:\n",
	"subs.saveError":           "ошибка сохранения: %s",
	"subs.disconnected":        "✓ %s отключён",

	// === /source (bare list) ===
	"subs.useDeprecated":     "ℹ /use — deprecated, используй /source\n\n",
	"subs.source.listHeader": "Доступные источники (/source <name>):\n",
	"subs.source.listExecai": "  • execai  — наш биллинг (дефолт)\n",
	"subs.source.listItem":   "  • %-8s — подписка %s\n",
	"subs.source.listFooter": "\nПодсказка: набери '/source ' и Tab — будет меню.",

	// === /source <name> how-to on failure ===
	"subs.howto.zai": "\n\nКак подключить Z.ai:\n" +
		"  1. Возьми ключ на https://z.ai/manage-apikey/apikey-list\n" +
		"     (раздел Individual Coding Plan → Plan Overview)\n" +
		"  2. /connect zai sk-zai-XXXXXX\n" +
		"  3. /source zai",
	"subs.howto.anthropic": "\n\nКак подключить Anthropic:\n" +
		"  1. Возьми API-key на https://console.anthropic.com/settings/keys\n" +
		"     (формат sk-ant-... pay-per-token биллинг)\n" +
		"  2. /connect anthropic sk-ant-XXXXXX\n" +
		"  3. /source anthropic",
	"subs.sourceSwitched": "✓ источник: %s · модель: %s",

	// === /connect (bare) ===
	"subs.connect.usageShort": "Использование: /connect <provider> <api_key>\n" +
		"Поддерживается: zai (Z.ai Coding Plan)\n" +
		"Подсказка: '/connect ' + Tab — будет меню провайдеров.",

	// === /subscriptions ===
	"subs.list.empty": "Подключенных подписок нет.\n" +
		"Подключить: /connect zai (Z.ai Coding Plan)\n" +
		"Текущий источник: ExecAI (наш биллинг)",
	"subs.list.header":         "Подключенные подписки (активна: %s):\n",
	"subs.list.switchHint":     "\nПереключить: /source <provider>  (или /source execai → наш биллинг)\n",
	"subs.list.disconnectHint": "Отключить:   /disconnect <provider>",

	// === Provider descriptions (/source picker) ===
	"subs.provider.execai":    "наш биллинг (дефолт)",
	"subs.provider.zai":       "Z.ai GLM Coding Plan",
	"subs.provider.kimi":      "Kimi Code Coding Plan (K3/K2.7, подписка kimi.com/code)",
	"subs.provider.kimiapi":   "Moonshot Platform (pay-per-token, platform.moonshot.ai)",
	"subs.provider.anthropic": "Anthropic API (sk-ant-…)",
	"subs.provider.openai":    "OpenAI Platform (sk-… из platform.openai.com, pay-per-token)",
	"subs.provider.codexcli":  "локальный OpenAI Codex CLI (квота ChatGPT Plus/Pro)",
	"subs.provider.claudecli": "локальный Claude Code (квота Pro/Max-подписки)",
	"subs.provider.ollama":    "локальный Ollama runner (localhost:11434)",

	// === Picker hints ===
	"subs.hint.connected":     "подключено",
	"subs.hint.connectedPlan": "подключено · %s",
	"subs.hint.notConnected":  "не подключено — /connect %s",
	"subs.hint.remove":        "удалить подписку",

	// === /connect picker hints ===
	"subs.connectHint.zai":       "Z.ai GLM Coding Plan (Coding Plan API key)",
	"subs.connectHint.kimi":      "Kimi Code Coding Plan (K3/K2.7, ключ kimi.com/code/console)",
	"subs.connectHint.kimiapi":   "Moonshot Platform pay-per-token (ключ platform.moonshot.ai)",
	"subs.connectHint.anthropic": "Claude API (sk-ant-... из console.anthropic.com)",
	"subs.connectHint.openai":    "OpenAI Platform pay-per-token (sk-… из platform.openai.com)",
	"subs.connectHint.codexcli":  "локальный OpenAI Codex CLI (без ключа, нужна установленная `codex`)",
	"subs.connectHint.claudecli": "локальный Claude Code (без ключа, нужна установленная `claude`)",
	"subs.connectHint.ollama":    "локальный Ollama runner (localhost:11434, без ключа)",

	// === /connect flow ===
	"subs.connect.usage": "Использование: /connect <provider> <api_key> [base_url]\n" +
		"Поддерживается: zai (Z.ai Coding Plan)\n" +
		"Пример:  /connect zai sk-zai-XXXXX\n" +
		"         /connect zai sk-zai-XXXXX https://open.bigmodel.cn/api/paas/v4  (для CN)",
	"subs.connect.claudecliOK": "✓ claude-cli подключен (квота из твоей Pro/Max-подписки через OAuth Claude Code).\n" +
		"Переключись: /source claude-cli\n" +
		"\n" +
		"Ограничения:\n" +
		"  • execai-tools (Bash/Read/Write) НЕ работают — claude CLI запускает СВОИ tools.\n" +
		"  • Управление моделью — через `claude config set defaultModel <id>` снаружи.\n" +
		"  • История передаётся как plain-text промт (без session-id).",
	"subs.connect.codexcliOK": "✓ codex-cli подключен (квота из твоей ChatGPT Plus/Pro-подписки через OAuth OpenAI Codex).\n" +
		"Переключись: /source codex-cli\n" +
		"\n" +
		"Требования:\n" +
		"  • Установлен `codex` в PATH (github.com/openai/codex).\n" +
		"  • Выполнен `codex login` в свой ChatGPT-аккаунт.\n" +
		"\n" +
		"Ограничения:\n" +
		"  • execai-tools НЕ работают — codex запускает СВОИ tools.\n" +
		"  • Стриминга нет — codex exec возвращает финальный текст.",
	"subs.connect.ollamaHelp": "Ollama — 2 режима подключения:\n" +
		"\n" +
		"🌩  CLOUD (ollama.com):\n" +
		"   Модели glm-5.2, qwen3-coder-30b и др. крутятся у них на серверах.\n" +
		"   Anthropic-совместимый endpoint. Нужен API-ключ с https://ollama.com/settings/keys\n" +
		"   Использование:  /connect ollama <api-key>\n" +
		"\n" +
		"🏠  LOCAL (свой Ollama):\n" +
		"   Модели через `ollama pull <name>`, крутятся локально, 0 ₽.\n" +
		"   OpenAI-совместимый endpoint. Без ключа.\n" +
		"   Использование:  /connect ollama local\n" +
		"                   /connect ollama local http://192.168.1.10:11434  (свой URL)",
	"subs.connect.ollamaLocalOK": "✓ ollama (local) подключен: %s\nМодели (%d):  %s\n\nПереключись: /source ollama",
	"subs.connect.ollamaCloudOK": "✓ ollama (cloud) подключен: %s\n" +
		"Модели: glm-5.2, qwen3-coder-30b, kimi-k2 и др. (см. https://ollama.com/library)\n" +
		"\nПереключись: /source ollama\n" +
		"Смени модель:  /model",
	"subs.connect.unsupported": "провайдер %q пока не поддерживается. Доступно: zai, anthropic, openai, kimi (Kimi Code Coding Plan), kimi-api (Moonshot Platform pay-per-token), claude-cli, codex-cli, ollama",

	"subs.connect.example.zai":       "Пример: /connect zai sk-zai-XXXXX",
	"subs.connect.example.anthropic": "Пример: /connect anthropic sk-ant-XXXXX  (ключ из https://console.anthropic.com/settings/keys)",
	"subs.connect.example.kimi":      "Пример: /connect kimi sk-XXXXX  (Kimi Code Coding Plan из https://www.kimi.com/code/console)",
	"subs.connect.example.kimiapi":   "Пример: /connect kimi-api sk-XXXXX  (Moonshot Platform pay-per-token из https://platform.moonshot.ai/console/api-keys)",
	"subs.connect.example.openai":    "Пример: /connect openai sk-proj-XXXXX  (ключ из https://platform.openai.com/api-keys)",
	"subs.connect.keyRequired":       "API-key обязателен. %s",

	"subs.connect.kimiApiRejected": "✗ ключ отвергнут api.moonshot.ai (HTTP %d).\n" +
		"Возможно ты дал Kimi Code ключ вместо Moonshot Platform.\n\n" +
		"Ключи разные:\n" +
		"  • Kimi Code (подписка):    /connect kimi <key>  — из https://www.kimi.com/code/console\n" +
		"  • Moonshot pay-per-token:  /connect kimi-api <key> — из https://platform.moonshot.ai/console/api-keys",
	"subs.connect.openaiRejected": "✗ ключ отвергнут api.openai.com (HTTP %d).\n" +
		"Проверь: скопирован ли ключ целиком, актуален ли (не отозван).\n" +
		"Получить ключ: https://platform.openai.com/api-keys",
	"subs.connect.availableModels": "\n  Доступно моделей: %s",
	"subs.connected.via":           "✓ %s подключен через %s.%s\nПереключись:  /source %s",
	"subs.connected":               "✓ %s подключен. Переключись:  /source %s",

	// === Z.ai open platform (zai-api) + ToS-предупреждение по Coding Plan ===
	"subs.provider.zaiapi":        "Z.ai open platform (pay-per-token, api.z.ai)",
	"subs.connectHint.zaiapi":     "Z.ai pay-per-token (ключ z.ai/manage-apikey) — без ограничений по инструментам",
	"subs.connect.example.zaiapi": "Пример: /connect zai-api XXXXX  (ключ Z.ai open platform из https://z.ai/manage-apikey/apikey-list)",
	"subs.connect.zaiApiRejected": "✗ ключ отвергнут api.z.ai (HTTP %d).\n" +
		"Возможно ты дал ключ от GLM Coding Plan вместо open platform.\n" +
		"\n" +
		"Ключи разные:\n" +
		"  • GLM Coding Plan (подписка):   /connect zai <key>      — из https://z.ai/manage-apikey/subscription\n" +
		"  • Open platform (за токены):    /connect zai-api <key>  — из https://z.ai/manage-apikey/apikey-list",
	"subs.connect.zaiToSWarning": "⚠ Про условия GLM Coding Plan.\n" +
		"Z.ai ограничивает подписку закрытым списком инструментов (Claude Code, Cursor,\n" +
		"Cline, Roo Code, OpenCode, Pi, Crush, Goose и ещё несколько) — execai в него не входит.\n" +
		"Их правила допускают троттлинг, заморозку аккаунта и бан после трёх нарушений.\n" +
		"Рискует ТВОЙ аккаунт, не наш — решай сам.\n" +
		"\n" +
		"Альтернатива без ограничений — Z.ai open platform, оплата за токены:\n" +
		"  /connect zai-api <ключ>   — те же модели GLM, никакого списка инструментов.",
}

func init() { i18n.Register("ru", ruSubsMessages) }
