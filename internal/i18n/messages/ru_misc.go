// Package messages/ru_misc — русские строки misc-батча (tui.go, plain REPL,
// compact, agent loop, welcome). Значения = ИСХОДНЫЕ строки из кода до i18n —
// поведение ru-локали не меняется (PTY smoke-тест грепает эти строки).
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var ruMiscMessages = map[string]string{
	// === Boot / login flow (tui.go) ===
	"ui.boot.noLogin": "ℹ Работаешь без аккаунта ExecAI — источник: %s · модель: %s.\n" +
		"  /login — подключить аккаунт ExecAI (наш каталог ~34 моделей + биллинг).",
	"ui.login.intro": "Привет! Чтобы войти — нужно подтвердить агента в браузере (как gh auth login).\n" +
		"Если по какой-то причине device-flow не работает — можешь вставить JWT-токен (eyJ…) сюда и нажать Enter.\n\n" +
		"ℹ Аккаунт ExecAI ОПЦИОНАЛЕН: можно работать со своей подпиской без входа.\n" +
		"  /connect kimi <key>   → /source kimi     (Kimi Code)\n" +
		"  /connect zai <key>    → /source zai      (Z.ai GLM)\n" +
		"  /connect openai <key> → /source openai   (OpenAI API)\n" +
		"  Также: anthropic, kimi-api, claude-cli, codex-cli, ollama — /connect покажет всё.",
	"ui.login.staleToken":          "Старый токен невалиден на %s. Запускаю device-flow для нового входа…",
	"ui.login.startFlow":           "Стартую device-flow для нового входа…",
	"ui.login.deviceFlowOpen":      "Открой в браузере и подтверди:\n\n  %s\n\nКод (на случай если ввод вручную): %s\n\nНажми Enter / [y] чтобы открыть браузер автоматически. [n] чтобы открыть руками.",
	"ui.boot.modelsFallbackFailed": "не смогли ни получить, ни собрать fallback-список моделей",
	"ui.welcome":                   "execai %s · %s/%s · %s · %s\nВведите задачу. /model — модели, /help — команды, /quit — выход.",

	// === Stream errors / token expiry ===
	"ui.stream.tokenExpiredHint": "→ Смени /source zai|ollama|anthropic, или /login чтобы переподтвердить.",
	"ui.stream.tokenExpiredFlow": "Токен ExecAI истёк — стартую device-flow. Подтверди в браузере.",

	// === /compact ===
	"ui.compact.historyNote": "[Сжатая история ранее (%d сообщений): %s]",
	"ui.compact.done":        "📦 История сжата: %d сообщений → 1 summary (~%d симв.)",
	"ui.compact.working":     "сжимаю историю…",
	"ui.compact.tooShort":    "история ещё короткая — нечего сжимать (нужно >%d сообщений)",
	"ui.compact.truncated":   "…(обрезано)",
	"ui.compact.promptSystem": "Ты сжиматель контекста для AI-агента. Тебе дают transcript беседы. " +
		"Верни КРАТКОЕ summary (≤500 слов) которое сохранит:\n" +
		"  • ключевые решения и причины\n" +
		"  • важные пути к файлам и команды\n" +
		"  • результаты tool-calls которые могут пригодиться дальше\n" +
		"  • ошибки и как они решены\n" +
		"Опусти болтовню и подтверждения. Пиши на русском, телеграфным стилем.",
	"ui.compact.promptUser": "Сожми эту беседу:\n\n%s",

	// === Autoloop ===
	"ui.autoloop.defaultPrompt": "продолжай",
	"ui.autoloop.wake":          "🌙 autoloop: пробуждение через %s (%s) → промт: %q",

	// === /paste ===
	"ui.paste.empty":     "Вставок в этой сессии нет. Ctrl+V большой кусок текста → маркер.",
	"ui.paste.header":    "Вставки (Ctrl+V ≥200 chars или c \\n):\n",
	"ui.paste.showHint":  "\nПоказать: /paste show <N>",
	"ui.paste.notNumber": "не число: %s",
	"ui.paste.notFound":  "вставки #%d нет",
	"ui.paste.usage":     "usage: /paste [list|show <N>]",

	// === /whoami ===
	"ui.whoami.notLoggedIn": "(не залогинен — /login)",

	// === /classic & /mouse ===
	"ui.classic.on":  "✓ classic TUI ON — рестартни execai (/quit → execai). Alt-screen + прибитый статус-бар, Shift+drag для копирования.",
	"ui.classic.off": "✓ Ink-style (default) — рестартни execai. История в scrollback, native selection и scroll.",
	"ui.mouse.off":   "🖱  захват мыши OFF — мышь выделяет текст, меню кликами не реагирует. Включить: /mouse on",
	"ui.mouse.on":    "🖱  захват мыши ON — колесо скроллит, клик по меню. Выделить текст: Shift+drag. Выкл: /mouse off",

	// === /effort ===
	"ui.effort.pickerHint": "effort picker: ←/→ выбрать, Enter подтвердить, Esc отмена",
	"ui.effort.current":    "Effort сейчас: %s (%d токенов)\nИзменить: /effort <off|low|medium|high|max>\n  off=0  low=1024  medium=4096  high=8192  max=32000\nРаботает для Anthropic-compat источников (Z.ai, Kimi, Anthropic, ollama-cloud, claude-cli).",
	"ui.effort.set":        "✓ effort=%s (%d токенов)",

	// === /max-iterations ===
	"ui.maxIter.current": "Max iterations сейчас: %d\nЛимит tool-use итераций за один ход. При исчерпании — мягкий stop, юзер может сказать 'продолжай'.\nИзменить: /max-iterations <N>  (рекомендуется 20-200; дефолт 50)",
	"ui.maxIter.usage":   "/max-iterations <N>  где N от 1 до 500 (рекомендуется 20-200)",

	// === /loop ===
	"ui.loop.status":      "🔁 loop: каждые %s — %q. /loop stop чтобы остановить",
	"ui.loop.inactive":    "loop неактивен. Использование: /loop <интервал> <prompt>  (пример: /loop 5m проверь статус билда)",
	"ui.loop.notRunning":  "loop и так не запущен",
	"ui.loop.stopped":     "🔁 loop остановлен",
	"ui.loop.usage":       "/loop <интервал> <prompt>  (например: /loop 5m проверь статус билда)",
	"ui.loop.badInterval": "не парсится интервал %s — нужно типа 30s, 5m, 1h",
	"ui.loop.started":     "🔁 loop запущен: каждые %s — %q\nОстановить: /loop stop",

	// === /log ===
	"ui.log.none":   "лога ещё нет: %s",
	"ui.log.header": "📜 Последние %d запросов (%s):\n",

	// === /usage & /cd ===
	"ui.usage.fetchFailed": "не удалось получить usage: %s",
	"ui.cd.current":        "cwd сейчас: %s",

	// === Sessions ===
	"ui.session.new":         "новая беседа",
	"ui.sessions.header":     "\nСохранённые беседы (свежие сверху):\n",
	"ui.sessions.empty":      "(пусто)\n",
	"ui.sessions.switchHint": "\nПереключиться: /resume <номер|id>",
	"ui.resume.notFound":     "беседа %q не найдена. /sessions — список",
	"ui.resume.loadFailed":   "не удалось загрузить: %s",
	"ui.resume.resumed":      "продолжаем: %s",
	"ui.title.renamed":       "переименовано: %s",

	// === /permissions ===
	"ui.perms.toolsEmpty": "  always allowed tools: (пусто)\n",
	"ui.perms.exactCount": "  always allowed exact commands: %d записей\n",
	"ui.perms.resetHint":  "\nЧтобы сбросить — удали файл вручную или запусти: rm ~/.config/execai/permissions.json",

	// === /model ===
	"ui.model.notFound": "модель %q не найдена",
	"ui.model.switched": "переключено на %s/%s — %s (история сохранена)",

	// === Approve ===
	"ui.approve.denied":       "отклонено",
	"ui.approve.allowedTool":  "разрешено: все вызовы %s в этой сессии",
	"ui.approve.allowedExact": "разрешено: эта команда в этой сессии",
	"ui.approve.navHint":      "← → или Tab — переключение, Enter — выбор, Esc — отклонить",

	// === Tool-call summaries ===
	"ui.toolSummary.write": "Write %s  (%d байт)",

	// === Plain REPL (chat.go) ===
	"plain.err.fetchModels":    "не удалось получить список моделей",
	"plain.err.emptyModels":    "сервер вернул пустой список моделей",
	"plain.err.pickModelEmpty": "не удалось выбрать модель (список пустой?)",
	"plain.err.pickModel":      "не удалось выбрать модель",
	"plain.commands":           "Команды: /model — выбрать модель, /clear — очистить историю, /quit — выход.",
	"plain.historyCleared":     "(история очищена)",
	"plain.modelSwitchHint":    "\nПереключение: /model <номер> или /model <model_name>",
	"plain.modelNotFound":      "(модель %q не найдена. /model — посмотреть список)",
	"plain.modelSwitched":      "(переключено на %s/%s — %s; история сохранена)",
	"plain.errorPrefix":        "ошибка:",
	"plain.modelsHeader":       "\nДоступные модели (★ — primary, • — current):",

	// === Agent loop (internal/agent) ===
	"loop.iterationLimit": "⚠ Достигнут лимит %d итераций — задача не закрыта. Скажи «продолжай» чтобы дать ещё %d, или переформулируй.",

	// === Welcome screen (first launch) ===
	"welcome.text": `Привет! Это execai — CLI-агент для разработки.

Что умеет:
  • читать/писать/править файлы (Read, Write, Edit)
  • искать (Grep, Glob, LS, Tree)
  • выполнять shell-команды (Bash) — read-only без вопроса, остальное со спроcом
  • ходить в HTTP (WebFetch — без браузера; реальный браузер появится отдельно)
  • вести to-do список (TodoWrite)

Память:
  • ./EXECAI.md           — память проекта (контекст репо)
  • ~/.config/execai/EXECAI.md — твои персональные настройки
Оба файла подгружаются в system prompt автоматически каждую сессию.

Команды:
  /model               — список моделей
  /model <num|подстрока> — переключиться (история сохраняется)
  /clear               — очистить историю
  /help                — это сообщение
  /quit                — выход

Подсказка: Enter — отправить, Shift+Enter — перенос строки.`,

	// === WebSearch / WebFetch tools ===
	"tool.websearch.noLogin": "Поиск в интернете недоступен: он идёт через шлюз ExecAI и требует аккаунта ExecAI.\n" +
		"Без логина остаётся локальный браузер — открывай страницы через WebFetch и переходи по ссылкам, которые он возвращает.\n" +
		"Команда /login включит поиск (а заодно и каталог моделей ExecAI).",
	"tool.websearch.sources": "Источники:",
	"tool.websearch.empty":   "Поиск ничего не вернул. Переформулируй запрос или открой конкретную страницу через WebFetch.",

	// === AskUser picker + субагенты ===
	"ask.title":             "Агент спрашивает:",
	"ask.hint":              "↑↓ выбрать · Enter подтвердить · 1-4 сразу · Esc — решай сам",
	"ask.answered":          "Вопрос: %s → %s",
	"ask.dismissed":         "оставлено на усмотрение агента",
	"ask.dismissedForModel": "Пользователь закрыл вопрос, не выбрав вариант. Решай сам, скажи, какое допущение принял, и продолжай.",
	"subagent.emptyResult":  "субагент вернул пустой результат",

	// === /key — ключ шифрования памяти ===
	"hint.key":  "ключ шифрования памяти (показать / экспорт / импорт / создать)",
	"key.usage": "/key — статус · /key new — создать · /key export — показать приватный ключ · /key import <ключ> — поставить ключ с другой машины",
	"key.absent": "Ключа шифрования пока нет.\n" +
		"Он создастся автоматически при первом синке — или сейчас, командой /key new.\n" +
		"Место: %s",
	"key.present": "Ключ шифрования на месте.\n" +
		"Публичный ключ (можно показывать; им шифруют ДЛЯ вас):\n" +
		"  %s\n" +
		"Файл: %s",
	"key.created": "Ключ шифрования создан.\n" +
		"Публичный ключ: %s\n" +
		"Файл: %s",
	"key.alreadyExists": "Ключ уже есть (публичный: %s). Он НЕ заменён — это означало бы потерю доступа ко всему, что им зашифровано.",
	"key.noRecoveryWarning": "⚠ Восстановления не существует. У нас вашего ключа нет и не будет.\n" +
		"Потеряете — синканутая память станет нечитаемой, её можно будет только собрать заново.\n" +
		"\n" +
		"Сохраните копию сейчас: /key export, и положите её в менеджер паролей.\n" +
		"Чтобы работать с той же памятью на другой машине, выполните там /key import <ключ>.",
	"key.exportWarning": "⚠ Ниже ПРИВАТНЫЙ ключ. Кто им владеет — читает вашу синканутую память. Не вставляйте его в чаты, issue и скриншоты.",
	"key.exportHint":    "Публичная часть (её показывать безопасно): %s",
	"key.imported":      "Ключ установлен. Публичный: %s",
	"key.importFailed": "Не удалось установить ключ: %v\n" +
		"\n" +
		"Если вы хотели заменить существующий — сначала удалите его, но убедитесь, что\n" +
		"копия сохранена: иначе всё, зашифрованное старым ключом, станет нечитаемым: %s",
	"key.invalid": "Это не похоже на приватный ключ (%v). Ожидается формат AGE-SECRET-KEY-1…",
	"key.error":   "Операция с ключом не удалась: %v",

	// === /memory — импорт и экспорт памяти ===
	"hint.memory":            "память агента: импорт от других агентов, экспорт",
	"memory.usage":           "/memory — статус · /memory import — забрать память других агентов отсюда · /memory export [каталог] — выгрузить память в markdown",
	"memory.status":          "Память: %d записей в %s",
	"memory.foundNearby":     "В этом каталоге найдено файлов памяти других агентов: %d",
	"memory.importHint":      "\nЗабрать их: /memory import",
	"memory.nothingToImport": "В %s памяти других агентов не найдено.\nИскал: CLAUDE.md, .claude/, AGENTS.md, .cursorrules, .cursor/rules/, .github/copilot-instructions.md, EXECAI.md",
	"memory.importQuestion":  "Импортировать %d файл(ов) в память агента?",
	"memory.importYes":       "Импортировать",
	"memory.importYesDesc":   "Содержимое станет частью памяти — а память потом синкается на сервер (зашифрованной твоим ключом)",
	"memory.importNo":        "Отмена",
	"memory.importNoDesc":    "Ничего не читаем и не копируем",
	"memory.importCancelled": "Импорт отменён — ничего не скопировано.",
	"memory.imported":        "Импортировано: %d",
	"memory.exported":        "Выгружено файлов: %d → %s",
	"memory.error":           "Операция с памятью не удалась: %v",

	// === /project — привязка каталога к проекту ===
	"hint.project":         "привязать этот каталог к проекту из веб-чата",
	"project.usage":        "/project — список проектов · bind <имя> — привязать этот каталог · unbind — отвязать · on/off — включить/выключить агента в проекте",
	"project.listHeader":   "Твои проекты (● привязан к этому каталогу, ○ привязан в другом месте):",
	"project.listHint":     "Привязать: /project bind <имя>",
	"project.none":         "Проектов пока нет — создай в веб-чате.",
	"project.defaultTag":   "[по умолчанию]",
	"project.bound":        "Проект «%s» привязан к %s",
	"project.unbound":      "Привязка снята с %s",
	"project.notBound":     "К каталогу %s не привязан ни один проект",
	"project.notFound":     "Проект «%s» не найден. Доступны: %s",
	"project.needLogin":    "нужен аккаунт ExecAI — /login",
	"project.error":        "Операция с проектом не удалась: %v",
	"project.agentOn":      "[агент вкл]",
	"project.agentOff":     "[агент ВЫКЛ]",
	"project.enabled":      "Агент «%s» включён в проекте «%s» — задачи оттуда он берёт",
	"project.disabled":     "Агент «%s» выключен в проекте «%s» — задачи оттуда он брать не будет",
	"project.notAgent":     "Эта сессия не агент: добавить в проект нечего. Нужен вход через /login на самой машине.",
	"project.notInProject": "Агента нет в составе проекта «%s» — сначала /project bind",
	"project.boundNoTool":  "Каталог %[2]s привязан к проекту «%[1]s», но добавить агента в состав проекта не вышло: %[3]v",
}

func init() {
	i18n.Register("ru", ruMiscMessages)
}
