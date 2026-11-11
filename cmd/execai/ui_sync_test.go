package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/ide"
)

// execai ui: what sync says goes to the terminal that started the page and,
// as notice events, to the page — the lines said before the protocol was up
// and the later ones. The page has no slash commands for sync, so sync is
// told to name the shell command.
func TestUI_SyncLinesReachThePageAndTheTerminal(t *testing.T) {
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

	var term lockedBuffer
	notices := ide.NewSyncNotices(0)
	hooks := uiSyncHooks(&term, notices)
	if hooks.Cmd != "execai sync" {
		t.Errorf("sync names its command %q on the page", hooks.Cmd)
	}
	// The first exchange speaks before the protocol is up.
	hooks.Notify("a copy was kept: ~/p/.execai/notes.md.local-1")

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		err := uiIDE(cfg, home, notices)(ctx, inR, outW)
		_ = outW.Close()
		done <- err
	}()

	waitFor(`"type":"ready"`)
	_, _ = inW.Write([]byte(`{"type":"command","name":"resume_last"}` + "\n"))
	waitFor(`a copy was kept`)
	hooks.Notify("two\nlines from a later exchange")
	waitFor(`lines from a later exchange`)
	_ = inW.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the agent behind the page did not end on EOF")
	}
	<-readDone

	got := map[string]bool{}
	for i, l := range lines {
		var o struct {
			Type, Text, Chat string
		}
		if err := json.Unmarshal([]byte(l), &o); err != nil {
			t.Fatalf("protocol line %d is not JSON: %q", i, l)
		}
		if o.Type == "notice" && o.Chat == "" {
			got[o.Text] = true
		}
	}
	for _, want := range []string{"a copy was kept: ~/p/.execai/notes.md.local-1", "two\nlines from a later exchange"} {
		if !got[want] {
			t.Errorf("notice %q did not reach the page", want)
		}
		if !strings.Contains(term.String(), want) {
			t.Errorf("line %q did not reach the terminal", want)
		}
	}
}

// The page is opened later than the agent starts (the address copied by
// hand, a slow browser): the stream the agent writes to says whether a page
// listens, and the sync lines wait for one instead of going out to nobody.
func TestUI_SyncLinesWaitForAPage(t *testing.T) {
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

	var term lockedBuffer
	notices := ide.NewSyncNotices(0)
	hooks := uiSyncHooks(&term, notices)
	hooks.Notify("a copy was kept before any page")

	inR, inW := io.Pipe()
	stream := &pageStream{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- uiIDE(cfg, home, notices)(ctx, inR, stream) }()

	stream.waitFor(t, `"type":"ready"`)
	// Well past the few seconds the editor panel is given: no page yet.
	time.Sleep(6 * time.Second)
	if strings.Contains(stream.String(), "a copy was kept before any page") {
		t.Fatal("the line went out while no page was connected")
	}
	stream.listening.Store(true)
	_, _ = inW.Write([]byte(`{"type":"command","name":"resume_last"}` + "\n"))
	stream.waitFor(t, "a copy was kept before any page")
	if !strings.Contains(term.String(), "a copy was kept before any page") {
		t.Error("the line did not reach the terminal")
	}
	_ = inW.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the agent behind the page did not end on EOF")
	}
}

// pageStream — what the agent writes to in execai ui: it keeps the bytes and
// says whether a page listens, as the hub of internal/uiserve does.
type pageStream struct {
	lockedBuffer
	listening atomic.Bool
}

func (p *pageStream) Listening() bool { return p.listening.Load() }

func (p *pageStream) waitFor(t *testing.T, sub string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(p.String(), sub) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%q not seen in %q", sub, p.String())
}

// lockedBuffer — a bytes.Buffer safe to write from the sync goroutine.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}
