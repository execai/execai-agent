// Package messages/ru_custom — Russian strings for live model lists and the
// custom provider (/connect custom). Merged into the "ru" catalog.
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var ruCustomMessages = map[string]string{
	"catalog.cliDefault":               "По умолчанию",
	"catalog.cliDefaultDesc":           "без --model: модель из настроек самого CLI",
	"subs.connect.modelsCount":         "Моделей у провайдера: %d — выбрать: /model.",
	"subs.connect.noModels":            "Провайдер не отдал список моделей. Укажи модель сам: /model <id> — она запомнится для этого источника.",
	"subs.connect.ollamaLocalFailed":   "✗ Ollama на %s не ответила (HTTP %d). Она запущена? `ollama serve`",
	"subs.connect.ollamaCloudRejected": "✗ ollama.com не принял ключ (HTTP %d). Ключи: https://ollama.com/settings/keys",
	"ui.model.listEmpty":               "Источник пока не сообщил ни одной модели. Укажи сам: /model <id>",
	"ui.model.typed":                   "Модели %q нет в списке провайдера — беру как написано и запоминаю для этого источника.",
	"hint.connect.custom":              "свой провайдер: любой OpenAI- или Anthropic-совместимый адрес",
	"subs.connectHint.custom":          "свой провайдер: тип интерфейса, адрес, ключ — по шагам",
	"subs.hint.custom":                 "свой (%s) · %s",
	"subs.custom.usage": "Использование:\n" +
		"  /connect custom                                — по шагам\n" +
		"  /connect custom <имя> <openai|anthropic> <base_url> [api_key] [--models a,b] [--header \"Имя: значение\"]\n" +
		"Примеры:\n" +
		"  /connect custom \"LM Studio\" openai http://localhost:1234/v1\n" +
		"  /connect custom deepseek openai https://api.deepseek.com sk-xxx\n" +
		"  /connect custom corp anthropic https://llm.corp.local sk-xxx --header \"X-Tenant: dev\"",

	"custom.wiz.intro":              "Подключаем свой провайдер — любой сервер с API в стиле OpenAI или Anthropic. Esc — отмена.",
	"custom.wiz.name":               "Шаг 1/4. Как назвать? Например: LM Studio, vLLM, deepseek, corp-gateway (латиницей — из имени получится id для /source).",
	"custom.wiz.idIs":               "id: %s  (потом переключаться: /source <id>)",
	"custom.wiz.api":                "Шаг 2/4. Тип интерфейса:\n  1 — OpenAI-совместимый (POST …/chat/completions): LM Studio, vLLM, llama.cpp, LiteLLM, DeepSeek, Groq, Together, Mistral…\n  2 — Anthropic-совместимый (POST …/v1/messages): LiteLLM, шлюзы к Claude, MiniMax, DeepSeek…\nВведи 1 или 2.",
	"custom.wiz.urlOpenAI":          "Шаг 3/4. Адрес сервера (base URL), например http://localhost:1234/v1, http://gpu-box:8000/v1, https://api.deepseek.com",
	"custom.wiz.urlAnthropic":       "Шаг 3/4. Адрес сервера (base URL) без /v1/messages, например https://llm.corp.local, https://api.deepseek.com/anthropic",
	"custom.wiz.key":                "Шаг 4/4. API-ключ — Enter, если ключа нет (у локальных серверов его обычно нет). Хранится только на этой машине и в историю ввода не попадает.",
	"custom.wiz.noKey":              "(без ключа)",
	"custom.wiz.keyRejected":        "✗ Сервер отверг ключ (HTTP %d). Введи ещё раз или Esc — отмена.",
	"custom.wiz.models":             "Сервер не отдал список моделей (GET …/models: HTTP %d). Перечисли ID моделей через запятую или Enter — пропустить и потом указать через /model <id>.",
	"custom.wiz.cancelled":          "Подключение своего провайдера отменено.",
	"custom.wiz.placeholder.name":   "имя провайдера, например LM Studio",
	"custom.wiz.placeholder.api":    "1 — OpenAI-совместимый, 2 — Anthropic-совместимый",
	"custom.wiz.placeholder.url":    "base URL, например http://localhost:1234/v1",
	"custom.wiz.placeholder.key":    "API-ключ или Enter, если ключа нет",
	"custom.wiz.placeholder.models": "ID моделей через запятую или Enter",
	"custom.keyRejected":            "✗ %[2]s отверг ключ (HTTP %[1]d).",
	"custom.connected":              "✓ Подключено: %s (id %s, протокол %s, %s)",
	"custom.connectedModels":        "Моделей: %d, текущая: %s — выбрать: /model.",
	"custom.connectedHint":          "Вернуться потом: /source %[1]s · удалить: /disconnect %[1]s",
}

func init() { i18n.Register("ru", ruCustomMessages) }
