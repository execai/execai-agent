package sync

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

const secretKey = "sk-zai-SECRET-0123456789"

// cfg writes this side's settings files with one mtime.
func (s *side) cfg(configJSON, subsJSON string, mtime time.Time) {
	s.t.Helper()
	s.write(filepath.Join(s.eng.Paths.ConfigDir, "config.json"), configJSON, mtime)
	s.write(filepath.Join(s.eng.Paths.ConfigDir, "subscriptions.json"), subsJSON, mtime)
}

// copiesOf — the paths of copies not sent of one kind, by reason (all
// reasons when none is given).
func copiesOf(rep Report, kind string, why ...reason) []string {
	var out []string
	for _, c := range rep.Unsent {
		if c.kind != kind {
			continue
		}
		if len(why) > 0 && c.why != why[0] {
			continue
		}
		out = append(out, c.path)
	}
	return out
}

// The settings blob changed on both computers: the server's version stays
// (it is what the other computers have) — whichever side changed later and
// whatever the clocks say — and the settings files of this side that differ
// are kept in <config>/sync-unsent/<date>/, mode 0600 — they may hold
// provider keys. The copies never travel: the settings blob still packs only
// the three settings files.
func TestEngine_ConfigBothChangedServerStays(t *testing.T) {
	subs := `{"active":"zai","subscriptions":{"zai":{"provider":"zai","api_key":"` + secretKey + `"}}}`
	cases := []struct {
		name         string
		aTime, bTime time.Time
		pushOnly     bool
	}{
		{"pull, this side later", past, future, false},
		{"pull, the other side later", future, past, false},
		{"push refused, this side later", past, future, true},
		{"push refused, the other side later", future, past, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stub := newStub(t)
			key := mustKey(t)
			a, b := newSide(t, stub, key), newSide(t, stub, key)
			a.cfg(`{"model":"v1"}`, subs, past)
			a.run(Options{})
			b.run(Options{})

			a.cfg(`{"model":"from-A"}`, subs, c.aTime)
			a.run(Options{PushOnly: true})
			b.cfg(`{"model":"from-B"}`, subs, c.bTime)
			writes := len(stub.keyFPs)
			rep := b.run(Options{PushOnly: c.pushOnly})
			if c.pushOnly {
				if n := len(stub.keyFPs) - writes; n != 1 {
					t.Errorf("a refused send-only exchange wrote %d times, want once", n)
				}
				// A send-only exchange takes nothing from the server: the
				// refused send waits for the next full exchange.
				if got := b.read(filepath.Join(b.eng.Paths.ConfigDir, "config.json")); got != `{"model":"from-B"}` {
					t.Fatalf("a send-only exchange took the server's settings: %s", got)
				}
				rep = b.run(Options{})
			}
			if got := b.read(filepath.Join(b.eng.Paths.ConfigDir, "config.json")); got != `{"model":"from-A"}` {
				t.Fatalf("config.json = %s, want the server's", got)
			}
			copies := b.conflictCopies()
			var paths []string
			for p := range copies {
				paths = append(paths, p)
			}
			sort.Strings(paths)
			if len(paths) != 1 || copies[paths[0]] != `{"model":"from-B"}` {
				t.Fatalf("copies %v, want only this side's config.json", copies)
			}
			p := paths[0]
			if base := filepath.Base(p); !strings.HasPrefix(base, "config.local-") || !strings.HasSuffix(base, ".json") {
				t.Errorf("copy name %s", base)
			}
			if fi, err := os.Stat(p); err != nil || fi.Mode().Perm() != 0o600 {
				t.Errorf("copy mode %v %v, want 0600", fi, err)
			}
			if got := copiesOf(rep, KindConfig, reasonBoth); len(got) != 1 || got[0] != p {
				t.Errorf("report names %v", got)
			}

			// The settings blob packs the three files only, nothing from
			// sync-unsent; the next exchange sends nothing new.
			it, ok, err := b.eng.configItem()
			if err != nil || !ok {
				t.Fatal(err)
			}
			files, _ := unpackFiles(it.Data)
			for name := range files {
				if name != "config.json" && name != "subscriptions.json" && name != "permissions.json" {
					t.Errorf("the settings blob carries %s", name)
				}
			}
			stub.mu.Lock()
			n := len(stub.blobs)
			stub.mu.Unlock()
			b.run(Options{})
			stub.mu.Lock()
			defer stub.mu.Unlock()
			if len(stub.blobs) != n {
				t.Errorf("blobs %d → %d: a copy travelled", n, len(stub.blobs))
			}
		})
	}
}

// Joining: the server's settings win (what the person came for); this
// computer's own settings that differ are kept as a copy.
func TestEngine_ConfigJoinKeepsLocalCopy(t *testing.T) {
	stub := newStub(t)
	key := mustKey(t)
	a, b := newSide(t, stub, key), newSide(t, stub, key)
	a.cfg(`{"model":"from-A"}`, `{}`, time.Time{})
	a.run(Options{})
	b.cfg(`{"model":"mine-before-joining"}`, `{}`, future)
	rep := b.run(Options{Join: true})
	if got := b.read(filepath.Join(b.eng.Paths.ConfigDir, "config.json")); got != `{"model":"from-A"}` {
		t.Fatalf("join: config.json = %s", got)
	}
	if _, ok := b.hasCopy(`{"model":"mine-before-joining"}`); !ok || len(copiesOf(rep, KindConfig, reasonJoin)) != 1 {
		t.Fatalf("own settings lost on joining: %v / %v", b.conflictCopies(), rep.Unsent)
	}
}

// The person is told where the settings copy is — never what is in it:
// neither the notice nor /sync status carries a provider key.
func TestConfigBackup_ToldWithoutContent(t *testing.T) {
	i18n.SetLocale("en")
	stub := newStub(t)
	a := newMachine(t, stub, "a")
	a.converse(a.svc.OnFlow(), password, password)
	b := newMachine(t, stub, "b")
	b.converse(b.svc.OnFlow(), password)

	const keyB = "sk-zai-B-ONLY-9876543210"
	a.use()
	a.writeFile("subscriptions.json", `{"active":"zai","subscriptions":{"zai":{"provider":"zai","api_key":"`+secretKey+`"}}}`)
	now(a)
	b.use()
	b.writeFile("subscriptions.json", `{"active":"zai","subscriptions":{"zai":{"provider":"zai","api_key":"`+keyB+`"}}}`)
	// B never agreed a version of the settings (A sent the first): row 5c —
	// the server's stays, B's is kept.

	var notes []string
	b.svc.background(context.Background(), Hooks{Notify: func(s string) { notes = append(notes, s) }}, false)
	if !strings.Contains(b.readFile("subscriptions.json"), secretKey) {
		t.Fatalf("the server's settings did not stay: %s", b.readFile("subscriptions.json"))
	}
	var copyPath string
	for _, n := range notes {
		if strings.Contains(n, "A settings file differs from the server's version, and there was no agreed earlier version") {
			copyPath = expandHome(n[strings.Index(n, "is kept: ")+len("is kept: ") : strings.LastIndex(n, ". If it holds")])
		}
		if strings.Contains(n, keyB) || strings.Contains(n, secretKey) {
			t.Errorf("a notice shows a provider key: %q", n)
		}
	}
	if copyPath == "" {
		t.Fatalf("no notice about the settings copy: %q", notes)
	}
	if got, err := os.ReadFile(copyPath); err != nil || !strings.Contains(string(got), keyB) {
		t.Fatalf("copy %s: %v", copyPath, err)
	}
	if fi, _ := os.Stat(copyPath); fi.Mode().Perm() != 0o600 {
		t.Errorf("copy mode %v", fi.Mode().Perm())
	}
	if st := b.svc.Status(); strings.Contains(st, keyB) || strings.Contains(st, secretKey) ||
		!strings.Contains(st, "Copies of memory and settings content that was not sent: 1") {
		t.Errorf("status:\n%s", st)
	}
}

// A second computer joins sync: its config.json has only what is its own by
// design (api_base, notifications), its sources are kept by the join, its
// permissions are a part of the account's. Nothing of it is lost — no copy
// and no message (the probe of 7.17 got «settings were changed both here and
// on another computer» every time).
func TestEngine_ConfigJoinNothingLostNoCopy(t *testing.T) {
	stub := newStub(t)
	key := mustKey(t)
	a, b := newSide(t, stub, key), newSide(t, stub, key)
	a.cfg(`{"api_base":"https://api.a","model":"glm","notifications":"off"}`,
		`{"active":"zai","subscriptions":{"zai":{"provider":"zai","api_key":"`+secretKey+`","models":[{"id":"m1"}]}}}`, time.Time{})
	a.write(filepath.Join(a.eng.Paths.ConfigDir, "permissions.json"), `{"always_allowed_tools":["Read","Grep"],"always_allowed_exact":[]}`, time.Time{})
	a.run(Options{})

	b.cfg(`{"api_base":"https://api.b","notifications":"on"}`,
		`{"active":"openai","subscriptions":{"openai":{"provider":"openai","api_key":"sk-b"},`+
			`"zai":{"provider":"zai","api_key":"`+secretKey+`","models":[{"id":"other"}],"connected_at":"2026-11-01T00:00:00Z"}}}`, future)
	b.write(filepath.Join(b.eng.Paths.ConfigDir, "permissions.json"), `{"always_allowed_tools":["Grep"]}`, future)
	rep := b.run(Options{Join: true})
	if len(copiesOf(rep, KindConfig)) != 0 || len(b.conflictCopies()) != 0 {
		t.Fatalf("copies on a join that loses nothing: %v %v", rep.Unsent, b.conflictCopies())
	}
	if got := b.read(filepath.Join(b.eng.Paths.ConfigDir, "config.json")); !strings.Contains(got, "https://api.b") || !strings.Contains(got, `"model": "glm"`) {
		t.Errorf("join applied %s", got)
	}
}

// Joining with settings of its own that the arriving ones replace: a copy
// of exactly those files, reported as a join (this computer took the
// settings of the other), not as «changed on both».
func TestEngine_ConfigJoinOwnSettingsKept(t *testing.T) {
	stub := newStub(t)
	key := mustKey(t)
	a, b := newSide(t, stub, key), newSide(t, stub, key)
	a.cfg(`{"api_base":"https://api.a","model":"glm"}`, `{"active":"zai","subscriptions":{"zai":{"provider":"zai","api_key":"`+secretKey+`"}}}`, time.Time{})
	a.write(filepath.Join(a.eng.Paths.ConfigDir, "permissions.json"), `{"always_allowed_tools":["Read"]}`, time.Time{})
	a.run(Options{})

	b.cfg(`{"api_base":"https://api.b","model":"mine"}`, `{"active":"zai","subscriptions":{"zai":{"provider":"zai","api_key":"sk-zai-OTHER-KEY"}}}`, future)
	b.write(filepath.Join(b.eng.Paths.ConfigDir, "permissions.json"), `{"always_allowed_tools":["Read","Bash"]}`, future)
	rep := b.run(Options{Join: true})
	if len(copiesOf(rep, KindConfig)) != 3 || len(copiesOf(rep, KindConfig, reasonJoin)) != 3 {
		t.Fatalf("copies %v; want config, subscriptions and permissions as join copies", rep.Unsent)
	}
	for _, want := range []string{`"model":"mine"`, "sk-zai-OTHER-KEY", `"Bash"`} {
		if _, ok := b.hasCopyContaining(want); !ok {
			t.Errorf("no copy holds %s: %v", want, b.conflictCopies())
		}
	}
}

// hasCopyContaining — a copy whose content contains sub.
func (s *side) hasCopyContaining(sub string) (string, bool) {
	for p, c := range s.conflictCopies() {
		if strings.Contains(c, sub) {
			return p, true
		}
	}
	return "", false
}

// What the person sees at /sync on on a second computer: nothing about
// settings when its own config holds only what is its own by design; the
// join wording (not «changed on both») when settings of its own are
// replaced.
func TestJoin_SettingsMessageTellsTheTruth(t *testing.T) {
	i18n.SetLocale("en")
	for _, c := range []struct {
		name, config string
		wantCopy     bool
	}{
		{"only api_base here", `{"api_base":"https://api.b"}`, false},
		{"a setting of its own", `{"api_base":"https://api.b","model":"mine"}`, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			stub := newStub(t)
			a := newMachine(t, stub, "a")
			a.writeFile("config.json", `{"api_base":"https://api.a","model":"glm"}`)
			a.converse(a.svc.OnFlow(), password, password)
			b := newMachine(t, stub, "b")
			b.writeFile("config.json", c.config)
			text := allText(b.converse(b.svc.OnFlow(), password))
			if strings.Contains(text, "changed both here and on another computer") {
				t.Errorf("«changed on both» on joining:\n%s", text)
			}
			took := strings.Contains(text, "took the settings of your other computer")
			_, err := os.Stat(filepath.Join(b.dir(), "sync-unsent"))
			if took != c.wantCopy || (err == nil) != c.wantCopy {
				t.Errorf("join message %v, copies folder %v, want %v:\n%s", took, err == nil, c.wantCopy, text)
			}
		})
	}
}

// The model the agent picked itself on a computer that only ever ran the
// chat (selected_model_id equal to model_auto) is not this computer's
// setting: joining loses nothing — no copy, no message. A model the person
// chose (it differs from model_auto) is, and is kept.
func TestEngine_ConfigJoinAutoModelIsNotASetting(t *testing.T) {
	for _, c := range []struct {
		name, config string
		wantCopy     bool
	}{
		{"picked by the agent", `{"api_base":"https://api.b","selected_model_id":"glm-5.3-flash","model_auto":"glm-5.3-flash"}`, false},
		{"chosen by the person", `{"api_base":"https://api.b","selected_model_id":"kimi-k2.6","model_auto":"glm-5.3-flash"}`, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			stub := newStub(t)
			key := mustKey(t)
			a, b := newSide(t, stub, key), newSide(t, stub, key)
			a.write(filepath.Join(a.eng.Paths.ConfigDir, "config.json"), `{"api_base":"https://api.a","selected_model_id":"claude-opus-5-5"}`, time.Time{})
			a.run(Options{})
			b.write(filepath.Join(b.eng.Paths.ConfigDir, "config.json"), c.config, future)
			rep := b.run(Options{Join: true})
			if got := len(copiesOf(rep, KindConfig)) > 0 || len(b.conflictCopies()) > 0; got != c.wantCopy {
				t.Errorf("copy %v, want %v: %v", got, c.wantCopy, b.conflictCopies())
			}
		})
	}
}

// A copy is kept only of a settings file that is really replaced: the
// server's settings have no permissions.json, so the local one stays as it
// is (applyConfig leaves a file the blob lacks alone) — no copy of it.
func TestEngine_ConfigJoinNoCopyOfAFileNotReplaced(t *testing.T) {
	stub := newStub(t)
	key := mustKey(t)
	a, b := newSide(t, stub, key), newSide(t, stub, key)
	a.write(filepath.Join(a.eng.Paths.ConfigDir, "config.json"), `{"api_base":"https://api.a","model":"glm"}`, time.Time{})
	a.run(Options{})

	perms := `{"always_allowed_tools":["Bash"]}`
	b.write(filepath.Join(b.eng.Paths.ConfigDir, "config.json"), `{"api_base":"https://api.b"}`, future)
	b.write(filepath.Join(b.eng.Paths.ConfigDir, "permissions.json"), perms, future)
	rep := b.run(Options{Join: true})
	if got := b.read(filepath.Join(b.eng.Paths.ConfigDir, "permissions.json")); got != perms {
		t.Fatalf("permissions.json changed: %s", got)
	}
	if len(copiesOf(rep, KindConfig)) != 0 || len(b.conflictCopies()) != 0 {
		t.Errorf("copies of files that stayed as they were: %v", b.conflictCopies())
	}
}
