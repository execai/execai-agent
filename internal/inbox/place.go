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

// WorkDir finds the directory bound to the task's project. Running in the
// owner's own directory instead is not an option: `execai serve` may have been
// started anywhere, and "fix the tests" would land in the wrong repository.
// A task outside a project (no workspace_id) runs in fallback.
func WorkDir(bindings []Binding, workspaceID, fallback string) (string, error) {
	if workspaceID == "" {
		return fallback, nil
	}
	for _, b := range bindings {
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

// WorkDirFor is WorkDir with the bindings fetched from the server; a task
// outside a project does not need them.
func (c *Client) WorkDirFor(ctx context.Context, workspaceID, fallback string) (string, error) {
	if workspaceID == "" {
		return fallback, nil
	}
	binds, err := c.Bindings(ctx)
	if err != nil {
		return "", errors.New(i18n.Tf("inbox.task.bindingsFailed", err))
	}
	return WorkDir(binds, workspaceID, fallback)
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

// Place decides where task t belongs for an owner working in cwd. It returns
// the directory (cwd for Here, the other project's for Elsewhere) and, for
// Broken, the reason.
func Place(t Task, bindings []Binding, cwd string) (Placement, string, error) {
	if t.WorkspaceID == "" {
		return Here, cwd, nil
	}
	for _, b := range bindings {
		if b.WorkspaceID != t.WorkspaceID || b.LocalPath == "" {
			continue
		}
		if SamePath(b.LocalPath, cwd) {
			return Here, cwd, nil
		}
	}
	dir, err := WorkDir(bindings, t.WorkspaceID, cwd)
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
