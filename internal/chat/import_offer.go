// One-time offer to bring over other agents' instructions (E100).
//
// Someone coming from Claude Code, Cursor or Codex CLI already has the
// project explained in CLAUDE.md, .cursorrules, AGENTS.md… When the chat
// starts in such a project and execai has no memory for it yet, we list what
// was found and ask once. The answer is remembered per project
// (~/.config/execai/projects.json); declining touches nothing else.
package chat

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/agent"
	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/tools"
)

// maxOfferLines keeps the question readable in a terminal.
const maxOfferLines = 12

// importOfferFor returns what to offer in dir, or nil when not to ask:
// already answered here, execai already has project memory, the folder is
// home or the filesystem root, or only user-level files were found (those
// alone would make every folder ask).
func importOfferFor(dir string) []agent.ImportSource {
	if dir == "" {
		return nil
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil
	}
	if home, err := os.UserHomeDir(); err == nil && abs == filepath.Clean(home) {
		return nil
	}
	if abs == filepath.Dir(abs) {
		return nil
	}
	if config.LoadProjectState(abs).ImportOffer != "" {
		return nil
	}
	for _, f := range []string{"EXECAI.md", filepath.Join(".execai", "EXECAI.md")} {
		if _, err := os.Stat(filepath.Join(abs, f)); err == nil {
			return nil
		}
	}
	plan, err := agent.PlanImport(agent.DiscoverAllMemorySources(abs))
	if err != nil {
		return nil
	}
	srcs := agent.Importable(plan)
	for _, s := range srcs {
		if !s.Global {
			return srcs
		}
	}
	return nil
}

// leaveLoginScreen finishes a start without an ExecAI account (a key was
// connected from the login screen): the system prompt with memory is built
// only at boot, so it is built here too, and the import offer is made.
func (m *tuiModel) leaveLoginScreen() {
	m.reloadMemory()
	m.maybeOfferImport()
}

// maybeOfferImport asks at chat start, in the current directory.
func (m *tuiModel) maybeOfferImport() {
	m.maybeOfferImportIn(getCWDForBoot())
}

func (m *tuiModel) maybeOfferImportIn(dir string) {
	if m.asking || m.approving {
		return
	}
	srcs := importOfferFor(dir)
	if len(srcs) == 0 {
		return
	}

	var names []string
	var list strings.Builder
	for i, s := range srcs {
		if !slices.Contains(names, s.Tool) {
			names = append(names, s.Tool)
		}
		if i < maxOfferLines {
			fmt.Fprintf(&list, "  • %s — %s\n", s.Rel, s.Label())
		}
	}
	if len(srcs) > maxOfferLines {
		list.WriteString(i18n.Tf("import.offer.more", len(srcs)-maxOfferLines) + "\n")
	}

	yes := i18n.T("import.offer.yes")
	m.asking = true
	m.askFocus = 0
	m.askQuestion = i18n.Tf("import.offer.question", strings.Join(names, " / "), strings.TrimRight(list.String(), "\n"))
	m.askOptions = []tools.AskOption{
		{Label: yes, Description: i18n.T("import.offer.yesDesc")},
		{Label: i18n.T("import.offer.no"), Description: i18n.T("import.offer.noDesc")},
	}
	m.askPending = func(answer string) string {
		// Esc counts as "no": the offer is made once, and a dismissed
		// question that keeps coming back is worse than a missed import.
		st := config.ProjectState{ImportOffer: config.ImportOfferDeclined, ImportOfferAt: time.Now().UTC().Format(time.RFC3339)}
		if answer == yes {
			st.ImportOffer = config.ImportOfferAccepted
		}
		_ = config.SaveProjectState(dir, st)
		if answer != yes {
			return i18n.T("import.offer.declined")
		}
		return m.runImport(srcs)
	}
}
