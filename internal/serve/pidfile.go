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
	"syscall"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/inbox"
)

// The lock itself (format, atomic creation, liveness) lives in
// internal/inbox: the editor panel (`execai ide`) owns the same inbox through
// the same file. Here — the daemon's side: its messages, --status, --stop.

// pidState — что записано в pid-файле.
type pidState = inbox.Owner

func pidPath() (string, error) { return inbox.LockPath() }

// readPidFile читает состояние. Отсутствие файла — не ошибка, а «не запущен».
func readPidFile() (*pidState, error) { return inbox.ReadOwner() }

func writePidFile(apiBase string) error { return inbox.WriteOwner(inbox.KindServe, apiBase, "") }

func removePidFile() { inbox.RemoveLock() }

// processAlive — жив ли процесс с таким pid.
func processAlive(pid int) bool { return inbox.ProcessAlive(pid) }

// acquireLock takes the inbox for the daemon. It fails when a live process
// holds it — another daemon or an editor window. Free it with release.
func acquireLock(apiBase string) (release func(), err error) {
	release, err = inbox.Acquire(inbox.KindServe, apiBase, "")
	var locked *inbox.LockedError
	if errors.As(err, &locked) {
		if locked.Owner.Kind == inbox.KindIDE {
			// An editor window accepts the tasks itself; a second owner
			// would split the stream with it.
			return nil, errors.New(i18n.Tf("inbox.lock.heldByIDE", locked.Owner.PID, locked.Owner.Cwd))
		}
		return nil, fmt.Errorf("демон уже запущен (pid %d, с %s). "+
			"Остановить: execai serve --stop · Проверить: execai serve --status",
			locked.Owner.PID, locked.Owner.Started.Format("15:04:05"))
	}
	return release, err
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
	if st.Kind == inbox.KindIDE {
		// An editor window holds the inbox: no daemon, yet tasks are taken.
		fmt.Println(i18n.Tf("inbox.serve.statusIDE", st.PID, st.Cwd))
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
	if st.Kind == inbox.KindIDE {
		// Not a daemon but an editor window's agent: a signal would kill
		// its chats.
		return errors.New(i18n.Tf("inbox.serve.stopIDE", st.PID, st.Cwd))
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
func waitGone(pid int, d time.Duration) bool { return inbox.WaitGone(pid, d) }
