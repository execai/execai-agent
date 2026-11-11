package ide

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/notify"
)

// fakeDesktop puts a notify-send into a fresh PATH directory that appends
// its last argument (the text) to a log, and points desktopEnv at it.
func fakeDesktop(t *testing.T, vars map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "sent.log")
	script := "#!/bin/sh\nfor a in \"$@\"; do last=\"$a\"; done\nprintf '%s\\n' \"$last\" >> '" + log + "'\n"
	if err := os.WriteFile(filepath.Join(dir, "notify-send"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	prev := desktopEnv
	desktopEnv = func() notify.Env {
		return notify.Env{GOOS: "linux", Getenv: func(k string) string { return vars[k] },
			LookPath: func(name string) (string, error) {
				p := filepath.Join(dir, name)
				if _, err := os.Stat(p); err != nil {
					return "", err
				}
				return p, nil
			}}
	}
	t.Cleanup(func() { desktopEnv = prev })
	return log
}

func readSent(path string) []string {
	data, _ := os.ReadFile(path)
	s := strings.TrimSpace(string(data))
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// waitSent waits for n lines in the log of the fake notify-send.
func waitSent(t *testing.T, path string, n int) []string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if got := readSent(path); len(got) >= n {
			return got
		}
		time.Sleep(20 * time.Millisecond)
	}
	got := readSent(path)
	t.Fatalf("want %d notifications, have %q", n, got)
	return got
}

// The whole path through Run: ready says the agent can show a desktop
// notification; the panel asks with a fixed kind, the agent builds the text
// itself (folder or tool name, nothing else) and writes nothing to the
// protocol; an unknown kind or a tool name that is not a name are refused
// quietly; the rate is bounded.
func TestRun_NotifyCommand(t *testing.T) {
	dir := isolateIDE(t)
	proj := filepath.Join(dir, "secret-parent", "my-proj")
	if err := os.MkdirAll(proj, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(proj)
	log := fakeDesktop(t, map[string]string{"DISPLAY": ":0"})
	t.Setenv("EXECAI_IDE_INBOX", "off")

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/billing-vbai/models_public" {
			_, _ = w.Write([]byte(`[{"id":"m1","provider":"p","name":"M1","is_primary":true}]`))
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer api.Close()
	cfg := config.Defaults()
	cfg.APIBase = api.URL
	cfg.Locale = "en"
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	buf := &syncBuf{}
	go func() {
		sc := bufio.NewScanner(outR)
		sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
		for sc.Scan() {
			_, _ = buf.Write(append(sc.Bytes(), '\n'))
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Run(ctx, cfg, Options{Cwd: proj, In: inR, Out: outW}) }()
	send := func(line string) {
		if _, err := io.WriteString(inW, line+"\n"); err != nil {
			t.Fatal(err)
		}
	}

	ready := buf.waitEvent(t, func(o Out) bool { return o.Type == "ready" })
	if len(ready.Features) != 1 || ready.Features[0] != "notify" {
		t.Fatalf("ready features %v", ready.Features)
	}
	before := len(buf.events())
	send(`{"type":"command","name":"notify","value":"turn_done"}`)
	send(`{"type":"command","name":"notify","value":"approval","tool":"Bash"}`)
	send(`{"type":"command","name":"notify","value":"ask"}`)
	send(`{"type":"command","name":"notify","value":"approval","tool":"x\"; rm -rf ~ #\n\u001b]0;pwn"}`)
	send(`{"type":"command","name":"notify","value":"$(reboot)"}`)
	got := waitSent(t, log, 4)
	want := []string{
		"ExecAI: turn finished — my-proj",
		"ExecAI is waiting for approval: Bash",
		"ExecAI has a question",
		"ExecAI is waiting for approval",
	}
	// Each goes out on its own goroutine: the order is not fixed.
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("sent %q", got)
	}
	// Bounded: a flood is cut.
	for i := 0; i < 10; i++ {
		send(`{"type":"command","name":"notify","value":"ask"}`)
	}
	send(`{"type":"command","name":"state"}`)
	buf.waitEvent(t, func(o Out) bool { return o.Type == "state" })
	time.Sleep(300 * time.Millisecond)
	if n := len(readSent(log)); n > desktopBurst {
		t.Fatalf("%d notifications in a burst", n)
	}
	for _, o := range buf.events()[before:] {
		if o.Type == "error" || o.Type == "notice" {
			t.Errorf("notify answered on the protocol: %+v", o)
		}
	}

	_ = inW.Close()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return")
	}
}

// No desktop program here (over SSH), or notifications off in the config:
// ready does not offer it, and the panel keeps its own toasts.
func TestReadyFeatures(t *testing.T) {
	fakeDesktop(t, map[string]string{"DISPLAY": ":0"})
	if f := readyFeatures(&config.Config{}); len(f) != 1 || f[0] != "notify" {
		t.Fatalf("features %v", f)
	}
	if f := readyFeatures(&config.Config{Notifications: "off"}); len(f) != 0 {
		t.Fatalf("off: %v", f)
	}
	fakeDesktop(t, map[string]string{"DISPLAY": ":0", "SSH_CONNECTION": "1 2 3 4"})
	if f := readyFeatures(&config.Config{}); len(f) != 0 {
		t.Fatalf("ssh: %v", f)
	}
}
