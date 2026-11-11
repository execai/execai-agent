package sync

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/crypto"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// Round 6 of the E105 loss review: small things after the fifth review.

func selectedModel(t *testing.T, m *machine) (sel, auto string) {
	t.Helper()
	m.use()
	var c struct {
		Sel  string `json:"selected_model_id"`
		Auto string `json:"model_auto"`
	}
	_ = json.Unmarshal([]byte(m.readFile("config.json")), &c)
	return c.Sel, c.Auto
}

func changeConfig(t *testing.T, m *machine, change func(*config.Config)) {
	t.Helper()
	m.use()
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	change(cfg)
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
}

// S4 of the fifth review: the person chose a model; one computer starts
// without the network and the agent there picks a fallback by itself. The
// fallback stays on that computer; the person's choice stays on the others,
// also when that computer changes another setting. A model the person
// chooses there — another one, or the very fallback — reaches the others.
func TestRound6_AModelTheAgentPickedItselfDoesNotTravel(t *testing.T) {
	stub, a, b := settingsPair(t, subsBase)
	a.use()
	a.writeFile("config.json", `{"api_base":"https://api.a","selected_model_id":"glm-best"}`)
	now(a)
	now(b)
	if sel, _ := selectedModel(t, b); sel != "glm-best" {
		t.Fatalf("setup: B has %q", sel)
	}

	changeConfig(t, b, func(c *config.Config) { c.SetAutoModel("fallback") })
	ver := stub.blob(KindConfig, NameConfig).version
	now(b)
	if got := stub.blob(KindConfig, NameConfig).version; got != ver {
		t.Errorf("the agent's own pick went up as a change (v%d → v%d)", ver, got)
	}
	now(a)
	if sel, _ := selectedModel(t, a); sel != "glm-best" {
		t.Errorf("A's model became %q", sel)
	}

	changeConfig(t, b, func(c *config.Config) { c.ThinkingBudget = 2048 })
	now(b)
	now(a)
	a.use()
	if sel, _ := selectedModel(t, a); sel != "glm-best" || !strings.Contains(a.readFile("config.json"), "2048") {
		t.Errorf("A: %q", a.readFile("config.json"))
	}
	if sel, auto := selectedModel(t, b); sel != "fallback" || auto != "fallback" {
		t.Errorf("B lost its own pick: %q / %q", sel, auto)
	}

	for _, chosen := range []string{"kimi-k3", "fallback"} {
		changeConfig(t, b, func(c *config.Config) { c.ChooseModel(chosen) })
		now(b)
		now(a)
		if sel, _ := selectedModel(t, a); sel != chosen {
			t.Errorf("the model the person chose on B (%s) did not reach A: %q", chosen, sel)
		}
		if chosen == "kimi-k3" {
			changeConfig(t, b, func(c *config.Config) { c.SetAutoModel("fallback") })
			now(b)
			now(a)
			if sel, _ := selectedModel(t, a); sel != "kimi-k3" {
				t.Errorf("after another pick of the agent on B, A has %q", sel)
			}
		}
	}
	assertI1(t, stub, a)
	assertI1(t, stub, b)
}

// The fifth review's E09: what an earlier key left is deleted only at the
// version shown in the question; a version written in between stays, the
// person is told, and the next /sync on asks about it again.
func TestRound6_LeftoverChangedAfterTheQuestionStays(t *testing.T) {
	i18n.SetLocale("en")
	stub := newStub(t)
	a := newMachine(t, stub, "a")
	a.converse(a.svc.OnFlow(), password, password)
	other, _ := crypto.Generate()
	put := func(text string) {
		ct, _ := seal(other, KindMemory, "old.md", []byte(text))
		stub.putDirect(KindMemory, memoryName(other, "old.md"), ct)
	}
	put("under the old key\n")
	now(a)
	ctx := context.Background()
	a.use()
	f := a.svc.OnFlow()
	q := f.Start(ctx)
	if !strings.Contains(q.Text, "a memory file") {
		t.Fatalf("not asked: %+v", q)
	}
	// Nobody kept a key here after a question: the cause said is the
	// earlier key, not a change after a question.
	if strings.Contains(q.Text, "changed after the question") || !strings.Contains(q.Text, "earlier sync key") {
		t.Errorf("the cause: %q", q.Text)
	}
	put("written again under the old key, after the question\n")
	done := f.Answer(ctx, "1")
	if bl := stub.blob(KindMemory, memoryName(other, "old.md")); bl == nil || bl.deleted {
		t.Error("a version written after the question was deleted")
	}
	if !strings.Contains(done.Text, "changed after the question") {
		t.Errorf("not told: %q", done.Text)
	}
	if again := a.converse(a.svc.OnFlow()); again[0].Done() || !strings.Contains(again[0].Text, "a memory file") {
		t.Errorf("not asked again: %+v", again[0])
	}
}

// The fifth review's note on the older snapshot line: the numbers agree
// with their words for 1, 2 and 5 files, in English and Russian.
func TestRound6_OlderSnapshotLineCounts(t *testing.T) {
	defer i18n.SetLocale("en")
	root := "/work/app/.execai"
	for _, lang := range []string{"en", "ru"} {
		i18n.SetLocale(lang)
		for _, n := range []int{1, 2, 5} {
			for _, m := range []int{1, 2, 5} {
				var r Report
				for i := 0; i < n; i++ {
					r.Unsent = append(r.Unsent, unsentCopy{kind: KindProjectMemory, root: root, path: root + "/b" + itoa(i) + ".md.local-1", why: reasonOlderBack})
				}
				for i := 0; i < m; i++ {
					r.Unsent = append(r.Unsent, unsentCopy{kind: KindProjectMemory, root: root, path: root + "/g" + itoa(i) + ".md.local-1", why: reasonOlderGone})
				}
				line := strings.Join(keepNotices(r, Paths{ConfigDir: "/cfg"}), "\n")
				bad := []string{"1 of its files are", "1 of your files are", "%!", "files: 1)"}
				if lang == "ru" {
					bad = []string{"%!", "файлов: 1)"}
				}
				for _, b := range bad {
					if strings.Contains(line, b) {
						t.Errorf("%s %d/%d: %q", lang, n, m, line)
					}
				}
				t.Logf("%s %d/%d: %s", lang, n, m, line)
			}
		}
	}
}

// ===== the fifth probe =====

// y3: a provider key added, sent, then removed. The file that comes to the
// other computers is the earlier version of subscriptions.json — not a
// return to keep the newer one for good: the removed key must not stay in
// sync-unsent on every other computer with advice to copy it back. The
// version that was here goes to the history for 30 days, and the person is
// told in one line.
func TestRound6_ARemovedProviderKeyIsNotKeptForGood(t *testing.T) {
	stub, a, b := settingsPair(t, subsBase)
	c := newMachine(t, stub, "c")
	c.converse(c.svc.OnFlow(), password)
	withKey := strings.Replace(subsBase, `"zai":{`, `"kimi":{"provider":"kimi","api_key":"k-kimi-secret"},"zai":{`, 1)
	a.use()
	a.writeFile("subscriptions.json", withKey)
	for _, m := range []*machine{a, b, c} {
		now(m)
	}
	for _, m := range []*machine{b, c} {
		m.use()
		if !strings.Contains(m.readFile("subscriptions.json"), "k-kimi-secret") {
			t.Fatalf("setup: %s has no key", m.name)
		}
	}
	a.use()
	a.writeFile("subscriptions.json", subsBase)
	outs := map[string]string{}
	for i := 0; i < 2; i++ {
		for _, m := range []*machine{a, b, c} {
			outs[m.name] += now(m)
		}
	}
	for _, m := range []*machine{a, b, c} {
		m.use()
		if strings.Contains(m.readFile("subscriptions.json"), "k-kimi-secret") {
			t.Errorf("%s: the removed key is still in the settings", m.name)
		}
		if anyHolds(m.conflictFiles(), "k-kimi-secret") {
			t.Errorf("%s: the removed key is kept for good: %v", m.name, m.conflictFiles())
		}
	}
	for _, m := range []*machine{b, c} {
		if !anyHolds(historyOf(m), "k-kimi-secret") {
			t.Errorf("%s: the version that was here is not in the history", m.name)
		}
		if o := outs[m.name]; !strings.Contains(o, "The list of provider keys went back to an earlier version that came from the server") ||
			strings.Contains(o, "copy it back") {
			t.Errorf("%s said:\n%s", m.name, o)
		}
	}
}

// The other way: an older agent puts permissions.json back to an earlier
// version (a prohibition undone). It is said, and the version that was here
// is in the history.
func TestRound6_PermissionsBackToAnEarlierVersionAreSaid(t *testing.T) {
	stub, a, b := settingsPair(t, subsBase)
	a.use()
	a.writeFile("permissions.json", `{"always_allowed_tools":["Read","Bash"]}`)
	now(a)
	now(b)
	a.writeFile("permissions.json", `{"always_allowed_tools":["Read"]}`) // Bash prohibited again
	now(a)
	now(b)
	a.use()
	key, _ := crypto.LoadIdentity(a.dir())
	bl := stub.blob(KindConfig, NameConfig)
	_, _, data, _ := unseal(key, bl.content)
	files, _ := unpackFiles(data)
	files["permissions.json"] = []byte(`{"always_allowed_tools":["Read","Bash"]}`)
	ct, _ := seal(key, KindConfig, NameConfig, mustPack(files))
	stub.putDirect(KindConfig, NameConfig, ct)
	out := now(b)
	b.use()
	if !strings.Contains(b.readFile("permissions.json"), "Bash") ||
		!strings.Contains(out, "Permissions and prohibitions on this computer went back to an earlier version") ||
		!strings.Contains(out, "Check that no prohibition you need was lifted") || !strings.Contains(out, "permissions.json") {
		t.Errorf("B: %q\n%s", b.readFile("permissions.json"), out)
	}
	if len(b.conflictFiles()) != 0 || !anyHolds(historyOf(b), `["Read"]`) {
		t.Errorf("copies %v; history %v", b.conflictFiles(), historyOf(b))
	}
}

// d, choice 2: the settings this computer has go up over the old ones in
// the first decision, as projects do; nothing about them is asked again.
func TestRound6_KeepingThisKeyReplacesTheSettingsAtOnce(t *testing.T) {
	stub, a, c, _, _, _ := overwriteSetup(t)
	a.use()
	a.writeFile("permissions.json", `{"always_allowed_tools":["Read"]}`)
	now(a)
	c.use()
	c.writeFile("permissions.json", `{"always_allowed_tools":["Grep"]}`)
	c.converse(c.svc.OnFlow(), password, "2")
	c.use()
	key, _ := crypto.LoadIdentity(c.dir())
	bl := stub.blob(KindConfig, NameConfig)
	_, _, data, err := unseal(key, bl.content)
	if err != nil {
		t.Fatalf("the settings stayed under the earlier key: %v", err)
	}
	files, _ := unpackFiles(data)
	if !strings.Contains(string(files["permissions.json"]), "Grep") {
		t.Errorf("the server's settings: %q", files)
	}
	if again := c.converse(c.svc.OnFlow()); !again[0].Done() || strings.Contains(again[0].Text, "earlier sync key") {
		t.Errorf("asked again: %+v", again[0])
	}
}

// s8b: all notes deleted at once — their lines go from the index too.
func TestRound6_AllNotesDeletedTakeTheirLines(t *testing.T) {
	_, a, b := memPair(t)
	a.use()
	for _, n := range []string{"role.md", "fb.md"} {
		rm(t, a, n)
	}
	now(a)
	now(b)
	now(a)
	for _, m := range []*machine{a, b} {
		m.use()
		if got := m.readFile("memory/MEMORY.md"); strings.Contains(got, ".md)") {
			t.Errorf("%s: lines of deleted notes: %q", m.name, got)
		}
	}
}

// A fresh account with notes and no index: after the first full exchange
// the index is made with their lines, on every computer.
func TestRound6_NotesWithNoIndexGetOne(t *testing.T) {
	i18n.SetLocale("en")
	stub := newStub(t)
	a := newMachine(t, stub, "a")
	a.converse(a.svc.OnFlow(), password, password)
	b := newMachine(t, stub, "b")
	b.converse(b.svc.OnFlow(), password)
	a.use()
	a.writeFile("memory/role.md", "---\ndescription: who I am\n---\nGo engineer\n")
	now(a)
	now(a)
	now(b)
	for _, m := range []*machine{a, b} {
		m.use()
		if got := m.readFile("memory/MEMORY.md"); !strings.Contains(got, "](role.md) — who I am") {
			t.Errorf("%s: %q", m.name, got)
		}
	}
}
