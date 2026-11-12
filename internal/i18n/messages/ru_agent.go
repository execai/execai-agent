// Package messages/ru_agent — Russian strings of the agent loop and
// the model catalog that the person sees: a declined tool, the scope of a
// permission, working without the catalog. Merged into the "ru" catalog
// via i18n.Register.
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var ruAgentMessages = map[string]string{
	"agent.tool.declined":          "Пользователь отклонил выполнение этого инструмента.",
	"agent.tool.noApprovalChannel": "в execai нет канала подтверждения для этого вызова",
	"agent.approve.scope":          "разрешение распространится на: %s",
	"llm.offline.fallback":         "⚠ Работаю оффлайн — каталог моделей недоступен, использую встроенный список. Причина: %v",
	"llm.offline.cache":            "ℹ Использую кешированный каталог моделей (возраст: %s). Сеть недоступна: %v",
	"llm.models.unauthorized":      "401 от /billing-vbai/models_public — вход устарел, войдите заново (в чате: /login)",
	"llm.models.status":            "каталог моделей ответил %d: %s",
	"llm.models.notJSON":           "ответ каталога моделей — не список: %s",
	"llm.models.none":              "нет ни сети, ни сохранённого каталога моделей",
	"llm.err.unauthorized":         "401 от %s — %s",
	"llm.err.status":               "ответ %d: %s",
	"llm.err.server":               "сервер вернул ошибку: %v",
	"llm.err.aichatUnauthorized":   "401 от /aichat-vbai/stream — токен истёк, нужно execai login",
	"llm.err.sse":                  "ошибка от сервера в SSE: %v",
	"llm.err.anthropicKey":         "401 от Anthropic API — неверный ключ. Проверь /subscriptions",
	"llm.err.providerKey":          "401 от %s — неверный API-ключ. Проверь /subscriptions и /connect",
	"llm.err.providerStatus":       "%s ответ %d: %s",
	"llm.billing.limit":            "⏱  ЛИМИТ ИСЧЕРПАН",
	"llm.billing.funds":            "💰 НЕДОСТАТОЧНО СРЕДСТВ",
	"llm.billing.inactive":         "⚠  ТАРИФ НЕАКТИВЕН",
	"llm.billing.unavailable":      "Сервис временно недоступен по биллингу.",
	"llm.billing.hint":             "Сменить тариф: раздел «Тариф» в веб-кабинете ExecAI (%s)",
	"llm.cli.claudeMissing":        "claude CLI не найден (PATH, ~/.local/bin, расширения VS Code/Cursor). Установка: https://docs.claude.com/en/docs/claude-code/quickstart",
	"llm.cli.claudeNotFound":       "claude CLI не найден",
	"llm.cli.claudeNoModels":       "claude не вернул ни одной модели",
	"llm.cli.codexMissing":         "codex CLI не найден в PATH. Установка: https://github.com/openai/codex\nПосле установки: codex login",
	"llm.cli.codexNotFound":        "codex CLI не найден в PATH",
	"llm.cli.codexNoModels":        "codex не вернул ни одной модели",
	"llm.cli.noAnswer":             "CLI завершился, не ответив",
	"llm.cli.emptyResult":          "пустой результат",
	"llm.cli.emptyAnswer":          "пустой ответ",
	"llm.cli.images":               "картинки",
	"llm.cli.noToolResult":         "нет результата: claude завершился раньше",
	"llm.ollama.down":              "Ollama %s недоступен",
	"llm.ollama.start":             "Запусти: ollama serve",
	"llm.ollama.noModel":           "Ollama: модель %q не установлена (ollama pull %s?). %s",
	"llm.ollama.status":            "Ollama ответ %d: %s",
	"llm.ollama.tagsStatus":        "Ollama /api/tags вернул %d: %s",
	"llm.ollama.tagsParse":         "Ollama /api/tags парсинг",
	"llm.ollama.noModels":          "в Ollama нет моделей. Установи что-нибудь: `ollama pull llama3.2`",
	"llm.ollama.localModel":        "локальная модель Ollama",
	"agent.memory.readFailed":      "чтение памяти",
	"agent.memory.exportDirFailed": "создание каталога выгрузки",
	"agent.memory.writeFailed":     "запись %s",
	"agent.permissions.saveFailed": "не удалось сохранить разрешение:",
	"auth.notSignedIn":             "вы не вошли. Запустите: execai login",
}

func init() { i18n.Register("ru", ruAgentMessages) }
