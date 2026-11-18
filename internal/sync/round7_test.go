package sync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/crypto"
)

// Round 7 of the E105 loss review: the lines about settings, a restored
// file, and the guard of the agreed model.

// A settings file back to an earlier version is said only where it
// matters to the person — permissions and prohibitions, provider keys;
// config.json switched there and back on another computer is replaced
// silently, the version here going to the history all the same.
func TestRound7_SettingsBackToAnEarlierVersionSaidWhereItMatters(t *testing.T) {
	_, a, b := settingsPair(t, subsBase)
	a.use()
	a.writeFile("config.json", `{"api_base":"https://api.a","selected_model_id":"glm-best"}`)
	a.writeFile("permissions.json", `{"always_allowed_tools":["Read"]}`)
	now(a)
	now(b)
	for _, tc := range []struct {
		file, newer, line string
	}{
		{"config.json", `{"api_base":"https://api.b","selected_model_id":"kimi-k3"}`, ""},
		{"permissions.json", `{"always_allowed_tools":["Read","Bash"]}`, "Permissions and prohibitions on this computer went back to an earlier version"},
		{"subscriptions.json", strings.Replace(subsBase, `"zai":{`, `"kimi":{"provider":"kimi","api_key":"k-kimi-x"},"zai":{`, 1), "The list of provider keys went back to an earlier version"},
	} {
		t.Run(tc.file, func(t *testing.T) {
			b.use()
			earlier := b.readFile(tc.file)
			b.writeFile(tc.file, tc.newer)
			now(b)
			now(a)
			b.use()
			b.writeFile(tc.file, earlier) // and back again on B
			now(b)
			out := now(a)
			a.use()
			if a.readFile(tc.file) != earlier && !(tc.file == "config.json" && strings.Contains(a.readFile(tc.file), "glm-best")) {
				t.Errorf("A's %s: %q", tc.file, a.readFile(tc.file))
			}
			said := strings.Contains(out, "earlier version")
			if tc.line == "" && said || tc.line != "" && !strings.Contains(out, tc.line) {
				t.Errorf("%s: said %v:\n%s", tc.file, said, out)
			}
			if len(a.conflictFiles()) != 0 {
				t.Errorf("a copy kept for good: %v", a.conflictFiles())
			}
			if !anyHolds(historyOf(a), strings.TrimSpace(tc.newer)[2:12]) {
				t.Errorf("the version that was here is not in the history: %v", historyOf(a))
			}
		})
	}
}

// Row 4 for subscriptions.json: the copy kept may hold a key removed on
// another computer; the line says so.
func TestRound7_KeysCopyWarnsOfRemovedKeys(t *testing.T) {
	_, a, b := settingsPair(t, subsBase)
	b.use()
	b.writeFile("subscriptions.json", strings.Replace(subsBase, "k-zai-test", "k-zai-from-B", 1))
	now(b)
	a.use()
	a.writeFile("subscriptions.json", strings.Replace(subsBase, "k-zai-test", "k-zai-from-A", 1))
	out := now(a)
	if !strings.Contains(out, "It may hold keys already removed on another computer") || !strings.Contains(out, "subscriptions.local-") {
		t.Errorf("A said:\n%s", out)
	}
}

// Files missing here and taken from the server are named: one by its
// path, several by their number and their folder.
func TestRound7_RestoredFilesAreNamed(t *testing.T) {
	_, _, b, _, pB := lossPair(t)
	for _, f := range []string{"EXECAI.md", "notes.md"} {
		if err := os.Remove(filepath.Join(ProjectDir(pB), f)); err != nil {
			t.Fatal(err)
		}
	}
	out := now(b)
	if !strings.Contains(out, "were missing here and were taken from the server (files: 2), in "+ProjectDir(pB)) {
		t.Errorf("B said:\n%s", out)
	}
}

// The model of the agreed settings stands in for the agent's own pick only
// while those settings are the base sync.json holds (the fifth review's
// G08): after an older agent agreed newer settings — the person chose
// another model meanwhile — the earlier agreed model never goes up over
// the person's newer choice.
func TestRound7_AnOutdatedAgreedModelIsNotSent(t *testing.T) {
	stub, a, b := settingsPair(t, subsBase)
	a.use()
	a.writeFile("config.json", `{"api_base":"https://api.a","selected_model_id":"glm-best"}`)
	now(a)
	now(b)
	changeConfig(t, a, func(c *config.Config) { c.ChooseModel("kimi-k3") })
	now(a)
	// An older agent on B agrees the new settings: it writes the files and
	// sync.json, and does not know sync-bases.json.
	b.use()
	key, _ := crypto.LoadIdentity(b.dir())
	bl := stub.blob(KindConfig, NameConfig)
	_, _, data, err := unseal(key, bl.content)
	if err != nil {
		t.Fatal(err)
	}
	files, _ := unpackFiles(data)
	for name, content := range files {
		if name == "config.json" {
			content = keepAPIBase([]byte(`{"api_base":"https://api.b"}`), content)
		}
		b.writeFile(name, string(content))
	}
	st, _ := LoadState(b.svc.Paths)
	st.Blobs[blobKey(KindConfig, NameConfig)] = BlobState{Path: NameConfig, Version: bl.version, Hash: hashOf(data)}
	if err := st.Save(b.svc.Paths); err != nil {
		t.Fatal(err)
	}
	// Then this agent on B picks a model by itself.
	changeConfig(t, b, func(c *config.Config) { c.SetAutoModel("fallback") })
	now(b)
	now(a)
	if sel, _ := selectedModel(t, a); sel == "glm-best" {
		t.Errorf("the outdated agreed model went over the person's newer choice")
	}
}
