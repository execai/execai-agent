package sync

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/crypto"
)

// side — one machine for engine tests: its own directory and state, a
// shared key.
type side struct {
	t   *testing.T
	eng *Engine
	st  *State
}

func newSide(t *testing.T, stub *stubServer, id *crypto.Identity) *side {
	t.Helper()
	return &side{t: t, st: &State{}, eng: &Engine{
		Client: NewClient(stub.URL, func() string { return "tok-x" }),
		ID:     id,
		Paths:  Paths{ConfigDir: t.TempDir()},
	}}
}

func mustKey(t *testing.T) *crypto.Identity {
	t.Helper()
	id, err := crypto.Generate()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (s *side) run(opt Options) Report {
	s.t.Helper()
	rep, err := s.eng.Run(context.Background(), s.st, opt)
	if err != nil {
		s.t.Fatalf("run: %v", err)
	}
	return rep
}

func (s *side) mem(rel string) string { return filepath.Join(s.eng.Paths.MemoryDir(), rel) }

// write puts a file with an explicit mtime (zero — leave as written).
func (s *side) write(file, content string, mtime time.Time) {
	s.t.Helper()
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		s.t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		s.t.Fatal(err)
	}
	if !mtime.IsZero() {
		if err := os.Chtimes(file, mtime, mtime); err != nil {
			s.t.Fatal(err)
		}
	}
}

func (s *side) read(file string) string {
	s.t.Helper()
	b, err := os.ReadFile(file)
	if err != nil {
		return "<missing>"
	}
	return string(b)
}

var (
	past   = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	future = time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
)

func TestEngine_PushNewThenPullNew(t *testing.T) {
	stub := newStub(t)
	key := mustKey(t)
	a, b := newSide(t, stub, key), newSide(t, stub, key)

	a.write(a.mem("MEMORY.md"), "- [Role](user_role.md) — who I am\n- [Deep](notes/deep.md)\n", time.Time{})
	a.write(a.mem("user_role.md"), "Go engineer\n", time.Time{})
	a.write(a.mem("notes/deep.md"), "nested\n", time.Time{})
	rep := a.run(Options{})
	if rep.Pushed != 3 || rep.Memory != 3 {
		t.Fatalf("push: %+v", rep)
	}
	// The server never sees file names.
	for _, c := range stub.allContent() {
		if strings.Contains(string(c), "user_role") {
			t.Fatal("plaintext on the server")
		}
	}
	stub.mu.Lock()
	for k := range stub.blobs {
		if strings.Contains(k, "user_role") || strings.Contains(k, "MEMORY") {
			t.Errorf("file name leaked into the blob name: %s", k)
		}
	}
	stub.mu.Unlock()

	rep = b.run(Options{})
	if rep.Pulled != 3 {
		t.Fatalf("pull: %+v", rep)
	}
	if got := b.read(b.mem("user_role.md")); got != "Go engineer\n" {
		t.Fatalf("pulled %q", got)
	}
	if got := b.read(b.mem("notes/deep.md")); got != "nested\n" {
		t.Fatalf("pulled nested %q", got)
	}
	// Nothing new: a second run sends and takes nothing.
	if rep := b.run(Options{}); rep.Pulled+rep.Pushed != 0 {
		t.Fatalf("idle run did something: %+v", rep)
	}
}

// Both edit. A sends first; whatever the clocks say, the version that
// reached the server stays and B's is kept as a copy (threeway.go, row 4) —
// the time of a file decides nothing.
func TestEngine_ConflictTheServersVersionStays(t *testing.T) {
	for _, bTime := range []time.Time{past, future} {
		stub := newStub(t)
		key := mustKey(t)
		a, b := newSide(t, stub, key), newSide(t, stub, key)
		a.write(a.mem("MEMORY.md"), "- [Fact](fact.md)\n", time.Time{})
		a.write(a.mem("fact.md"), "v1\n", time.Time{})
		a.run(Options{})
		b.run(Options{})
		a.write(a.mem("fact.md"), "from A\n", future.Add(-bTime.Sub(past)))
		a.run(Options{PushOnly: true})
		b.write(b.mem("fact.md"), "from B\n", bTime)
		if rep := b.run(Options{PushOnly: true}); rep.Pushed != 0 || b.read(b.mem("fact.md")) != "from B\n" {
			t.Fatalf("a refused send-only exchange took or sent something: %+v", rep)
		}
		rep := b.run(Options{}) // row 4
		if rep.Conflicts == 0 || rep.Pushed != 0 {
			t.Fatalf("expected a conflict settled for the server: %+v", rep)
		}
		if got := b.read(b.mem("fact.md")); got != "from A\n" {
			t.Fatalf("B has %q, want the server's", got)
		}
		if _, ok := b.hasCopy("from B\n"); !ok {
			t.Fatalf("B's edit is not kept: %v", b.conflictCopies())
		}
		a.run(Options{})
		if got := a.read(a.mem("fact.md")); got != "from A\n" {
			t.Fatalf("A has %q", got)
		}
	}
}

func TestEngine_IndexMergesLines(t *testing.T) {
	stub := newStub(t)
	key := mustKey(t)
	a, b := newSide(t, stub, key), newSide(t, stub, key)
	a.write(a.mem("one.md"), "1\n", time.Time{})
	a.write(a.mem("MEMORY.md"), "- [One](one.md)\n", time.Time{})
	a.run(Options{})
	b.run(Options{})

	a.write(a.mem("a.md"), "A\n", time.Time{})
	a.write(a.mem("MEMORY.md"), "- [One](one.md)\n- [From A](a.md)\n", time.Time{})
	a.run(Options{})
	b.write(b.mem("b.md"), "B\n", time.Time{})
	b.write(b.mem("MEMORY.md"), "- [One](one.md)\n- [From B](b.md)\n", time.Time{})
	b.run(Options{})
	a.run(Options{})
	for _, s := range []*side{a, b} {
		got := s.read(s.mem("MEMORY.md"))
		if !strings.Contains(got, "[From A](a.md)") || !strings.Contains(got, "[From B](b.md)") || strings.Count(got, "[One](one.md)") != 1 {
			t.Fatalf("index not merged: %q", got)
		}
	}
}

func TestEngine_TombstonesBothWays(t *testing.T) {
	stub := newStub(t)
	key := mustKey(t)
	a, b := newSide(t, stub, key), newSide(t, stub, key)
	a.write(a.mem("gone.md"), "x\n", time.Time{})
	a.write(a.mem("keep.md"), "y\n", time.Time{})
	a.run(Options{})
	b.run(Options{})

	// Deleted on A → tombstone → deleted on B.
	os.Remove(a.mem("gone.md"))
	if rep := a.run(Options{PushOnly: true}); rep.Deleted != 1 {
		t.Fatalf("no tombstone sent: %+v", rep)
	}
	if bl := stub.blob(KindMemory, memoryName(key, "gone.md")); bl == nil || !bl.deleted {
		t.Fatal("server has no tombstone")
	}
	if rep := b.run(Options{}); rep.Removed != 1 {
		t.Fatalf("tombstone not applied: %+v", rep)
	}
	if b.read(b.mem("gone.md")) != "<missing>" {
		t.Fatal("file still on B")
	}

	// Deleted on B, but edited on A later → the edit wins, the file returns.
	os.Remove(b.mem("keep.md"))
	b.run(Options{})
	a.write(a.mem("keep.md"), "edited later\n", future)
	a.run(Options{})
	b.run(Options{})
	if got := b.read(b.mem("keep.md")); got != "edited later\n" {
		t.Fatalf("edit after delete lost: %q", got)
	}

	// A missing memory directory is not «delete everything».
	os.RemoveAll(a.eng.Paths.MemoryDir())
	if rep := a.run(Options{}); rep.Deleted != 0 {
		t.Fatalf("a missing directory deleted files: %+v", rep)
	}
}

// The whole config travels as one blob: config.json, subscriptions.json
// with the keys, permissions.json. Only api_base stays per machine.
func TestEngine_ConfigBundle(t *testing.T) {
	stub := newStub(t)
	key := mustKey(t)
	a, b := newSide(t, stub, key), newSide(t, stub, key)
	const zai = "sk-zai-SECRET-1"
	a.write(a.eng.Paths.ConfigFile(), `{"api_base":"https://a.example","selected_model_id":"glm-5.3","classic_tui":true}`, time.Time{})
	a.write(a.eng.Paths.SubscriptionsFile(), `{"subscriptions":{"zai":{"provider":"zai","api_key":"`+zai+`"}},"active":"zai"}`, time.Time{})
	a.write(a.eng.Paths.PermissionsFile(), `{"always_allowed_tools":["Read"]}`, time.Time{})
	b.write(b.eng.Paths.ConfigFile(), `{"api_base":"https://b.example","max_iterations":7}`, past)
	a.run(Options{})

	bl := stub.blob(KindConfig, NameConfig)
	if bl == nil || len(stub.blobs) != 1 {
		t.Fatalf("one config blob expected, have %d", len(stub.blobs))
	}
	if strings.Contains(string(bl.content), zai) {
		t.Fatal("the key is readable in the ciphertext")
	}
	_, _, data, err := unseal(key, bl.content)
	if err != nil {
		t.Fatal(err)
	}
	files, err := unpackFiles(data)
	if err != nil || !strings.Contains(string(files["subscriptions.json"]), zai) ||
		!strings.Contains(string(files["permissions.json"]), "Read") || !strings.Contains(string(files["config.json"]), "classic_tui") {
		t.Fatalf("config blob content: %v %q", err, files)
	}

	b.run(Options{Join: true})
	var got map[string]any
	_ = json.Unmarshal([]byte(b.read(b.eng.Paths.ConfigFile())), &got)
	if got["api_base"] != "https://b.example" || got["selected_model_id"] != "glm-5.3" || got["classic_tui"] != true {
		t.Fatalf("config on B: %v", got)
	}
	if _, ok := got["max_iterations"]; ok {
		t.Fatalf("the config is not replaced as a whole: %v", got)
	}
	if !strings.Contains(b.read(b.eng.Paths.SubscriptionsFile()), zai) || !strings.Contains(b.read(b.eng.Paths.PermissionsFile()), "Read") {
		t.Fatal("sources or permissions did not arrive")
	}
	// B's own api_base does not make B push the config back.
	if rep := b.run(Options{}); rep.Pushed != 0 {
		t.Fatalf("B pushed after applying: %+v", rep)
	}

	// Without a local api_base the arriving one is taken; a broken file in
	// a blob is skipped, not written.
	c := newSide(t, stub, key)
	c.run(Options{})
	if !strings.Contains(c.read(c.eng.Paths.ConfigFile()), "https://a.example") {
		t.Fatalf("config on C: %s", c.read(c.eng.Paths.ConfigFile()))
	}
	broken, _ := packFiles(map[string][]byte{"config.json": []byte("{not json"), "permissions.json": []byte(`{"always_allowed_tools":["Bash"]}`)})
	ct, _ := seal(key, KindConfig, NameConfig, broken)
	if _, err := a.eng.Client.Put(context.Background(), KindConfig, NameConfig, ct, stub.blob(KindConfig, NameConfig).version); err != nil {
		t.Fatal(err)
	}
	c.run(Options{})
	if !strings.Contains(c.read(c.eng.Paths.ConfigFile()), "glm-5.3") || !strings.Contains(c.read(c.eng.Paths.PermissionsFile()), "Bash") {
		t.Fatal("broken config.json written or valid permissions skipped")
	}
}

// The notifications setting belongs to the machine: it never reaches the
// server, an arriving config leaves this machine's value alone (or its
// absence), and changing only it sends nothing.
func TestEngine_ConfigNotificationsStayLocal(t *testing.T) {
	stub := newStub(t)
	key := mustKey(t)
	a, b := newSide(t, stub, key), newSide(t, stub, key)
	a.write(a.eng.Paths.ConfigFile(), `{"api_base":"https://a.example","selected_model_id":"glm-5.3","notifications":"off"}`, time.Time{})
	b.write(b.eng.Paths.ConfigFile(), `{"api_base":"https://b.example","notifications":"bell"}`, past)
	a.run(Options{})

	bl := stub.blob(KindConfig, NameConfig)
	if bl == nil {
		t.Fatal("no config blob")
	}
	_, _, data, err := unseal(key, bl.content)
	if err != nil {
		t.Fatal(err)
	}
	files, _ := unpackFiles(data)
	if strings.Contains(string(files["config.json"]), "notifications") || !strings.Contains(string(files["config.json"]), "glm-5.3") {
		t.Fatalf("config.json on the server: %s", files["config.json"])
	}

	b.run(Options{Join: true})
	var got map[string]any
	_ = json.Unmarshal([]byte(b.read(b.eng.Paths.ConfigFile())), &got)
	if got["notifications"] != "bell" || got["selected_model_id"] != "glm-5.3" {
		t.Fatalf("config on B: %v", got)
	}
	if rep := b.run(Options{}); rep.Pushed != 0 {
		t.Fatalf("B pushed after applying: %+v", rep)
	}

	// Only the setting changed on A: nothing to send.
	a.write(a.eng.Paths.ConfigFile(), `{"api_base":"https://a.example","selected_model_id":"glm-5.3","notifications":"system"}`, future)
	if rep := a.run(Options{}); rep.Pushed != 0 {
		t.Fatalf("a change of the machine's own setting was pushed: %+v", rep)
	}

	// A machine without the setting stays without it, whatever arrives.
	c := newSide(t, stub, key)
	c.write(c.eng.Paths.ConfigFile(), `{"api_base":"https://c.example"}`, past)
	forged, _ := packFiles(map[string][]byte{"config.json": []byte(`{"api_base":"https://x","selected_model_id":"m2","notifications":"off"}`)})
	ct, _ := seal(key, KindConfig, NameConfig, forged)
	if _, err := a.eng.Client.Put(context.Background(), KindConfig, NameConfig, ct, stub.blob(KindConfig, NameConfig).version); err != nil {
		t.Fatal(err)
	}
	c.run(Options{Join: true})
	got = nil
	_ = json.Unmarshal([]byte(c.read(c.eng.Paths.ConfigFile())), &got)
	if _, ok := got["notifications"]; ok || got["selected_model_id"] != "m2" || got["api_base"] != "https://c.example" {
		t.Fatalf("config on C: %v", got)
	}
}

// Joining keeps the sources only the new machine had; the active one comes
// from the global config.
func TestEngine_JoinKeepsOwnSources(t *testing.T) {
	stub := newStub(t)
	key := mustKey(t)
	a, b := newSide(t, stub, key), newSide(t, stub, key)
	a.write(a.eng.Paths.SubscriptionsFile(), `{"subscriptions":{"zai":{"provider":"zai","api_key":"k-a"}},"active":"zai"}`, time.Time{})
	a.run(Options{})
	b.write(b.eng.Paths.SubscriptionsFile(), `{"subscriptions":{"zai":{"provider":"zai","api_key":"k-old"},"kimi-api":{"provider":"kimi-api","api_key":"k-b"}},"active":"kimi-api"}`, time.Time{})
	b.run(Options{Join: true})
	var st struct {
		Subscriptions map[string]struct {
			APIKey string `json:"api_key"`
		} `json:"subscriptions"`
		Active string `json:"active"`
	}
	_ = json.Unmarshal([]byte(b.read(b.eng.Paths.SubscriptionsFile())), &st)
	if st.Active != "zai" || st.Subscriptions["zai"].APIKey != "k-a" || st.Subscriptions["kimi-api"].APIKey != "k-b" {
		t.Fatalf("joined store: %+v", st)
	}
	// The union goes back to the server, so A gets B's source too.
	a.run(Options{})
	if !strings.Contains(a.read(a.eng.Paths.SubscriptionsFile()), "k-b") {
		t.Fatal("B's own source did not reach A")
	}
}

func TestEngine_ForeignKeyIsNeverOverwritten(t *testing.T) {
	stub := newStub(t)
	a := newSide(t, stub, mustKey(t))
	b := newSide(t, stub, mustKey(t)) // another key
	a.write(a.eng.Paths.PermissionsFile(), `{"always_allowed_tools":["Read"]}`, time.Time{})
	a.run(Options{})
	before := stub.blob(KindConfig, NameConfig).version

	b.write(b.eng.Paths.PermissionsFile(), `{"always_allowed_tools":["Bash"]}`, future)
	rep := b.run(Options{})
	if rep.Unreadable == 0 {
		t.Fatalf("foreign blob not reported: %+v", rep)
	}
	if v := stub.blob(KindConfig, NameConfig).version; v != before {
		t.Fatal("a blob this key cannot read was overwritten")
	}
}

func TestEngine_ProjectMemoryNeedsBinding(t *testing.T) {
	stub := newStub(t)
	key := mustKey(t)
	a, b := newSide(t, stub, key), newSide(t, stub, key)
	projA, projB := t.TempDir(), t.TempDir()
	a.write(ProjectMemoryFile(projA), "project facts\n", time.Time{})
	a.run(Options{Projects: map[string]string{"ws-1": projA}})
	if stub.blob(KindProjectMemory, "ws-1") == nil {
		t.Fatal("project memory not sent")
	}
	// Not bound on B: nothing lands, and nothing is forgotten either.
	b.run(Options{Projects: map[string]string{}})
	b.run(Options{Projects: map[string]string{"ws-1": projB}})
	if got := b.read(ProjectMemoryFile(projB)); got != "project facts\n" {
		t.Fatalf("project memory on B: %q", got)
	}
}

func TestEngine_TooLargeAndQuota(t *testing.T) {
	stub := newStub(t)
	a := newSide(t, stub, mustKey(t))
	a.write(a.mem("big.md"), strings.Repeat("x", MaxBlobBytes+1), time.Time{})
	a.write(a.mem("a.md"), "a", time.Time{})
	a.write(a.mem("b.md"), "b", time.Time{})
	stub.quotaBlobs = 1
	rep := a.run(Options{})
	if len(rep.Skipped) != 1 || rep.Skipped[0].Reason != SkipTooBig || !rep.QuotaFull || rep.Pushed != 1 {
		t.Fatalf("limits: %+v", rep)
	}
}

func TestEngine_ServerWipeRepushes(t *testing.T) {
	stub := newStub(t)
	a := newSide(t, stub, mustKey(t))
	a.write(a.mem("x.md"), "x", time.Time{})
	a.run(Options{})
	stub.mu.Lock()
	stub.blobs = map[string]*stubBlob{}
	stub.mu.Unlock()
	if rep := a.run(Options{}); rep.Pushed != 1 {
		t.Fatalf("wiped blob not sent again: %+v", rep)
	}
}

func TestEnvelope_BoundToItsName(t *testing.T) {
	key := mustKey(t)
	e := &Engine{ID: key}
	ct, _ := seal(key, KindMemory, "a.md", []byte("A"))
	if _, _, err := e.open(ct, KindMemory, memoryName(key, "b.md")); !errors.Is(err, errEnvelope) {
		t.Fatalf("swapped blob accepted: %v", err)
	}
	if _, _, err := e.open(ct, KindConfig, NameConfig); !errors.Is(err, errEnvelope) {
		t.Fatalf("wrong kind accepted: %v", err)
	}
	evil, _ := seal(key, KindMemory, "../../.bashrc", []byte("x"))
	if _, _, err := e.open(evil, KindMemory, memoryName(key, "../../.bashrc")); err == nil {
		t.Fatal("path escaping the memory directory accepted")
	}
}
