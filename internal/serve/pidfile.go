// Управление фоновым процессом: pid-файл, статус, остановка.
//
// Зачем pid-файл. Инбокс должен иметь ровно одного владельца на машину. Два
// демона не сломают данные — ack закрепляет задачу за одним, — но поделят
// поток задач случайным образом, и человек не поймёт, почему вывод появился
// не там, где он смотрит. Дешевле не пустить второй, чем объяснять.
//
// Живость проверяем сигналом 0, а не самим фактом файла: демона могли убить
// -9, перезагрузить машину, и оставшийся файл не должен блокировать запуск
// навсегда.
package serve

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// pidState — что записано в pid-файле.
type pidState struct {
	PID     int
	Started time.Time
	APIBase string
}

func pidPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "execai")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "serve.pid"), nil
}

// readPidFile читает состояние. Отсутствие файла — не ошибка, а «не запущен».
func readPidFile() (*pidState, error) {
	p, err := pidPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// Формат: <pid> <unix-время старта> <api base>
	f := strings.Fields(strings.TrimSpace(string(data)))
	if len(f) < 1 {
		return nil, nil // мусор в файле трактуем как «не запущен»
	}
	pid, err := strconv.Atoi(f[0])
	if err != nil {
		return nil, nil
	}
	st := &pidState{PID: pid}
	if len(f) > 1 {
		if sec, err := strconv.ParseInt(f[1], 10, 64); err == nil {
			st.Started = time.Unix(sec, 0)
		}
	}
	if len(f) > 2 {
		st.APIBase = f[2]
	}
	return st, nil
}

func writePidFile(apiBase string) error {
	p, err := pidPath()
	if err != nil {
		return err
	}
	line := fmt.Sprintf("%d %d %s\n", os.Getpid(), time.Now().Unix(), apiBase)
	return os.WriteFile(p, []byte(line), 0o600)
}

func removePidFile() {
	if p, err := pidPath(); err == nil {
		_ = os.Remove(p)
	}
}

// processAlive — жив ли процесс с таким pid.
//
// На Unix os.FindProcess успешен всегда, поэтому проверяем сигналом 0.
// На Windows FindProcess сам возвращает ошибку для мёртвого процесса.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return p.Signal(syscall.Signal(0)) == nil
}

// acquireLock занимает pid-файл. Возвращает ошибку, если демон уже работает.
// Освобождать — вызовом release.
func acquireLock(apiBase string) (release func(), err error) {
	st, err := readPidFile()
	if err != nil {
		return nil, err
	}
	if st != nil && processAlive(st.PID) {
		return nil, fmt.Errorf("демон уже запущен (pid %d, с %s). "+
			"Остановить: execai serve --stop · Проверить: execai serve --status",
			st.PID, st.Started.Format("15:04:05"))
	}
	// Файл остался от убитого процесса — перезаписываем молча, это норма
	// после kill -9 или перезагрузки.
	if err := writePidFile(apiBase); err != nil {
		return nil, err
	}
	return removePidFile, nil
}

// isDetached — отвязан ли процесс от терминала. Если stdout не TTY (перенаправлен
// в файл) или мы лидер своей группы после setsid — терминал уже не помеха, и
// подсказку показывать незачем.
func isDetached() bool {
	st, err := os.Stdout.Stat()
	if err != nil {
		return true // не смогли определить — молчим, чтобы не шуметь зря
	}
	return st.Mode()&os.ModeCharDevice == 0
}

// Status печатает состояние демона. Возвращает false, если он не запущен, —
// вызывающий превращает это в ненулевой код возврата для скриптов.
func Status() (bool, error) {
	st, err := readPidFile()
	if err != nil {
		return false, err
	}
	if st == nil || !processAlive(st.PID) {
		if st != nil {
			fmt.Printf("не запущен (в pid-файле %d — процесса нет)\n", st.PID)
		} else {
			fmt.Println("не запущен")
		}
		return false, nil
	}
	fmt.Printf("запущен · pid %d · с %s", st.PID, st.Started.Format("2006-01-02 15:04:05"))
	if !st.Started.IsZero() {
		fmt.Printf(" (%s назад)", time.Since(st.Started).Truncate(time.Second))
	}
	fmt.Println()
	if st.APIBase != "" {
		fmt.Printf("контур: %s\n", st.APIBase)
	}
	if p := AuditPath(); p != "" {
		fmt.Printf("журнал: %s\n", p)
	}
	return true, nil
}

// forceGrace — сколько даём на мягкий выход, прежде чем добить при --force.
// Не ноль: даже при форсе стоит дать шанс отправить результат, это дешёвые
// секунды против висящего в чате таймаута.
const forceGrace = 5 * time.Second

// Stop останавливает демона.
//
// Без force: SIGTERM и ожидание, жёстко не убиваем. Задача в работе должна
// успеть вернуть результат — иначе в чате останется висеть таймаут до
// истечения RunTimeout, и человек решит, что агент сломался.
//
// С force: тот же SIGTERM, короткая пауза, затем SIGKILL. Применять, когда
// демон завис и ждать нечего; о цене предупреждаем вслух.
func Stop(wait time.Duration, force bool) error {
	st, err := readPidFile()
	if err != nil {
		return err
	}
	if st == nil || !processAlive(st.PID) {
		removePidFile()
		fmt.Println("демон не запущен")
		return nil
	}
	p, err := os.FindProcess(st.PID)
	if err != nil {
		return err
	}
	// Даже при force сначала мягко: процесс может успеть закрыть задачу сам.
	if err := p.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("не удалось послать сигнал процессу %d: %w", st.PID, err)
	}

	grace := wait
	if force {
		grace = forceGrace
		fmt.Printf("останавливаю pid %d (жёстко через %s, если не выйдет сам)…\n", st.PID, grace)
	} else {
		fmt.Printf("останавливаю pid %d…\n", st.PID)
	}

	if waitGone(st.PID, grace) {
		removePidFile()
		fmt.Println("остановлен")
		return nil
	}

	if !force {
		// Не убиваем сами: возможно, идёт длинная задача. Решает человек.
		return fmt.Errorf("процесс %d не завершился за %s — он может доделывать задачу. "+
			"Добить: execai serve --stop --force", st.PID, wait)
	}

	// Форс. Результат текущей задачи потерян — говорим об этом прямо, иначе
	// человек будет искать причину в чате, а не здесь.
	fmt.Println("⚠ не вышел сам — убиваю. Результат текущей задачи не вернётся,")
	fmt.Println("  в чате она отвалится по таймауту.")
	if err := p.Kill(); err != nil {
		return fmt.Errorf("не удалось убить процесс %d: %w", st.PID, err)
	}
	if !waitGone(st.PID, 5*time.Second) {
		return fmt.Errorf("процесс %d не умер даже после SIGKILL — смотри вручную", st.PID)
	}
	// Убитый процесс не уберёт файл за собой — это наша работа.
	removePidFile()
	fmt.Println("убит")
	return nil
}

// waitGone ждёт исчезновения процесса, возвращает false по истечении срока.
func waitGone(pid int, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return !processAlive(pid)
}
