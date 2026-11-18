// Package messages/ru_ide — Russian strings the agent sends to the editor
// panel (execai ide): the permission buttons and the list of sources to
// connect.
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var ruIDEMessages = map[string]string{
	// The descriptions under the permission buttons; the buttons themselves
	// are approve.* (shared with the terminal chat).
	"ide.approve.onceDesc":    "разрешить только этот вызов",
	"ide.approve.sessionDesc": "до конца этого чата",
	"ide.approve.exactDesc":   "точно такой вызов до конца чата",
	"ide.approve.foreverDesc": "запомнить это разрешение — запишется в permissions.json",
	"ide.approve.denyDesc":    "не выполнять; агент получит отказ",

	"ide.connect.withKey":    "%s (ключ)",
	"ide.connect.noKey":      "%s (без ключа)",
	"ide.connect.zaiAPI":     "Z.ai платформа, pay-per-token (ключ)",
	"ide.connect.openrouter": "OpenRouter — все вендоры одним ключом (ключ)",
	"ide.connect.ollama":     "Ollama — локальная или облако (ключ для облака)",
	"ide.connect.custom":     "Свой провайдер — OpenAI- или Anthropic-совместимый адрес",

	// Notices and errors the panel shows (execai ide), the security levels
	// and the errors of the sign-in and connect paths from other packages.
	"ide.err.effort":            "effort: off|low|medium|high|max или число",
	"ide.err.positive":          "нужно положительное число",
	"ide.maxIter.set":           "предел итераций на ход: %d",
	"ide.login.startFailed":     "не удалось начать вход",
	"ide.login.saveFailed":      "не удалось сохранить токен",
	"ide.login.expired":         "код подтверждения истёк — попробуй ещё раз",
	"ide.login.timeout":         "вход не подтверждён вовремя",
	"ide.connect.keyRejectedBy": "%s отверг ключ (HTTP %d)",
	"ide.connect.noModels":      "подключено: %s (%s) — сервер не отдал список моделей (HTTP %d); переподключи с перечнем моделей",
	"ide.connect.ollamaKey":     "ollama.com не принял ключ (HTTP %d)",
	"ide.connect.ollamaDown":    "Ollama на %s не ответила (HTTP %d) — запущена ли она?",
	"ide.connect.unknown":       "неизвестный источник: %s",
	"ide.connect.needKey":       "нужен ключ провайдера",
	"ide.connect.keyRejected":   "ключ отвергнут (HTTP %d) — проверь, что он от %s",
	"ide.connect.done":          "подключено: %s",
	"ide.connect.models":        " — моделей доступно: %d",
	"ide.connect.unverified":    " (ключ сохранён; проверится на первом запросе)",
	"ide.turn.queued":           "ход в очереди: другие чаты ещё отвечают",
	"ide.err.chdir":             "не удалось перейти в каталог %s",
	"ide.err.noModel":           "не удалось выбрать модель — проверь /model в execai",
	"ide.err.noModelShort":      "нет модели",
	"ide.err.badMessage":        "не удалось разобрать сообщение: %s",
	"ide.err.noChat":            "нет такого чата: %s",
	"ide.subs.unreadable":       "subscriptions.json не прочитан (%s) — работаю на базовом источнике; подписки не потеряны, но файл нужно починить",
	"ide.err.noSuchModel":       "нет такой модели у текущего источника: %s",
	"ide.err.unknownLevel":      "неизвестный уровень: %s",
	"ide.security.set":          "уровень безопасности: %s",
	"ide.logout.done":           "вышли из ExecAI (подписки остались)",
	"ide.disconnect.done":       "отключено: %s",
	"ide.resume.none":           "продолжать нечего — это первый чат в проекте",
	"ide.resume.failed":         "не удалось открыть последний чат: %s",
	"ide.chat.openFailed":       "не удалось открыть чат: %s",
	"ide.wake.none":             "пробуждение не было запланировано",
	"ide.err.unknownCommand":    "неизвестная команда: %s",
	"ide.err.unknownType":       "неизвестный тип: %s",
	"ide.chat.busy":             "этот чат ещё отвечает — дождись конца хода или останови его",
	"security.level.light":      "лёгкий — не мешать",
	"security.level.deep":       "обычный — проект молча, наружу с вопросом",
	"security.level.paranoid":   "строгий — спрашивать всё",
	"connect.err.api":           "тип интерфейса %q не поддерживается: openai (chat/completions) или anthropic (messages)",
	"connect.err.noURL":         "нужен адрес сервера, например http://localhost:1234/v1",
	"connect.err.badURL":        "адрес %q не похож на http(s)-URL",
	"subs.err.nameLatin":        "имя должно содержать латинские буквы или цифры (из него получается id для /source)",
	"subs.err.nameLong":         "имя длиннее 40 символов",
	"subs.err.builtin":          "%q — встроенный источник, выбери другое имя",
	"subs.err.notLoaded":        "подписки не загружены",
	"subs.err.notConnected":     "подписка %q не подключена — сначала /connect %s",
	"auth.err.linkStart":        "agent-link/start вернул %d: %s",
	"auth.err.linkPoll":         "agent-link/poll вернул %d: %s",
	"auth.err.pollFailed":       "polling упал 10 раз подряд",
	"auth.err.linkExpired":      "ссылка истекла — запусти /login заново",
}

func init() { i18n.Register("ru", ruIDEMessages) }
