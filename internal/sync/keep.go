package sync

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// What the agent never deletes on its own.
//
// The memory of a whole project is never deleted from the server by the
// agent. A project folder without .execai/ files — the binding moved to an
// empty folder (a fresh clone), the folder deleted and made again, .execai/
// removed by hand, a disk not mounted — is not a deletion: the agent takes
// the server's version into the folder, as a fresh computer would, and says
// so. Deleting one file inside a live .execai/ — the folder trusted by its
// identity (bases.go) — is an ordinary change and travels. The same holds
// for the personal memory as a whole: a memory folder without its index, or
// with none of its files left, is filled from the server, not emptied on
// the server.
//
// Each file is decided on its own by its content against the base and the
// server (threeway.go); the base of a project — a manifest of its files —
// belongs to the pair (project, folder) (bases.go). When the binding moves
// to another folder, edits there are not taken for this project's edits:
// the server's version stays and whatever differs is kept as a copy.
//
// The other direction — what arrives from older agents (7.15 and 7.17),
// which still send a tombstone of the whole project when their folder
// moves: a computer with live files does not erase them. It keeps them,
// marks the project as deleted elsewhere (BlobState Deleted with the Hash
// of the kept files — a «kept» mark) and says so; it does not send them
// back by itself, so the two computers cannot bounce the project between
// them. A change made here — before the mark arrived or afterwards — sends
// it again as an ordinary edit. What an arriving version replaces or
// removes here is kept (keeper.go): in the history when the server had it,
// next to the file when it was never sent.

// contentless — files that do not make a project's memory: placeholders
// a repository keeps (.gitkeep) and what file managers leave behind. A
// .execai/ with nothing else is empty (keep.go): filled from the server,
// never sent as «everything deleted».
func contentless(name string) bool {
	switch strings.ToLower(name) {
	case ".gitkeep", ".keep", ".ds_store", "thumbs.db", "desktop.ini":
		return true
	}
	return false
}

// samePath reports whether two folder paths name the same folder: equal
// after cleaning, or after resolving symlinks; case-insensitive where the
// file system usually is. The identity of the folder (bases.go) is
// checked besides.
func samePath(a, b string) bool {
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

// projectCopies counts the copies kept in the bound projects' folders
// (<name>.local-<time>, <name>.server-<time>) — for /sync.
func projectCopies(projects map[string]string) int {
	n := 0
	for _, dir := range projects {
		pd := ProjectDir(dir)
		_ = filepath.WalkDir(pd, func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && backupRe.MatchString(d.Name()) {
				n++
			}
			return nil
		})
	}
	return n
}

// keptProjects counts the projects deleted on another computer whose files
// are kept here.
func keptProjects(st *State) int {
	n := 0
	for k, b := range st.Blobs {
		kind, ws := splitKey(k)
		if kind != KindProjectMemory || !b.Deleted {
			continue
		}
		if dir, bound := st.Projects[ws]; bound && hasProjectFiles(dir) {
			n++
		}
	}
	return n
}

// hasProjectFiles — dir's .execai/ holds something (a project kept here
// while the server marks it deleted).
func hasProjectFiles(dir string) bool {
	entries, err := os.ReadDir(ProjectDir(dir))
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !contentless(e.Name()) && !localOnly(e.Name()) && e.Name() != UploadsDirName {
			return true
		}
	}
	return false
}
