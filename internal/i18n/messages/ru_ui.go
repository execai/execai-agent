// Package messages/ru_ui — Russian strings for execai ui (the agent panel in
// a browser). Merged into the "ru" catalog via i18n.Register.
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var ruUIMessages = map[string]string{
	"ui.cmd.short": "Открыть панель агента в браузере (только на этом компьютере)",
	"ui.cmd.long": "Та же панель чата, что в VS Code и ExecAI Studio, во вкладке браузера на 127.0.0.1.\n\n" +
		"В адресе одноразовый ключ: при первом заходе он меняется на cookie, повторно ссылка ничего не открывает. " +
		"Подключиться можно только с этого компьютера. Задачи с сайта и проверка плана — как в панели редактора. Остановить: Ctrl+C.",
	"ui.flag.cwd":    "каталог проекта (по умолчанию — текущий)",
	"ui.flag.port":   "порт на 127.0.0.1 (по умолчанию — свободный)",
	"ui.flag.noOpen": "не открывать браузер, только напечатать адрес",
	"ui.badCwd":      "не каталог: %s",
	"ui.address":     "Панель ExecAI: %s",
	"ui.hint":        "Ссылка срабатывает один раз — браузер, который её открыл, сохраняет доступ, пока работает execai ui. Остановить: Ctrl+C.",
	"ui.openFailed":  "Не удалось открыть браузер (%v) — открой ссылку сам.",
	"ui.stopped":     "Панель остановлена.",
	"ui.http.linkUsed": "Эта ссылка уже использована. Открой панель в том браузере, где открывал её впервые, " +
		"или перезапусти execai ui — будет новая ссылка.",
	"ui.http.noSession": "Открой ссылку, которую execai ui напечатал в терминале.",
}

func init() { i18n.Register("ru", ruUIMessages) }
