package main

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	agentsync "github.com/velesbsdllc/agent-vbai/internal/sync"
)

// execai ide: what sync says reaches the panel as notice events, and the
// stdout — the editor's protocol — carries nothing but JSON lines, whatever
// sync says and whenever.
func TestIDE_SyncLinesReachThePanelAsJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("EXECAI_IDE_INBOX", "off")
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/billing-vbai/models_public" {
			_, _ = w.Write([]byte(`[{"id":"m1","provider":"p","name":"M1","is_primary":true}]`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer api.Close()
	cfg := config.Defaults()
	cfg.APIBase = api.URL
	cfg.Locale = "en"
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	var hookCmd string
	var mu sync.Mutex
	notify := func(string) {}
	old := startIDESync
	startIDESync = func(ctx context.Context, _ *config.Config, h agentsync.Hooks) func() {
		mu.Lock()
		hookCmd = h.Cmd
		if h.Notify != nil {
			notify = h.Notify
		}
		mu.Unlock()
		// The first exchange speaks before the protocol is up.
		notify("Not synced (a symbolic link): ~/p/.execai — it stays as it is here.")
		notify("two\nlines \"quoted\" ")
		return func() {}
	}
	defer func() { startIDESync = old }()

	inR, inW, _ := os.Pipe()
	outR, outW, _ := os.Pipe()
	oldIn, oldOut := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = inR, outW
	defer func() { os.Stdin, os.Stdout = oldIn, oldOut }()

	var linesMu sync.Mutex
	var lines []string
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		sc := bufio.NewScanner(outR)
		sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
		for sc.Scan() {
			linesMu.Lock()
			lines = append(lines, sc.Text())
			linesMu.Unlock()
		}
	}()
	waitFor := func(sub string) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			linesMu.Lock()
			for _, l := range lines {
				if strings.Contains(l, sub) {
					linesMu.Unlock()
					return
				}
			}
			linesMu.Unlock()
			time.Sleep(20 * time.Millisecond)
		}
		linesMu.Lock()
		defer linesMu.Unlock()
		t.Fatalf("%q not seen in %q", sub, lines)
	}

	cmd := newIDECmd()
	cmd.SetArgs([]string{"--cwd", home})
	done := make(chan error, 1)
	go func() { done <- cmd.ExecuteContext(context.Background()) }()

	waitFor(`"type":"ready"`)
	_, _ = inW.Write([]byte(`{"type":"command","name":"resume_last"}` + "\n"))
	waitFor(`a symbolic link`)
	mu.Lock()
	n := notify
	mu.Unlock()
	n("a line from a later exchange")
	waitFor(`a line from a later exchange`)
	_ = inW.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("execai ide did not end on EOF")
	}
	_ = outW.Close()
	<-readDone

	if hookCmd != "execai sync" {
		t.Errorf("sync names its command %q in the panel", hookCmd)
	}
	got := map[string]bool{}
	for i, l := range lines {
		var o struct {
			Type, Text, Chat string
		}
		if err := json.Unmarshal([]byte(l), &o); err != nil {
			t.Fatalf("stdout line %d is not JSON: %q", i, l)
		}
		if o.Type == "notice" && o.Chat == "" {
			got[o.Text] = true
		}
	}
	for _, want := range []string{"two\nlines \"quoted\" ", "a line from a later exchange"} {
		if !got[want] {
			t.Errorf("notice %q not in the protocol", want)
		}
	}
}
