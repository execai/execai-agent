//go:build !windows

package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// execai sync now|off from a shell while another ExecAI process holds the
// sync lock: the command fails — non-zero exit (main turns the returned
// error into stderr and code 1) — instead of printing «busy» and exiting 0.
func TestSyncCmd_BusyFailsInTheShell(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv("EXECAI_LOCALE", "en")
	billing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tariff_code":"week_work","state":"active","valid_until":"2099-01-01T00:00:00Z","is_accessible":true}`))
	}))
	defer billing.Close()
	cfgDir := filepath.Join(dir, "execai")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(`{"api_base":"`+billing.URL+`","locale":"en"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "sync.json"), []byte(`{"enabled":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveCredentials(&config.Credentials{Token: "tok", AgentID: "agent-1"}); err != nil {
		t.Fatal(err)
	}
	lock, err := os.OpenFile(filepath.Join(cfgDir, "sync.lock"), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		t.Skipf("no flock: %v", err)
	}
	prevWait := syncCommandWait
	syncCommandWait = 100 * time.Millisecond
	defer func() { syncCommandWait = prevWait }()

	for _, sub := range []string{"now", "off"} {
		stdout := captureStdout(t, func() {
			cmd := newSyncCmd()
			cmd.SetArgs([]string{sub})
			err = cmd.ExecuteContext(context.Background())
		})
		i18n.SetLocale("en")
		if err == nil || err.Error() != i18n.T("sync.busy") {
			t.Errorf("execai sync %s while busy: err %v", sub, err)
		}
		if strings.Contains(stdout, i18n.T("sync.busy")) {
			t.Errorf("execai sync %s: the refusal went to stdout: %q", sub, stdout)
		}
	}
}

func captureStdout(t *testing.T, run func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	prev := os.Stdout
	os.Stdout = w
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	run()
	os.Stdout = prev
	_ = w.Close()
	return <-done
}

// execai sync on from a shell without the plan: refused with a non-zero
// exit, the refusal on stderr.
func TestSyncCmd_NoPlanFailsInTheShell(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv("EXECAI_LOCALE", "en")
	billing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"user_id":"u1","subscription":null,"is_accessible":false}`))
	}))
	defer billing.Close()
	cfgDir := filepath.Join(dir, "execai")
	_ = os.MkdirAll(cfgDir, 0o700)
	_ = os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(`{"api_base":"`+billing.URL+`","locale":"en"}`), 0o600)
	if err := config.SaveCredentials(&config.Credentials{Token: "tok", AgentID: "agent-1"}); err != nil {
		t.Fatal(err)
	}
	var err error
	stdout := captureStdout(t, func() {
		cmd := newSyncCmd()
		cmd.SetArgs([]string{"on"})
		cmd.SetIn(strings.NewReader(""))
		err = cmd.ExecuteContext(context.Background())
	})
	if err == nil || !strings.Contains(err.Error(), "\"Work\"") {
		t.Errorf("execai sync on without the plan: err %v, stdout %q", err, stdout)
	}
	if strings.Contains(stdout, "\"Work\"") {
		t.Errorf("the refusal went to stdout: %q", stdout)
	}
}
