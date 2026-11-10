package sync

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// item — one local file as sync sees it.
type item struct {
	Kind, Name string
	// Path — what goes into the envelope (see BlobState.Path).
	Path string
	// File — where it lives on this machine.
	File    string
	Data    []byte
	Hash    string
	ModTime time.Time
}

func hashOf(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// skipMemoryFile — files in the memory directory that are not memory:
// hidden files, editor and temp leftovers.
func skipMemoryFile(name string) bool {
	return strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".tmp") ||
		strings.HasSuffix(name, "~") || strings.HasSuffix(name, ".swp")
}

// collect reads every local file that takes part in sync. tooBig receives
// the files over the size limit. memDirOK reports whether the memory
// directory exists — without it, missing memory files are not deletions.
func (e *Engine) collect(projects map[string]string, tooBig *[]string) (items map[string]item, memDirOK bool, err error) {
	items = map[string]item{}
	add := func(kind, name, p, file string, data []byte, mt time.Time) {
		items[blobKey(kind, name)] = item{Kind: kind, Name: name, Path: p, File: file, Data: data, Hash: hashOf(data), ModTime: mt}
	}

	memDir := e.Paths.MemoryDir()
	if st, err := os.Stat(memDir); err == nil && st.IsDir() {
		memDirOK = true
		err := filepath.WalkDir(memDir, func(file string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if file == memDir {
				return nil
			}
			if skipMemoryFile(d.Name()) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if d.IsDir() || !d.Type().IsRegular() {
				return nil
			}
			rel, err := filepath.Rel(memDir, file)
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
			if info.Size() > MaxBlobBytes {
				*tooBig = append(*tooBig, "memory/"+rel)
				return nil
			}
			data, err := os.ReadFile(file)
			if err != nil {
				return err
			}
			add(KindMemory, memoryName(e.ID, rel), rel, file, data, info.ModTime())
			return nil
		})
		if err != nil {
			return nil, false, err
		}
	}

	if it, ok, err := e.configItem(); err != nil {
		return nil, false, err
	} else if ok {
		items[blobKey(it.Kind, it.Name)] = it
	}

	for _, ws := range sortedKeys(projects) {
		if !validName(ws) {
			continue
		}
		it, ok, big, err := e.projectItem(ws, projects[ws])
		switch {
		case err != nil:
			continue // an unreadable project is skipped, not fatal
		case big:
			*tooBig = append(*tooBig, ProjectDir(projects[ws]))
		case ok:
			items[blobKey(it.Kind, it.Name)] = it
		}
	}
	return items, memDirOK, nil
}

// readFile — the content and mtime of a regular file; ok=false when absent.
func readFile(path string) ([]byte, time.Time, bool, error) {
	st, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, time.Time{}, false, nil
	}
	if err != nil {
		return nil, time.Time{}, false, err
	}
	if !st.Mode().IsRegular() {
		return nil, time.Time{}, false, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, time.Time{}, false, err
	}
	return data, st.ModTime(), true, nil
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// mergeLines — for MEMORY.md, the index, when both sides changed it: the
// remote lines in their order, then the local lines the remote lacks. Two
// machines adding notes at the same time is the common case, and losing a
// line of the index hides a whole note from the agent.
func mergeLines(remote, local []byte) []byte {
	have := map[string]bool{}
	rl := strings.Split(strings.TrimRight(string(remote), "\n"), "\n")
	for _, l := range rl {
		have[strings.TrimRight(l, " \t\r")] = true
	}
	out := append([]string(nil), rl...)
	for _, l := range strings.Split(strings.TrimRight(string(local), "\n"), "\n") {
		t := strings.TrimRight(l, " \t\r")
		if t == "" || have[t] {
			continue
		}
		have[t] = true
		out = append(out, l)
	}
	return []byte(strings.Join(out, "\n") + "\n")
}

// isIndex — the memory index file.
func isIndex(kind, p string) bool { return kind == KindMemory && p == "MEMORY.md" }
