package inbox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// Own keeps the bindings of the machine agentID. The server lists the
// bindings of all the person's machines, and another machine's binding is
// not this one's even when its directory happens to exist here (the same
// path on two computers is common): a task run there would land in a folder
// nobody bound on this computer. Without an agent id (signed in with a
// browser token) nothing is this machine's.
func Own(bindings []Binding, agentID string) []Binding {
	if agentID == "" {
		return nil
	}
	var out []Binding
	for _, b := range bindings {
		if b.AgentID == agentID {
			out = append(out, b)
		}
	}
	return out
}

// WorkDir finds the directory this machine (selfAgentID) bound to the task's
// project. Running in the owner's own directory instead is not an option:
// `execai serve` may have been started anywhere, and "fix the tests" would
// land in the wrong repository. Neither is another machine's directory: the
// project bound only elsewhere is refused here, with the reason. A task
// outside a project (no workspace_id) runs in fallback.
func WorkDir(bindings []Binding, selfAgentID, workspaceID, fallback string) (string, error) {
	if workspaceID == "" {
		return fallback, nil
	}
	for _, b := range Own(bindings, selfAgentID) {
		if b.WorkspaceID != workspaceID {
			continue
		}
		if b.LocalPath == "" {
			return "", errors.New(i18n.T("inbox.task.noDir"))
		}
		if st, err := os.Stat(b.LocalPath); err != nil || !st.IsDir() {
			// The directory may have moved. Quietly working elsewhere is not
			// an option.
			return "", errors.New(i18n.Tf("inbox.task.dirMissing", b.LocalPath))
		}
		return b.LocalPath, nil
	}
	return "", errors.New(i18n.T("inbox.task.notBound"))
}

// WorkDirFor is WorkDir with this machine's bindings fetched from the
// server; a task outside a project does not need them.
func (c *Client) WorkDirFor(ctx context.Context, selfAgentID, workspaceID, fallback string) (string, error) {
	if workspaceID == "" {
		return fallback, nil
	}
	binds, err := c.OwnBindings(ctx, selfAgentID)
	if err != nil {
		return "", errors.New(i18n.Tf("inbox.task.bindingsFailed", err))
	}
	return WorkDir(binds, selfAgentID, workspaceID, fallback)
}

// Placement says where a task belongs for an owner working in one directory
// (the editor panel: one window — one project).
type Placement int

const (
	// Here — the task runs in this directory: it belongs to the project bound
	// here, or to no project at all.
	Here Placement = iota
	// Elsewhere — the task belongs to a project bound to another directory of
	// this machine: leave it pending for the process working there.
	Elsewhere
	// Broken — no process on this machine can run it (the project is not
	// bound here, or its directory is gone): take it and answer with the
	// error, otherwise the web chat waits for nothing.
	Broken
)

// Place decides where task t belongs for an owner working in cwd on the
// machine selfAgentID; only this machine's bindings count. It returns the
// directory (cwd for Here, the other project's for Elsewhere) and, for
// Broken, the reason.
func Place(t Task, bindings []Binding, selfAgentID, cwd string) (Placement, string, error) {
	if t.WorkspaceID == "" {
		return Here, cwd, nil
	}
	for _, b := range Own(bindings, selfAgentID) {
		if b.WorkspaceID != t.WorkspaceID || b.LocalPath == "" {
			continue
		}
		if SamePath(b.LocalPath, cwd) {
			return Here, cwd, nil
		}
	}
	dir, err := WorkDir(bindings, selfAgentID, t.WorkspaceID, cwd)
	if err != nil {
		return Broken, "", err
	}
	return Elsewhere, dir, nil
}

// SamePath reports whether a and b name the same directory: equal after
// cleaning, or after resolving symlinks (an editor may open a project through
// a link the terminal never used). Case-insensitive where the file system
// usually is.
func SamePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	eq := func(x, y string) bool {
		x, y = filepath.Clean(x), filepath.Clean(y)
		if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
			return strings.EqualFold(x, y)
		}
		return x == y
	}
	if eq(a, b) {
		return true
	}
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && eq(ra, rb)
}
