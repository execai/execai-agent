package sync

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	gosync "sync"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/crypto"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// holdList makes the next GET /sync/blobs wait until the returned release
// is called; entered is closed once that request arrived. Later requests
// pass.
func holdList(stub *stubServer) (entered <-chan struct{}, release func()) {
	in := make(chan struct{})
	out := make(chan struct{})
	var once gosync.Once
	stub.mu.Lock()
	stub.onList = func() {
		first := false
		once.Do(func() { first = true })
		if first {
			close(in)
			<-out
		}
	}
	stub.mu.Unlock()
	var rel gosync.Once
	return in, func() { rel.Do(func() { close(out) }) }
}

func (s *stubServer) lists() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listCalls
}

// Two processes of one computer (the chat and execai serve) share the
// config directory. While one of them is in the middle of an exchange, the
// other one's exchange waits: it does not run alongside on the same
// sync.json.
func TestExchangeLock_SecondProcessWaits(t *testing.T) {
	i18n.SetLocale("en")
	stub := newStub(t)
	a := newMachine(t, stub, "a")
	a.converse(a.svc.OnFlow(), password, password)
	daemon, err := New(stub.URL) // the other process: same directory, own Service
	if err != nil {
		t.Fatal(err)
	}
	before := stub.lists()
	entered, release := holdList(stub)
	defer release()
	daemonDone := make(chan string, 1)
	go func() { daemonDone <- daemon.SyncNow(context.Background()) }()
	<-entered

	chatDone := make(chan string, 1)
	go func() { chatDone <- a.svc.SyncNow(context.Background()) }()
	time.Sleep(300 * time.Millisecond)
	if n := stub.lists() - before; n != 1 {
		t.Errorf("exchanges running at once: %d, want 1 — the second must wait for the first", n)
	}
	release()
	for _, ch := range []chan string{daemonDone, chatDone} {
		select {
		case out := <-ch:
			if !strings.Contains(out, "Done") {
				t.Errorf("exchange: %s", out)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("an exchange did not finish")
		}
	}
}

// /sync off typed in the chat while execai serve is in the middle of an
// exchange: the daemon saves the state it loaded before, with sync on —
// without a lock between the processes that save undoes the person's
// /sync off.
func TestExchangeLock_OffDuringExchangeNotLost(t *testing.T) {
	i18n.SetLocale("en")
	stub := newStub(t)
	a := newMachine(t, stub, "a")
	a.converse(a.svc.OnFlow(), password, password)
	daemon, err := New(stub.URL)
	if err != nil {
		t.Fatal(err)
	}
	entered, release := holdList(stub)
	defer release()
	daemonDone := make(chan string, 1)
	go func() { daemonDone <- daemon.SyncNow(context.Background()) }()
	<-entered

	offDone := make(chan string, 1)
	go func() { offDone <- a.svc.Off(context.Background()) }()
	time.Sleep(200 * time.Millisecond)
	release()
	for _, ch := range []chan string{daemonDone, offDone} {
		select {
		case <-ch:
		case <-time.After(10 * time.Second):
			t.Fatal("did not finish")
		}
	}
	st, err := LoadState(a.svc.Paths)
	if err != nil {
		t.Fatal(err)
	}
	if st.Enabled {
		t.Fatal("/sync off was lost: the other process's exchange saved its old state over it")
	}
}

// TestHelperHoldsSyncLock is not a test: run as a child process by
// TestExchangeLock_OtherProcessAndCrash, it takes the lock, says so and
// waits to be killed.
func TestHelperHoldsSyncLock(t *testing.T) {
	path := os.Getenv("EXECAI_SYNC_LOCK_HELPER")
	if path == "" {
		t.Skip("helper process for TestExchangeLock_OtherProcessAndCrash")
	}
	release, err := acquireExchange(context.Background(), path, 5*time.Second)
	if err != nil {
		fmt.Println("error:", err)
		os.Exit(2)
	}
	fmt.Println("held")
	time.Sleep(time.Minute)
	release()
	os.Exit(0)
}

// A real second process holds the lock: this one cannot take it and says
// busy after its wait. The holder is killed (no clean exit, no release):
// the lock goes with it at once — a crash never leaves sync blocked.
func TestExchangeLock_OtherProcessAndCrash(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sync.lock")
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperHoldsSyncLock$")
	cmd.Env = append(os.Environ(), "EXECAI_SYNC_LOCK_HELPER="+path)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		if !waited {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	lines := make(chan string, 4)
	go func() {
		sc := bufio.NewScanner(out)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	select {
	case l := <-lines:
		if l != "held" {
			t.Fatalf("helper: %q", l)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("the helper process did not take the lock")
	}

	start := time.Now()
	if _, err := acquireExchange(context.Background(), path, 300*time.Millisecond); !errors.Is(err, errBusy) {
		t.Fatalf("taken while another process holds it: %v", err)
	}
	if d := time.Since(start); d < 250*time.Millisecond {
		t.Errorf("gave up after %v, before its wait", d)
	}

	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	waited = true
	start = time.Now()
	release, err := acquireExchange(context.Background(), path, 5*time.Second)
	if err != nil {
		t.Fatalf("the lock outlived the killed process: %v", err)
	}
	release()
	if d := time.Since(start); d > time.Second {
		t.Errorf("took %v to get the lock of a dead process", d)
	}
}

// Two owners in one process (two Services, as the chat may hold) exclude
// each other too; a cancelled wait gives up at once.
func TestExchangeLock_TwoOwnersOneProcess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sync.lock")
	r1, err := acquireExchange(context.Background(), path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acquireExchange(context.Background(), path, 100*time.Millisecond); !errors.Is(err, errBusy) {
		t.Fatalf("second owner got in: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if _, err := acquireExchange(ctx, path, time.Minute); !errors.Is(err, errBusy) || time.Since(start) > time.Second {
		t.Fatalf("cancelled wait: %v after %v", err, time.Since(start))
	}
	r1()
	r2, err := acquireExchange(context.Background(), path, time.Second)
	if err != nil {
		t.Fatalf("not free after release: %v", err)
	}
	r2()
}

// While another process exchanges, a background round of this one is
// skipped: no request, nothing shown, the change not marked as sent; a
// command the person typed says sync is busy. Once the lock is free the
// runner sends the change by itself, without waiting for a new edit.
func TestExchangeLock_BackgroundSkipsSilentlyAndRetries(t *testing.T) {
	i18n.SetLocale("en")
	stub := newStub(t)
	a := newMachine(t, stub, "a")
	a.converse(a.svc.OnFlow(), password, password)
	svc := a.svc
	svc.busyWait, svc.CommandWait = 100*time.Millisecond, 100*time.Millisecond

	other, err := acquireExchange(context.Background(), svc.Paths.LockFile(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	released := false
	defer func() {
		if !released {
			other()
		}
	}()

	a.writeFile("memory/busy.md", "written while another process syncs\n")
	var notes []string
	var mu gosync.Mutex
	hooks := Hooks{Notify: func(s string) { mu.Lock(); notes = append(notes, s); mu.Unlock() }}
	before := stub.lists()
	fp := svc.getLastFP()
	if svc.background(context.Background(), hooks, false) {
		t.Fatal("the round was not reported as skipped")
	}
	if stub.lists() != before {
		t.Error("an exchange ran while another process held the lock")
	}
	if svc.getLastFP() != fp {
		t.Error("the unsent change was marked as sent")
	}
	if out := svc.SyncNow(context.Background()); out != i18n.T("sync.busy") {
		t.Errorf("/sync now while busy: %q", out)
	}
	if out := svc.Off(context.Background()); out != i18n.T("sync.busy") {
		t.Errorf("/sync off while busy: %q", out)
	}
	if st, _ := LoadState(svc.Paths); !st.Enabled {
		t.Error("/sync off went through without the lock")
	}

	// The runner: its push round is skipped, then retried once free.
	svc.WatchEvery, svc.PushDelay, svc.PullEvery = 20*time.Millisecond, 100*time.Millisecond, time.Hour
	stop := svc.Start(context.Background(), hooks)
	defer stop()
	time.Sleep(300 * time.Millisecond)
	key, _ := crypto.LoadIdentity(a.dir())
	name := memoryName(key, "busy.md")
	if stub.blob(KindMemory, name) != nil {
		t.Fatal("pushed while another process held the lock")
	}
	other()
	released = true
	deadline := time.Now().Add(5 * time.Second)
	for stub.blob(KindMemory, name) == nil && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if stub.blob(KindMemory, name) == nil {
		t.Fatal("the skipped change was not sent after the lock was freed")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(notes) != 0 {
		t.Errorf("messages shown for a busy lock: %q", notes)
	}
}

// holdLock — another process holds the exchange lock of m's config
// directory for the rest of the test; m's commands wait only briefly.
func holdLock(t *testing.T, m *machine) {
	t.Helper()
	m.svc.busyWait, m.svc.CommandWait = 100*time.Millisecond, 100*time.Millisecond
	release, err := acquireExchange(context.Background(), m.svc.Paths.LockFile(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
}

// /sync on writes the sync key (sync.key) — on the first computer and when
// joining. While another process of this computer holds the exchange lock
// nothing is written: the step says sync is busy, and the server gets no
// new key either.
func TestExchangeLock_SyncOnKeyWriteWaits(t *testing.T) {
	i18n.SetLocale("en")
	stub := newStub(t)

	first := newMachine(t, stub, "first")
	holdLock(t, first)
	steps := first.converse(first.svc.OnFlow(), password, password)
	if _, err := os.Stat(filepath.Join(first.dir(), "sync.key")); err == nil {
		t.Error("first computer: sync.key written while another process held the lock")
	}
	if last := steps[len(steps)-1]; last.Text != i18n.T("sync.busy") {
		t.Errorf("first computer: %q", last.Text)
	}
	stub.mu.Lock()
	sent := stub.identity != nil
	stub.mu.Unlock()
	if sent {
		t.Error("first computer: the key went to the server while the lock was held")
	}

	a := newMachine(t, stub, "a")
	a.converse(a.svc.OnFlow(), password, password)
	b := newMachine(t, stub, "b")
	holdLock(t, b)
	steps = b.converse(b.svc.OnFlow(), password)
	if _, err := os.Stat(filepath.Join(b.dir(), "sync.key")); err == nil {
		t.Error("joining: sync.key written while another process held the lock")
	}
	if last := steps[len(steps)-1]; last.Text != i18n.T("sync.busy") {
		t.Errorf("joining: %q", last.Text)
	}
}

// A key import rewrites subscriptions.json — a file the exchange of another
// process reads and writes. While that process holds the lock the import
// says busy and leaves the file alone.
func TestExchangeLock_KeysImportWaits(t *testing.T) {
	i18n.SetLocale("en")
	stub := newStub(t)
	a := newMachine(t, stub, "a")
	a.writeFile("subscriptions.json", `{"active":"zai","subscriptions":{"zai":{"provider":"zai","api_key":"sk-zai-0123456789abcdef"}}}`)
	out := a.svc.KeysExport()
	code := regexp.MustCompile(`execai-keys-1:\S+`).FindString(out)
	phrase := ""
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "    ") {
			phrase = strings.TrimSpace(l)
		}
	}
	if code == "" || phrase == "" {
		t.Fatalf("export: %s", out)
	}

	c := newMachine(t, stub, "c")
	c.writeFile("subscriptions.json", `{"subscriptions":{}}`)
	holdLock(t, c)
	steps := c.converse(c.svc.KeysImportFlow(code), phrase)
	if got := c.readFile("subscriptions.json"); got != `{"subscriptions":{}}` {
		t.Errorf("subscriptions.json rewritten while another process held the lock: %s", got)
	}
	if last := steps[len(steps)-1]; last.Text != i18n.T("sync.busy") {
		t.Errorf("import: %q", last.Text)
	}
}
