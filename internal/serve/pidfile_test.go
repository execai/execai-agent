package serve

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestAcquireLock_SecondFails(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	release, err := acquireLock("https://api.example")
	if err != nil {
		t.Fatalf("первый замок не взялся: %v", err)
	}
	defer release()

	// Второй демон на той же машине поделил бы поток задач с первым.
	_, err = acquireLock("https://api.example")
	if err == nil {
		t.Fatal("второй замок взялся — два демона поделят инбокс")
	}
	// Текст ошибки должен вести к решению, а не просто ругаться.
	for _, want := range []string{"уже запущен", "--stop", "--status"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("в ошибке нет %q: %v", want, err)
		}
	}
}

// Убили -9 или перезагрузили машину — оставшийся файл не должен блокировать
// запуск навсегда.
func TestAcquireLock_StalePidIgnored(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	// Заведомо мёртвый pid: короткоживущий процесс, которого уже нет.
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Skipf("нет /bin/true: %v", err)
	}
	dead := cmd.Process.Pid

	p := filepath.Join(dir, "execai", "serve.pid")
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(strings.TrimSpace(
		strconv.Itoa(dead)+" 1780000000 https://old")), 0o600); err != nil {
		t.Fatal(err)
	}

	release, err := acquireLock("https://api.example")
	if err != nil {
		t.Fatalf("протухший pid-файл заблокировал запуск: %v", err)
	}
	release()
}

// Мусор в файле не должен ронять демона — трактуем как «не запущен».
func TestReadPidFile_Garbage(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	p := filepath.Join(dir, "execai", "serve.pid")
	_ = os.MkdirAll(filepath.Dir(p), 0o700)

	for _, junk := range []string{"", "   ", "не-число", "abc def ghi"} {
		if err := os.WriteFile(p, []byte(junk), 0o600); err != nil {
			t.Fatal(err)
		}
		st, err := readPidFile()
		if err != nil {
			t.Errorf("мусор %q дал ошибку: %v", junk, err)
		}
		if st != nil {
			t.Errorf("мусор %q разобран как состояние: %+v", junk, st)
		}
	}
}

func TestPidFile_RoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := writePidFile("https://api.example.com"); err != nil {
		t.Fatal(err)
	}
	st, err := readPidFile()
	if err != nil || st == nil {
		t.Fatalf("не прочитано: %v", err)
	}
	if st.PID != os.Getpid() {
		t.Errorf("pid %d, ожидался %d", st.PID, os.Getpid())
	}
	if st.APIBase != "https://api.example.com" {
		t.Errorf("контур %q — без него --status не покажет, к чему подключён", st.APIBase)
	}
	if time.Since(st.Started) > time.Minute {
		t.Errorf("время старта не записалось: %v", st.Started)
	}
	// Права: файл лежит рядом с токенами.
	p, _ := pidPath()
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o600 {
		t.Errorf("права %v, ожидалось 0600", fi.Mode().Perm())
	}

	removePidFile()
	if st, _ := readPidFile(); st != nil {
		t.Error("файл не удалён при остановке — следующий запуск увидит чужой pid")
	}
}

func TestProcessAlive(t *testing.T) {
	if !processAlive(os.Getpid()) {
		t.Error("собственный процесс признан мёртвым")
	}
	if processAlive(0) || processAlive(-1) {
		t.Error("нулевой/отрицательный pid признан живым")
	}
}

// Журнал не должен расти бесконечно: демон живёт неделями.
func TestAuditLog_Rotates(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	a := newAuditLog()
	if a.f == nil {
		t.Fatal("журнал не открылся")
	}
	a.setTask("t1")
	a.size = maxAuditSize // имитируем заполненный файл
	a.record("call", "Bash", "после ротации", "")
	a.close()

	logPath := filepath.Join(dir, "execai", "serve-audit.log")
	if _, err := os.Stat(logPath + ".1"); err != nil {
		t.Errorf("прошлое поколение не сохранено: %v", err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("новый журнал не создан: %v", err)
	}
	if !strings.Contains(string(data), "после ротации") {
		t.Errorf("запись после ротации потеряна: %q", data)
	}
}

// Stop без force не должен убивать: задача в работе обязана успеть вернуть
// результат, иначе в чате останется висеть таймаут.
func TestStop_GracefulDoesNotKill(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	// Процесс, который игнорирует SIGTERM — имитация «завис на задаче».
	cmd := exec.Command("sh", "-c", "trap '' TERM; sleep 30")
	if err := cmd.Start(); err != nil {
		t.Skipf("не запустить sh: %v", err)
	}
	defer func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() }()

	p := filepath.Join(dir, "execai", "serve.pid")
	_ = os.MkdirAll(filepath.Dir(p), 0o700)
	_ = os.WriteFile(p, []byte(strconv.Itoa(cmd.Process.Pid)+" 1780000000 https://x"), 0o600)

	err := Stop(1*time.Second, false)
	if err == nil {
		t.Fatal("мягкая остановка отчиталась успехом, не дождавшись выхода")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("в ошибке нет подсказки про --force: %v", err)
	}
	if !processAlive(cmd.Process.Pid) {
		t.Error("процесс убит без force — результат задачи потерян бы молча")
	}
	// pid-файл не должен исчезнуть: демон-то жив.
	if st, _ := readPidFile(); st == nil {
		t.Error("pid-файл убран, хотя процесс жив")
	}
}

// С force добиваем и обязательно убираем pid-файл: убитый процесс сам этого
// не сделает, а оставшийся файл соврёт следующему запуску.
func TestStop_ForceKillsAndCleans(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cmd := exec.Command("sh", "-c", "trap '' TERM; sleep 30")
	if err := cmd.Start(); err != nil {
		t.Skipf("не запустить sh: %v", err)
	}
	pid := cmd.Process.Pid
	go func() { _, _ = cmd.Process.Wait() }() // не оставлять зомби

	p := filepath.Join(dir, "execai", "serve.pid")
	_ = os.MkdirAll(filepath.Dir(p), 0o700)
	_ = os.WriteFile(p, []byte(strconv.Itoa(pid)+" 1780000000 https://x"), 0o600)

	if err := Stop(1*time.Second, true); err != nil {
		t.Fatalf("force не сработал: %v", err)
	}
	if processAlive(pid) {
		t.Error("процесс жив после --force")
	}
	if st, _ := readPidFile(); st != nil {
		t.Error("pid-файл остался — следующий запуск решит, что демон работает")
	}
}

// Остановка несуществующего демона — не ошибка, а сообщение. И заодно уборка
// протухшего файла.
func TestStop_NotRunning(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := Stop(time.Second, false); err != nil {
		t.Errorf("остановка незапущенного вернула ошибку: %v", err)
	}
	if err := Stop(time.Second, true); err != nil {
		t.Errorf("force-остановка незапущенного вернула ошибку: %v", err)
	}
}

// Демон, запущенный из TUI, обязан пережить закрытие терминала — иначе он
// умрёт ровно тогда, когда должен работать. Проверяем сам факт отвязки:
// у процесса должен смениться sid (Unix).
func TestSpawn_RefusesWhenAlreadyRunning(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := writePidFile("https://api.example.com"); err != nil {
		t.Fatal(err)
	}
	// В pid-файле — мы сами, значит «уже запущен».
	_, err := Spawn()
	if err == nil {
		t.Fatal("Spawn поднял второй демон поверх работающего")
	}
	if !strings.Contains(err.Error(), "уже запущен") {
		t.Errorf("непонятная причина отказа: %v", err)
	}
}

func TestAlreadyRunning(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if running, _ := AlreadyRunning(); running {
		t.Error("демон найден на пустом конфиге")
	}
	_ = writePidFile("https://api.example.com")
	running, pid := AlreadyRunning()
	if !running || pid != os.Getpid() {
		t.Errorf("AlreadyRunning() = (%v, %d), ожидалось (true, %d)", running, pid, os.Getpid())
	}
}

func TestSpawnLogPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	p := SpawnLogPath()
	if !strings.HasPrefix(p, dir) || !strings.HasSuffix(p, "serve.out.log") {
		t.Errorf("путь лога %q", p)
	}
}
