package ide

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// What the background sync says to the person — a copy kept next to a
// file, a file left out, the server's version taken — reaches the panel of
// the editor as a notice event (in execai ide the stdout is the protocol,
// nothing else may be printed there).

// runWithNotices starts Run over pipes with n; returns the writer of the
// panel's commands and what Run wrote.
func runWithNotices(t *testing.T, n *SyncNotices, out io.Writer) (io.WriteCloser, context.CancelFunc) {
	t.Helper()
	dir := isolateIDE(t)
	t.Chdir(dir)
	t.Setenv("EXECAI_IDE_INBOX", "off")
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/billing-vbai/models_public" {
			_, _ = w.Write([]byte(`[{"id":"m1","provider":"p","name":"M1","is_primary":true}]`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(api.Close)
	cfg := config.Defaults()
	cfg.APIBase = api.URL
	cfg.Locale = "en"
	inR, inW := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = Run(ctx, cfg, Options{Cwd: dir, In: inR, Out: out, SyncNotices: n})
	}()
	t.Cleanup(func() {
		cancel()
		_ = inW.Close()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("Run did not return")
		}
	})
	return inW, cancel
}

func send(t *testing.T, w io.Writer, line string) {
	t.Helper()
	if _, err := w.Write([]byte(line + "\n")); err != nil {
		t.Fatal(err)
	}
}

// indexOf — the position of the first event matching ok, -1 when none.
func indexOf(evs []Out, ok func(Out) bool) int {
	for i, o := range evs {
		if ok(o) {
			return i
		}
	}
	return -1
}

func isNotice(text string) func(Out) bool {
	return func(o Out) bool { return o.Type == "notice" && o.Chat == "" && o.Text == text }
}

// (a) A line sync said before the protocol was up (the first exchange runs
// before ready) is not lost and not mixed into the start: it comes after
// ready and after the answer to the panel's first command (resume_last —
// the released panel takes any notice without a chat for that answer).
func TestSyncNotices_BeforeReadyComeAfterTheFirstCommand(t *testing.T) {
	n := NewSyncNotices(0)
	n.Notify("a copy is kept next to EXECAI.md")
	out := &syncBuf{}
	in, _ := runWithNotices(t, n, out)
	out.waitEvent(t, func(o Out) bool { return o.Type == "ready" })
	time.Sleep(300 * time.Millisecond)
	if indexOf(out.events(), isNotice("a copy is kept next to EXECAI.md")) >= 0 {
		t.Fatal("the line went out before the panel's first command")
	}
	send(t, in, `{"type":"command","name":"resume_last"}`)
	out.waitEvent(t, isNotice("a copy is kept next to EXECAI.md"))
	evs := out.events()
	ready := indexOf(evs, func(o Out) bool { return o.Type == "ready" })
	answer := indexOf(evs, isNotice(i18n.T("ide.resume.none")))
	line := indexOf(evs, isNotice("a copy is kept next to EXECAI.md"))
	if !(ready < answer && answer < line) {
		t.Errorf("order: ready %d, answer to resume_last %d, the sync line %d", ready, answer, line)
	}
}

// Without such a command the lines go out a few seconds after ready.
func TestSyncNotices_GoOutAfterAWhileWithoutACommand(t *testing.T) {
	old := syncNoticeDelay
	syncNoticeDelay = 300 * time.Millisecond
	t.Cleanup(func() { syncNoticeDelay = old })
	n := NewSyncNotices(0)
	n.Notify("left out of sync")
	out := &syncBuf{}
	runWithNotices(t, n, out)
	out.waitEvent(t, isNotice("left out of sync"))
}

// In execai ui nobody may be looking when sync speaks: the page is opened
// later than the agent starts. The lines wait for a page that listens — the
// few seconds after ready do not let them out to nobody — and come after
// the answer to that page's first command.
func TestSyncNotices_WaitForAPageThatListens(t *testing.T) {
	old := syncNoticeDelay
	syncNoticeDelay = 200 * time.Millisecond
	t.Cleanup(func() { syncNoticeDelay = old })
	n := NewSyncNotices(0)
	var listening atomic.Bool
	n.WhileListening(listening.Load)
	n.Notify("a copy is kept while nobody looks")
	out := &syncBuf{}
	in, _ := runWithNotices(t, n, out)
	out.waitEvent(t, func(o Out) bool { return o.Type == "ready" })
	time.Sleep(700 * time.Millisecond)
	if indexOf(out.events(), isNotice("a copy is kept while nobody looks")) >= 0 {
		t.Fatal("the line went out while no page was listening")
	}
	listening.Store(true)
	send(t, in, `{"type":"command","name":"resume_last"}`)
	out.waitEvent(t, isNotice("a copy is kept while nobody looks"))
	evs := out.events()
	answer := indexOf(evs, isNotice(i18n.T("ide.resume.none")))
	line := indexOf(evs, isNotice("a copy is kept while nobody looks"))
	if !(answer >= 0 && answer < line) {
		t.Errorf("order: answer to resume_last %d, the sync line %d", answer, line)
	}
	// The page is closed again: a later line waits for the next page.
	listening.Store(false)
	n.Notify("said to a closed page")
	time.Sleep(400 * time.Millisecond)
	if indexOf(out.events(), isNotice("said to a closed page")) >= 0 {
		t.Fatal("a later line went out while no page was listening")
	}
	listening.Store(true)
	send(t, in, `{"type":"command","name":"resume_last"}`)
	out.waitEvent(t, isNotice("said to a closed page"))
}

// (b) A line from a later exchange (the background round every few
// minutes) goes out at once.
func TestSyncNotices_LaterLinesGoOutAtOnce(t *testing.T) {
	n := NewSyncNotices(0)
	out := &syncBuf{}
	in, _ := runWithNotices(t, n, out)
	out.waitEvent(t, func(o Out) bool { return o.Type == "ready" })
	send(t, in, `{"type":"new_chat"}`)
	out.waitEvent(t, func(o Out) bool { return o.Type == "chat_reset" })
	start := time.Now()
	n.Notify("the server's version is used here now")
	out.waitEvent(t, isNotice("the server's version is used here now"))
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("took %v", d)
	}
}

// (c) A panel that does not read must not stop sync: Notify never blocks;
// the oldest lines over the limit go, and the panel is told how many.
func TestSyncNotices_AReaderThatDoesNotReadDoesNotStopSync(t *testing.T) {
	n := NewSyncNotices(10)
	outR, outW := io.Pipe()
	in, _ := runWithNotices(t, n, outW)
	finished := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			n.Notify("line " + strings.Repeat("x", i%7))
		}
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("Notify blocked while nobody read the protocol")
	}
	out := &syncBuf{}
	go func() { _, _ = io.Copy(out, outR) }()
	out.waitEvent(t, func(o Out) bool { return o.Type == "ready" })
	send(t, in, `{"type":"new_chat"}`)
	more := out.waitEvent(t, func(o Out) bool {
		return o.Type == "notice" && strings.Contains(o.Text, "990")
	})
	time.Sleep(200 * time.Millisecond)
	lines := 0
	for _, o := range out.events() {
		if o.Type == "notice" && strings.HasPrefix(o.Text, "line ") {
			lines++
		}
	}
	if lines != 10 {
		t.Errorf("%d lines went out, want the newest 10 (and %q)", lines, more.Text)
	}
}

// (d) Whatever sync says, every line Run writes is one JSON message.
func TestSyncNotices_TheStreamStaysJSON(t *testing.T) {
	n := NewSyncNotices(0)
	nasty := []string{
		"two\nlines", `a "quoted" path C:\Users\x`, "line\u2028separator", "\x1b[31mred\x1b[0m",
		strings.Repeat("long ", 5000), "",
	}
	for _, s := range nasty {
		n.Notify(s)
	}
	out := &syncBuf{}
	in, _ := runWithNotices(t, n, out)
	out.waitEvent(t, func(o Out) bool { return o.Type == "ready" })
	send(t, in, `{"type":"new_chat"}`)
	out.waitEvent(t, isNotice("two\nlines"))
	n.Notify("after\nthe start")
	out.waitEvent(t, isNotice("after\nthe start"))
	out.mu.Lock()
	raw := out.b.String()
	out.mu.Unlock()
	for i, line := range strings.Split(strings.TrimRight(raw, "\n"), "\n") {
		var v map[string]any
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			t.Fatalf("line %d is not JSON: %q", i, line)
		}
	}
	for _, s := range nasty[:5] {
		if indexOf(out.events(), isNotice(s)) < 0 {
			t.Errorf("not delivered as it was: %q", s[:min(len(s), 20)])
		}
	}
}
