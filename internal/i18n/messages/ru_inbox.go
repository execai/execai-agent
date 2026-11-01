// Package messages/ru_inbox — русские строки канала «веб → агент»: задачи с
// сайта, которые принимает execai serve или панель редактора.
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var ruInboxMessages = map[string]string{
	"inbox.err.parse": "не разобрать ответ сервера",

	"inbox.lock.busy":         "замок инбокса всё время перехватывают — попробуйте ещё раз",
	"inbox.lock.heldByIDE":    "Задачи с сайта уже принимает окно редактора (pid %d, папка %s). Закройте в нём панель ExecAI или запускайте задачи оттуда.",
	"inbox.lock.heldByServe":  "Задачи с сайта уже принимает фоновый процесс execai serve (pid %d).",
	"inbox.lock.signalFailed": "не удалось послать сигнал процессу %d: %v",
	"inbox.lock.stopTimeout":  "execai serve (pid %d) не завершился за %s — возможно, доделывает задачу. Повторите или выполните: execai serve --stop --force",

	"inbox.plan.refused":   "Запуск с сайта доступен на тарифах «Работа» и «Макс»: %s",
	"inbox.plan.serveWarn": "⚠ Ваш тариф не включает запуск задач с сайта — на каждую задачу придёт отказ:",

	"inbox.result.interrupted": "(прервано: %s)",
	"inbox.result.noText":      "(агент отработал, но текстового ответа не дал)",

	"inbox.task.bindingsFailed": "не получить привязки проектов: %v",
	"inbox.task.dirMissing":     "каталог проекта не найден: %s",
	"inbox.task.noDir":          "для этого проекта не указан каталог — выполни здесь: execai → /project bind",
	"inbox.task.notBound":       "эта машина не привязана к проекту задачи — выполни: execai → /project bind",
	"inbox.task.empty":          "пустая задача",
	"inbox.task.stopped":        "остановлено в редакторе",
	"inbox.task.timeout":        "задача не уложилась в %s",
	"inbox.task.editorClosed":   "редактор закрыли раньше, чем задача завершилась",

	"inbox.serve.refused":     "✗ задача %s отклонена: тариф не включает запуск с сайта",
	"inbox.serve.statusIDE":   "не запущен: задачи с сайта принимает окно редактора (pid %d, папка %s)",
	"inbox.serve.stopIDE":     "задачи с сайта принимает окно редактора (pid %d, папка %s), а не execai serve — закройте это окно",
	"inbox.ide.otherProject":  "Задача для другого проекта (%s) ждёт — откройте его в редакторе или запустите execai serve",
	"inbox.ide.anotherWindow": "Задачи с сайта принимает другое окно: %s",
	"inbox.ide.taken":         "Теперь задачи с сайта принимает это окно",
	"inbox.ide.bindWhich":     "какой проект? Передайте его id (список — list_workspaces)",
}

func init() { i18n.Register("ru", ruInboxMessages) }
