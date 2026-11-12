package sync

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// A project bound to this machine (a web project ↔ a local directory) syncs
// its whole <dir>/.execai/ folder — today the project memory EXECAI.md,
// tomorrow whatever else lives there — as ONE opaque blob: kind
// project-memory, name = workspace id, a pack of the files, then age. Each
// machine may keep the project in its own directory; the blob carries
// paths relative to .execai/. A binding removed here leaves the blob alone:
// other machines may still hold the project.

// ProjectDirName — the synced folder inside a bound project.
const ProjectDirName = ".execai"

// MaxProjectBlobBytes — the server's limit for one project blob.
const MaxProjectBlobBytes = 1 << 20

// ProjectDir — <dir>/.execai.
func ProjectDir(dir string) string { return filepath.Join(dir, ProjectDirName) }

// blobLimit — the largest blob of a kind the server takes.
func blobLimit(kind string) int {
	if kind == KindProjectMemory {
		return MaxProjectBlobBytes
	}
	return MaxBlobBytes
}

// backupRe — the copies sync leaves next to a file when it loses a
// conflict: <name>.local-<time> or <name>.server-<time>. They stay on this
// machine and never travel.
var backupRe = regexp.MustCompile(`\.(local|server)-\d{8}-\d{6}$`)

// localOnly — files of .execai/ that stay on this machine.
func localOnly(name string) bool {
	return backupRe.MatchString(name) || strings.HasSuffix(name, ".tmp") ||
		strings.HasSuffix(name, "~") || strings.HasSuffix(name, ".swp")
}

// UploadsDirName — .execai/uploads/, where execai ui saves files the person
// attaches in the browser. Screenshots there would push the project blob
// over its limit, and a pull would delete them as "gone", so the folder
// stays on this machine and is neither sent nor pruned.
const UploadsDirName = "uploads"

// localOnlyRel — a path relative to .execai/ that never travels.
func localOnlyRel(rel string) bool {
	return rel == UploadsDirName || strings.HasPrefix(rel, UploadsDirName+"/") || localOnly(path.Base(rel))
}

// walkProject calls fn for every synced file of a project folder.
func walkProject(pd string, fn func(rel, file string, info fs.FileInfo) error) error {
	return filepath.WalkDir(pd, func(file string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			if d.Name() == UploadsDirName && filepath.Dir(file) == filepath.Clean(pd) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || localOnly(d.Name()) {
			return nil
		}
		rel, err := filepath.Rel(pd, file)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !safeRel(rel) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return fn(rel, file, info)
	})
}

// projectScan packs <dir>/.execai/; ok=false when it has no files. sk —
// the project is left out of this exchange in both directions: .execai/
// itself is a link, not a folder or cannot be read, or the pack is over the
// server's limit. An entry inside that is a link, not a regular file, or
// cannot be read is left out alone (item.Left): the rest of the project
// syncs, and for that name the server's version stays on the server and
// the one here stays here (projectsync.go).
func (e *Engine) projectScan(ws, dir string) (it item, ok bool, sk *Skip) {
	pd := ProjectDir(dir)
	fi, err := os.Lstat(pd)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return item{}, false, nil
	case err != nil:
		sk := unreadable(pd, pd, err)
		return item{}, false, &sk
	case fi.Mode()&fs.ModeSymlink != 0:
		return item{}, false, &Skip{Path: pd, Reason: SkipLink}
	case !fi.IsDir():
		return item{}, false, &Skip{Path: pd, Reason: SkipSpecial}
	}
	files := map[string][]byte{}
	var bad *Skip
	var left []string
	var notes []Skip
	leave := func(file string, d fs.DirEntry, sk Skip) error {
		rel, err := filepath.Rel(pd, file)
		if err != nil || !safeRel(filepath.ToSlash(rel)) {
			return nil
		}
		left = append(left, filepath.ToSlash(rel))
		sk.Path, sk.Detail, sk.One = file, "", true
		notes = append(notes, sk)
		if d != nil && d.IsDir() {
			return filepath.SkipDir
		}
		return nil
	}
	_ = filepath.WalkDir(pd, func(file string, d fs.DirEntry, err error) error {
		if err != nil {
			if file == pd {
				sk := unreadable(pd, file, err)
				bad = &sk
				return filepath.SkipAll
			}
			return leave(file, d, unreadable(file, file, err))
		}
		if file == pd {
			return nil
		}
		if d.IsDir() {
			if d.Name() == UploadsDirName && filepath.Dir(file) == filepath.Clean(pd) {
				return filepath.SkipDir
			}
			return nil
		}
		if localOnly(d.Name()) {
			return nil
		}
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			return leave(file, d, Skip{Reason: SkipLink})
		case !d.Type().IsRegular():
			return leave(file, d, Skip{Reason: SkipSpecial})
		}
		rel, err := filepath.Rel(pd, file)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if !safeRel(rel) {
			return nil // a name no other computer could take
		}
		data, err := os.ReadFile(file)
		if err == nil {
			files[rel] = data
			return nil
		}
		return leave(file, d, unreadable(file, file, err))
	})
	if bad != nil {
		return item{}, false, bad
	}
	if !hasContent(files) && len(left) == 0 {
		return item{}, false, nil // nothing but placeholders: empty (keep.go)
	}
	data, err := packFiles(files)
	if err != nil {
		sk := unreadable(pd, pd, err)
		return item{}, false, &sk
	}
	if sealedSize(KindProjectMemory, ws, int64(len(data))) > MaxProjectBlobBytes {
		return item{}, false, &Skip{Path: pd, Reason: SkipTooBig, Limit: MaxProjectBlobBytes >> 10}
	}
	return item{Kind: KindProjectMemory, Name: ws, Path: ws, File: pd, Data: data, Hash: hashOf(data),
		Left: left, LeftNotes: notes}, true, nil
}

// hasContent — files hold something besides placeholders (keep.go).
func hasContent(files map[string][]byte) bool {
	for rel := range files {
		if !contentless(path.Base(rel)) {
			return true
		}
	}
	return false
}

// isLeft — rel is one of left, or inside a folder in it.
func isLeft(left []string, rel string) bool {
	for _, x := range left {
		if rel == x || strings.HasPrefix(rel, x+"/") {
			return true
		}
	}
	return false
}

// projectItem — projectScan for a folder just written: a project left out
// is an error here.
func (e *Engine) projectItem(ws, dir string) (item, bool, error) {
	it, ok, sk := e.projectScan(ws, dir)
	if sk != nil {
		return item{}, false, fmt.Errorf("sync: %s: %s %s", sk.Path, sk.Reason, sk.Detail)
	}
	return it, ok, nil
}

func mustPack(files map[string][]byte) []byte {
	b, _ := packFiles(files)
	return b
}
