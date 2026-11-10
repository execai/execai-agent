package machine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/connect"
	"github.com/velesbsdllc/agent-vbai/internal/inbox"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
	agentsync "github.com/velesbsdllc/agent-vbai/internal/sync"

	qrcode "github.com/skip2/go-qrcode"
)

func isolate(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	return filepath.Join(dir, "execai")
}

// The marks live in the config directory but outside config.json: that file
// travels to the other computers with sync, and "the phone was set up" is
// about this machine.
func TestMarks_FirstTimeOnly(t *testing.T) {
	dir := isolate(t)
	if m := LoadMarks(); !m.PhoneShownAt.IsZero() || !m.WebCallAt.IsZero() {
		t.Fatalf("marks of a fresh machine: %+v", m)
	}
	if !MarkPhoneShown() {
		t.Fatal("first /phone is not reported as first")
	}
	if MarkPhoneShown() {
		t.Fatal("second /phone reported as first")
	}
	if !MarkWebCall() || MarkWebCall() {
		t.Fatal("web call mark")
	}
	m := LoadMarks()
	if m.PhoneShownAt.IsZero() || m.WebCallAt.IsZero() {
		t.Fatalf("marks not saved: %+v", m)
	}
	if _, err := os.Stat(filepath.Join(dir, MarksFile)); err != nil {
		t.Fatalf("marks file: %v", err)
	}
	for _, name := range agentsync.ConfigFiles {
		if name == MarksFile {
			t.Fatalf("%s would travel with sync", MarksFile)
		}
	}
}

// /machine check asks every source the way /connect does and remembers the
// answer, failures included.
func TestCheckSources(t *testing.T) {
	isolate(t)
	prev := connect.Fetch
	t.Cleanup(func() { connect.Fetch = prev })
	connect.Fetch = func(sub subscriptions.Subscription) ([]connect.ModelMeta, int) {
		if sub.Provider == "zai" {
			return []connect.ModelMeta{{ID: "glm"}}, 200
		}
		return nil, 401
	}
	subs := &subscriptions.Store{Subscriptions: map[string]subscriptions.Subscription{
		"zai":    {Provider: "zai", APIKey: "k"},
		"openai": {Provider: "openai", APIKey: "bad"},
	}}
	got := CheckSources(context.Background(), subs)
	if !got["zai"].OK || got["openai"].OK || got["openai"].Status != 401 || got["zai"].At.IsZero() {
		t.Fatalf("checks %+v", got)
	}
	saved := LoadChecks()
	if !saved["zai"].OK || saved["openai"].OK {
		t.Fatalf("not saved: %+v", saved)
	}
	// No key, no request: the source is reported as not answering.
	subs.Subscriptions["kimi"] = subscriptions.Subscription{Provider: "kimi"}
	if c := CheckSources(context.Background(), subs)["kimi"]; c.OK || c.At.IsZero() {
		t.Fatalf("kimi without a key: %+v", c)
	}
}

// Local reads the machine's own files: sync.json, the inbox lock, the marks.
func TestLocal_ReadsFiles(t *testing.T) {
	dir := isolate(t)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	st := &agentsync.State{Enabled: true, LastPush: time.Now()}
	if err := st.Save(agentsync.Paths{ConfigDir: dir}); err != nil {
		t.Fatal(err)
	}
	release, err := inbox.Acquire(inbox.KindServe, "https://api.execai.ru", "")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	MarkPhoneShown()

	cr := &config.Credentials{Token: "t", AgentID: "a1"}
	in := Local("https://api.execai.ru", cr, nil, nil)
	if in.Sync == nil || !in.Sync.Enabled {
		t.Errorf("sync state not read: %+v", in.Sync)
	}
	if in.Listener == nil || in.Listener.Kind != inbox.KindServe {
		t.Errorf("listener not seen: %+v", in.Listener)
	}
	if in.Marks.PhoneShownAt.IsZero() {
		t.Error("marks not read")
	}
	if in.Bindings != -1 || in.OS == "" || in.Version == "" || in.Hostname == "" {
		t.Errorf("defaults: %+v", in)
	}
	s := Build(in)
	if !s.Machine.Presence.ListenerRunning || s.Machine.Presence.ListenerKind != inbox.KindServe {
		t.Errorf("presence %+v", s.Machine.Presence)
	}
}

// The QR code is drawn with half blocks: two rows of modules per line, the
// quiet zone included, nothing but the four glyphs.
func TestQR_Lines(t *testing.T) {
	lines, err := QRLines("https://execai.ru/m/agent-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) < 10 {
		t.Fatalf("%d lines", len(lines))
	}
	width := len([]rune(lines[0]))
	for i, l := range lines {
		if n := len([]rune(l)); n != width {
			t.Fatalf("line %d width %d, want %d", i, n, width)
		}
		for _, r := range l {
			if !strings.ContainsRune("█▀▄ ", r) {
				t.Fatalf("line %d has %q", i, r)
			}
		}
	}
	// A square code: width modules, ceil(width/2) lines.
	if want := (width + 1) / 2; len(lines) != want {
		t.Errorf("%d lines for width %d, want %d", len(lines), width, want)
	}
	// The quiet zone is light: the first line is all full blocks.
	if strings.Trim(lines[0], "█") != "" {
		t.Errorf("first line is not the quiet zone: %q", lines[0])
	}
	if _, err := QRLines(""); err == nil {
		t.Error("empty text gave a code")
	}

	// Reading the glyphs back gives exactly the modules of the code.
	q, err := qrcode.New("https://execai.ru/m/agent-1", qrcode.Medium)
	if err != nil {
		t.Fatal(err)
	}
	bm := q.Bitmap()
	for y := range bm {
		row := []rune(lines[y/2])
		for x := range bm[y] {
			r := row[x]
			var light bool
			if y%2 == 0 {
				light = r == '█' || r == '▀'
			} else {
				light = r == '█' || r == '▄'
			}
			if light == bm[y][x] {
				t.Fatalf("module %d,%d: glyph %q, dark=%v", y, x, r, bm[y][x])
			}
		}
	}
}
