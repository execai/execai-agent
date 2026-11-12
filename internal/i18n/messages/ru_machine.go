// Package messages/ru_machine — русские строки обучалки (/start), карточки
// машины (/machine) и ссылки для телефона (/phone).
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var ruMachineMessages = map[string]string{
	"hint.start":        "начало работы: пять шагов и что уже сделано",
	"hint.machine":      "моя машина: модели, синк, откуда можно позвать",
	"hint.machineCheck": "проверить, что источники моделей отвечают",
	"hint.phone":        "QR-код: позвать эту машину с телефона",

	// === /start ===
	"start.title":    "Начало работы",
	"start.progress": "сделано %d из %d",
	"start.allDone":  "Всё готово: эта машина настроена полностью.",
	"start.footer": "Моя машина: /machine · все команды: /help · этот список снова: /start\n" +
		"Enter — отправить, Shift+Enter — новая строка.",

	"start.login.title": "Войти в аккаунт ExecAI",
	"start.login.why":   "Аккаунт связывает эту машину с веб-чатом, телефоном и другими вашими компьютерами.",
	"start.login.cmd":   "Введите: /login — и подтвердите вход в браузере.",

	"start.models.title": "Подключить модели",
	"start.models.why":   "Свои ключи провайдеров или тариф ExecAI — память и вызов с телефона работают с любым вариантом.",
	"start.models.cmd":   "Введите: /connect <провайдер> — свой ключ, /plan — тариф ExecAI, /machine check — проверить, что источник отвечает.",

	"start.sync.title":    "Включить синхронизацию",
	"start.sync.why":      "Одинаковые настройки и память на всех ваших компьютерах.",
	"start.sync.what":     "Уедет, зашифрованное на этом компьютере вашим паролем синхронизации: конфиг вместе с ключами провайдеров, личная память, файлы привязанных проектов.",
	"start.sync.notWhat":  "Останется здесь: api_base (адрес сервера этой машины) и вход в аккаунт.",
	"start.sync.password": "Пароль синхронизации знаете только вы: мы его не храним и восстановить не сможем.",
	"start.sync.cmd":      "Введите: /sync on (тарифы «Работа» и «Макс»).",

	"start.project.title": "Привязать папку к проекту в вебе",
	"start.project.why":   "Папка становится проектом в веб-чате: задачи из веба выполняются в ней, память проекта общая.",
	"start.project.cmd":   "Введите: /project publish — сделать проект из этой папки, /project bind <имя> — привязать к существующему.",

	"start.phone.title": "Позвать агента с телефона",
	"start.phone.why":   "Ставьте этой машине задачи из веб-чата или с телефона — даже если она работает на ваших собственных ключах.",
	"start.phone.cmd":   "Введите: /phone — QR-код для телефона. Чтобы машина принимала задачи, запустите execai serve.",

	// === /machine ===
	"machine.title":               "Моя машина",
	"machine.label.machine":       "Машина",
	"machine.label.account":       "Аккаунт",
	"machine.label.models":        "Модели",
	"machine.label.sync":          "Синхронизация",
	"machine.label.presence":      "Доступность",
	"machine.identity":            "%s · %s · агент %s",
	"machine.account.in":          "%s · тариф: %s",
	"machine.account.noPlan":      "%s · без тарифа ExecAI",
	"machine.account.planPending": "%s · тариф уточняется",
	"machine.account.out":         "вход не выполнен — /login",

	"machine.source.alive":     "отвечает (проверено %s)",
	"machine.source.down":      "не ответил (проверено %s)",
	"machine.source.unchecked": "ещё не проверялся — /machine check",
	"machine.source.needLogin": "нужен вход — /login",
	"machine.source.noPlan":    "нет активного тарифа — /plan",
	"machine.source.active":    "используется",

	"machine.sync.off":      "выключена — /sync on",
	"machine.sync.on":       "включена · последняя отправка: %s",
	"machine.sync.never":    "включена · отправок ещё не было",
	"machine.sync.paused":   "на паузе — подробности: /sync",
	"machine.sync.inside":   "внутри: %s",
	"machine.sync.config":   "конфиг и ключи провайдеров",
	"machine.sync.memory":   "личная память (файлов: %d)",
	"machine.sync.projects": "проектов: %d",
	"machine.sync.empty":    "пока ничего",
	"machine.sync.mode":     "режим: %s",
	"machine.mode.all":      "все проекты",
	"machine.mode.selected": "только привязанные папки",
	"machine.mode.none":     "без проектов",

	"machine.presence.serve":      "приём задач: да — execai serve (pid %d)",
	"machine.presence.ide":        "приём задач: да — окно редактора (pid %d)",
	"machine.presence.noListener": "приём задач: нет — запустите execai serve",
	"machine.presence.reachable":  "звать из веба и с телефона: можно — /phone",
	"machine.presence.blocked":    "звать из веба и с телефона: пока нельзя — %s",
	"machine.block.login":         "войдите (/login)",
	"machine.block.plan":          "нужен тариф «Работа» или «Макс»",
	"machine.block.planPending":   "тариф уточняется",
	"machine.block.listener":      "запустите execai serve",

	"machine.footer":   "Проверить источники заново: /machine check · обучалка: /start",
	"machine.checking": "Проверяю источники…",

	// === /phone ===
	"phone.title":     "Позвать эту машину с телефона",
	"phone.scan":      "Наведите камеру телефона на код или откройте ссылку:",
	"phone.what":      "Страница откроет веб-чат, где эта машина уже выбрана для задач.",
	"phone.needLogin": "Ссылка для телефона появится после входа: /login",
	"phone.listener":  "Чтобы машина принимала задачи, запустите execai serve (в отдельном окне).",
	"phone.plan":      "Задачи из веба и с телефона — на тарифах «Работа» и «Макс»: %s",
	"phone.qrFailed":  "Не удалось нарисовать QR-код: %v",
}

func init() { i18n.Register("ru", ruMachineMessages) }
