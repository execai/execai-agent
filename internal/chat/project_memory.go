// Project memory and the root EXECAI.md.
//
// The agent keeps a project's memory in <dir>/.execai/EXECAI.md: that
// folder is what sync carries between the person's computers. Older
// versions wrote it to <dir>/EXECAI.md at the root, which stays readable
// (people keep it in git) but does not travel. Nothing is moved on its own:
// the person is told once, in the chat, and /project memory copies the root
// file into .execai/ — the root file stays where it is.
package chat

import (
	"os"
	"strings"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/agent"
	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/inbox"
	agentsync "github.com/velesbsdllc/agent-vbai/internal/sync"
)

// projectMemoryHintFor — whether to say, once, that dir's project memory
// stays behind: sync is on, dir is a project bound on this machine (as
// sync.json last saw it — no network at start), EXECAI.md is at the root
// and .execai/EXECAI.md is not there.
func projectMemoryHintFor(dir string) bool {
	if dir == "" || config.LoadProjectState(dir).MemoryHintAt != "" {
		return false
	}
	if !nonEmptyFile(agent.ProjectInstructionsPath(dir)) || nonEmptyFile(agent.ProjectMemoryPath(dir)) {
		return false
	}
	paths, err := agentsync.DefaultPaths()
	if err != nil {
		return false
	}
	st, err := agentsync.LoadState(paths)
	if err != nil || !st.Enabled {
		return false
	}
	for _, bound := range st.Projects {
		if inbox.SamePath(bound, dir) {
			return true
		}
	}
	return false
}

// maybeHintProjectMemory adds the note to the chat when it is due and
// remembers that it was given.
func (m *tuiModel) maybeHintProjectMemory(dir string) {
	if !projectMemoryHintFor(dir) {
		return
	}
	st := config.LoadProjectState(dir)
	st.MemoryHintAt = time.Now().UTC().Format(time.RFC3339)
	if config.SaveProjectState(dir, st) != nil {
		return // not remembered: better silent than the same note every start
	}
	m.uiSegments = append(m.uiSegments, segment{kind: "system_hint", body: i18n.T("project.memory.hint")})
}

// projectMemory — /project memory: copy the root EXECAI.md into
// .execai/EXECAI.md.
func (m *tuiModel) projectMemory() string {
	cwd, _ := os.Getwd()
	res, err := agent.CopyRootToProjectMemory(cwd)
	if err != nil {
		return i18n.Tf("project.error", err)
	}
	root, target := agent.ProjectInstructionsPath(cwd), agent.ProjectMemoryPath(cwd)
	switch res {
	case agent.RootMemoryCopied:
		// It travels only from a bound folder with sync on: say what is
		// missing instead of promising.
		bound, syncOn := m.projectSyncState(cwd)
		switch {
		case bound && syncOn:
			return i18n.Tf("project.memory.copied", root, target)
		case !bound && !syncOn:
			return i18n.Tf("project.memory.copiedNeither", root, target)
		case !bound:
			return i18n.Tf("project.memory.copiedNotBound", root, target)
		}
		return i18n.Tf("project.memory.copiedSyncOff", root, target)
	case agent.RootMemoryAlreadyThere:
		return i18n.Tf("project.memory.same", target)
	case agent.RootMemoryTargetDiffers:
		return i18n.Tf("project.memory.differs", target, root)
	}
	return i18n.Tf("project.memory.noRoot", cwd, target)
}

// projectSyncState — whether dir is a project bound on this machine and
// whether sync is on here. The binding is asked from the server (this
// machine's own); without an agent sign-in or the network, sync.json's last
// list of bound projects answers.
func (m *tuiModel) projectSyncState(dir string) (bound, syncOn bool) {
	var st *agentsync.State
	if paths, err := agentsync.DefaultPaths(); err == nil {
		st, _ = agentsync.LoadState(paths)
	}
	syncOn = st != nil && st.Enabled
	if m.selfAgentID() == "" {
		// Not signed in as an agent: the server cannot tell this
		// machine's bindings; the local list answers below.
	} else if binds, err := m.fetchBindings(); err == nil {
		for _, b := range binds {
			if inbox.SamePath(b.LocalPath, dir) {
				return true, syncOn
			}
		}
		return false, syncOn
	}
	if st != nil {
		for _, p := range st.Projects {
			if inbox.SamePath(p, dir) {
				return true, syncOn
			}
		}
	}
	return false, syncOn
}

// nonEmptyFile — the file exists and holds more than blanks (as LoadMemory
// counts it).
func nonEmptyFile(path string) bool {
	data, err := os.ReadFile(path)
	return err == nil && strings.TrimSpace(string(data)) != ""
}
