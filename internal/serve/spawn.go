// Запуск фонового слушателя из TUI.
//
// После /project bind человек почти всегда хочет именно этого: привязал
// каталог — начал принимать задачи. Заставлять его помнить про отдельную
// команду и про setsid — лишний шаг, на котором всё и обрывается: задача из
// веба уходит в очередь, а в чате «агент не отвечает».
//
// Процесс отвязываем от текущей сессии (см. spawn_unix.go / spawn_windows.go),
// иначе он умрёт вместе с TUI — то есть ровно тогда, когда должен работать.
package serve

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// SpawnLogPath — куда уходит вывод фонового процесса.
func SpawnLogPath() string {
	base, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "execai", "serve.out.log")
}

// AlreadyRunning — работает ли демон прямо сейчас.
func AlreadyRunning() (bool, int) {
	st, err := readPidFile()
	if err != nil || st == nil {
		return false, 0
	}
	return processAlive(st.PID), st.PID
}

// Spawn запускает `execai serve` в фоне и ждёт подтверждения, что он
// действительно поднялся.
//
// Возвращает pid. Проверяем через pid-файл, а не по факту старта процесса:
// демон может умереть сразу (нет токена, занят замок), и «запустил» без
// проверки было бы обещанием, которое некому сдержать.
func Spawn(extraArgs ...string) (int, error) {
	if running, pid := AlreadyRunning(); running {
		return pid, fmt.Errorf("уже запущен (pid %d)", pid)
	}

	exe, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("не найти собственный бинарь: %w", err)
	}
	logPath := SpawnLogPath()
	if logPath == "" {
		return 0, fmt.Errorf("не определить каталог конфига")
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return 0, err
	}
	// Дописываем, а не перетираем: прошлые запуски — часть истории отладки.
	logf, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, err
	}
	defer logf.Close()

	args := append([]string{"serve"}, extraArgs...)
	cmd := exec.Command(exe, args...)
	cmd.Stdout, cmd.Stderr = logf, logf
	cmd.Stdin = nil
	// Рабочий каталог не наследуем от TUI: демон берёт каталог задачи из
	// привязки проекта, а свой cwd держал бы папку занятой при удалении.
	cmd.Dir = string(os.PathSeparator)
	detach(cmd)

	if err := cmd.Start(); err != nil {
		return 0, err
	}
	// Родитель не ждёт — иначе останется зомби в таблице процессов.
	go func() { _ = cmd.Wait() }()

	// Ждём появления pid-файла: только он доказывает, что демон прошёл
	// авторизацию и взял замок.
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if running, pid := AlreadyRunning(); running {
			return pid, nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return 0, fmt.Errorf("не поднялся за 8с — смотри %s", logPath)
}
