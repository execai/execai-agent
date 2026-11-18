package chat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/config"
)

// offerHome isolates HOME and the config folder for one test.
func offerHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("CODEX_HOME", "")
	return home
}

func offerWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func importedFiles(t *testing.T, home string) []string {
	t.Helper()
	entries, _ := os.ReadDir(filepath.Join(home, ".config", "execai", "memory"))
	var out []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "imported_") {
			out = append(out, e.Name())
		}
	}
	return out
}

// Declining copies nothing, and the question is never asked again here.
func TestImportOffer_DeclineIsRemembered(t *testing.T) {
	home := offerHome(t)
	proj := filepath.Join(home, "work", "app")
	offerWrite(t, filepath.Join(proj, "CLAUDE.md"), "prod on R5")
	offerWrite(t, filepath.Join(proj, ".cursorrules"), "answer briefly")

	m := &tuiModel{}
	m.maybeOfferImportIn(proj)
	if !m.asking || m.askPending == nil || len(m.askOptions) != 2 {
		t.Fatalf("no offer: asking=%v options=%v", m.asking, m.askOptions)
	}
	for _, want := range []string{"CLAUDE.md", ".cursorrules", "Claude Code", "Cursor"} {
		if !strings.Contains(m.askQuestion, want) {
			t.Errorf("question does not mention %q:\n%s", want, m.askQuestion)
		}
	}

	if out := m.askPending(m.askOptions[1].Label); out == "" {
		t.Error("no reply after declining")
	}
	if got := importedFiles(t, home); len(got) != 0 {
		t.Errorf("declined, but memory got %v", got)
	}
	if st := config.LoadProjectState(proj); st.ImportOffer != config.ImportOfferDeclined {
		t.Errorf("answer not remembered: %+v", st)
	}

	again := &tuiModel{}
	again.maybeOfferImportIn(proj)
	if again.asking {
		t.Error("asked a second time after the user declined")
	}
}

// Esc is an answer too: the offer is made once.
func TestImportOffer_DismissCountsAsDecline(t *testing.T) {
	home := offerHome(t)
	proj := filepath.Join(home, "app")
	offerWrite(t, filepath.Join(proj, "AGENTS.md"), "make test")

	m := &tuiModel{}
	m.maybeOfferImportIn(proj)
	if !m.asking {
		t.Fatal("no offer")
	}
	m.askPending("")
	if got := importedFiles(t, home); len(got) != 0 {
		t.Errorf("dismissed, but memory got %v", got)
	}
	again := &tuiModel{}
	again.maybeOfferImportIn(proj)
	if again.asking {
		t.Error("asked again after Esc")
	}
}

// Accepting imports the files and the offer is not repeated.
func TestImportOffer_AcceptImports(t *testing.T) {
	home := offerHome(t)
	proj := filepath.Join(home, "work", "app")
	offerWrite(t, filepath.Join(proj, "CLAUDE.md"), "prod on R5")
	offerWrite(t, filepath.Join(home, ".codex", "AGENTS.md"), "global codex rules")

	m := &tuiModel{}
	m.maybeOfferImportIn(proj)
	if !m.asking {
		t.Fatal("no offer")
	}
	if !strings.Contains(m.askQuestion, "Codex CLI") {
		t.Errorf("user-level Codex instructions not offered:\n%s", m.askQuestion)
	}
	m.askPending(m.askOptions[0].Label)
	if got := importedFiles(t, home); len(got) != 2 {
		t.Errorf("imported %v, want 2 files", got)
	}
	if st := config.LoadProjectState(proj); st.ImportOffer != config.ImportOfferAccepted {
		t.Errorf("answer not remembered: %+v", st)
	}
	again := &tuiModel{}
	again.maybeOfferImportIn(proj)
	if again.asking {
		t.Error("asked again after importing")
	}
}

// No offer where execai already has project memory, where only user-level
// files exist (that would ask in every folder), or in the home folder.
func TestImportOffer_NotOffered(t *testing.T) {
	home := offerHome(t)

	withExecai := filepath.Join(home, "a")
	offerWrite(t, filepath.Join(withExecai, "EXECAI.md"), "our memory")
	offerWrite(t, filepath.Join(withExecai, "CLAUDE.md"), "theirs")

	globalOnly := filepath.Join(home, "b")
	if err := os.MkdirAll(globalOnly, 0o755); err != nil {
		t.Fatal(err)
	}
	offerWrite(t, filepath.Join(home, ".claude", "CLAUDE.md"), "global prefs")
	offerWrite(t, filepath.Join(home, "CLAUDE.md"), "home file")

	for _, dir := range []string{withExecai, globalOnly, home} {
		m := &tuiModel{}
		m.maybeOfferImportIn(dir)
		if m.asking {
			t.Errorf("%s: offered, but must not be:\n%s", dir, m.askQuestion)
		}
		if st := config.LoadProjectState(dir); st.ImportOffer != "" {
			t.Errorf("%s: an answer was recorded without asking: %+v", dir, st)
		}
	}
}
