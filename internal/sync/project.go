package sync

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
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

// projectItem packs <dir>/.execai/; ok=false when it has no files. tooBig —
// the pack is over the server's limit.
func (e *Engine) projectItem(ws, dir string) (it item, ok, tooBig bool, err error) {
	pd := ProjectDir(dir)
	files := map[string][]byte{}
	var mt time.Time
	err = walkProject(pd, func(rel, file string, info fs.FileInfo) error {
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		files[rel] = data
		if info.ModTime().After(mt) {
			mt = info.ModTime()
		}
		return nil
	})
	if err != nil || len(files) == 0 {
		return item{}, false, false, err
	}
	data, err := packFiles(files)
	if err != nil {
		return item{}, false, false, err
	}
	if len(data) > MaxProjectBlobBytes {
		return item{}, false, true, nil
	}
	return item{Kind: KindProjectMemory, Name: ws, Path: ws, File: pd, Data: data, Hash: hashOf(data), ModTime: mt}, true, false, nil
}

// applyProject makes the folder what the blob says: its files written, the
// synced files it no longer has removed. Copies left by conflicts stay.
func applyProject(pd string, data []byte, mtime time.Time) error {
	files, err := unpackFiles(data)
	if err != nil {
		return err
	}
	for _, rel := range sortedKeys(files) {
		if !safeRel(rel) || localOnlyRel(rel) {
			continue
		}
		target := filepath.Join(pd, filepath.FromSlash(rel))
		if err := writeFileAtomic(target, files[rel]); err != nil {
			return err
		}
		if !mtime.IsZero() {
			_ = os.Chtimes(target, mtime, mtime)
		}
	}
	var gone []string
	if err := walkProject(pd, func(rel, file string, _ fs.FileInfo) error {
		if _, ok := files[rel]; !ok {
			gone = append(gone, file)
		}
		return nil
	}); err != nil {
		return err
	}
	for _, f := range gone {
		if err := os.Remove(f); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	pruneEmptyDirs(pd)
	return nil
}

// pruneEmptyDirs removes the empty directories under pd, pd included.
func pruneEmptyDirs(pd string) {
	var dirs []string
	_ = filepath.WalkDir(pd, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			dirs = append(dirs, p)
		}
		return nil
	})
	for i := len(dirs) - 1; i >= 0; i-- {
		_ = os.Remove(dirs[i]) // fails, as it should, on a directory with files
	}
}

// removeProject — the project was deleted elsewhere: its synced files go,
// copies left by conflicts stay; emptied directories are removed.
func removeProject(pd string) error {
	return applyProject(pd, mustPack(map[string][]byte{}), time.Time{})
}

func mustPack(files map[string][]byte) []byte {
	b, _ := packFiles(files)
	return b
}

// backupLosing keeps the losing side of a project conflict: every file of
// loser that the winner does not have as is, written next to it as
// <name>.<tag>-<time> (tag: local or server). Returns the paths written.
func backupLosing(pd string, loser, winner []byte, tag string, now time.Time) ([]string, error) {
	lf, err := unpackFiles(loser)
	if err != nil {
		return nil, err
	}
	wf, err := unpackFiles(winner)
	if err != nil {
		return nil, err
	}
	stamp := now.Local().Format("20060102-150405")
	var out []string
	for _, rel := range sortedKeys(lf) {
		if !safeRel(rel) || localOnly(rel) {
			continue
		}
		if w, ok := wf[rel]; ok && string(w) == string(lf[rel]) {
			continue
		}
		target := filepath.Join(pd, filepath.FromSlash(rel)) + fmt.Sprintf(".%s-%s", tag, stamp)
		if err := writeFileAtomic(target, lf[rel]); err != nil {
			return out, err
		}
		out = append(out, target)
	}
	sort.Strings(out)
	return out, nil
}
