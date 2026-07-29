// Welcome screen — onboarding on first launch. The `.welcome_seen` marker
// lives in the config directory. It is created after the first display, and
// follow-up launches skip the greeting.
package welcome

import (
	"os"
	"path/filepath"
)

const welcomeText = `Привет! Это execai — CLI-агент для разработки.

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

Подсказка: Enter — отправить, Shift+Enter — перенос строки.`

// MaybeWelcome returns the greeting text on the FIRST launch, otherwise an
// empty string. The marker is written immediately upon display.
func MaybeWelcome() string {
	base, err := os.UserConfigDir()
	if err != nil {
		return welcomeText
	}
	dir := filepath.Join(base, "execai")
	_ = os.MkdirAll(dir, 0o700)
	marker := filepath.Join(dir, ".welcome_seen")
	if _, err := os.Stat(marker); err == nil {
		return ""
	}
	_ = os.WriteFile(marker, []byte("1"), 0o644)
	return welcomeText
}
