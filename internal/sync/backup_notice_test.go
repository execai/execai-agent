package sync

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// The line about a conflict copy says whose version stayed and where the
// other one is — not «the newer version wins»: the agent compares the time
// of the edit here with the time the other version reached the server, so
// the version edited last does not always stay (docs, section 7).
func TestBackupNotice_SaysWhoseVersionStayed(t *testing.T) {
	for _, loc := range []string{"en", "ru"} {
		t.Run(loc, func(t *testing.T) {
			i18n.SetLocale(loc)
			defer i18n.SetLocale("en")
			stub := newStub(t)
			a := newMachine(t, stub, "a")
			a.converse(a.svc.OnFlow(), password, password)
			b := newMachine(t, stub, "b")
			b.converse(b.svc.OnFlow(), password)

			a.use()
			a.writeFile("memory/fact.md", "v1\n")
			now(a)
			now(b)
			a.use()
			a.writeFile("memory/fact.md", "from A\n")
			now(a)
			b.use()
			b.writeFile("memory/fact.md", "from B\n")
			_ = os.Chtimes(filepath.Join(b.dir(), "memory", "fact.md"), past, past)

			var notes []string
			b.svc.background(context.Background(), Hooks{Notify: func(s string) { notes = append(notes, s) }}, false)
			text := strings.Join(notes, "\n")
			for _, promise := range []string{"newer", "свеж", "поздн"} {
				if strings.Contains(text, promise) {
					t.Errorf("promises %q: %s", promise, text)
				}
			}
			want := map[string]string{"en": "the server's version is used here", "ru": "здесь теперь версия с сервера"}[loc]
			if !strings.Contains(text, want) || !strings.Contains(text, "fact.local-") {
				t.Errorf("does not say whose version stayed (%q): %s", want, text)
			}
		})
	}
}
