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
)

// item — one local file as sync sees it.
type item struct {
	Kind, Name string
	// Path — what goes into the envelope (see BlobState.Path).
	Path string
	// File — where it lives on this machine.
	File string
	Data []byte
	Hash string
	// Raw — for the settings: file name → hash of the file as read (the
	// pack holds config.json without what stays this computer's own).
	Raw map[string]string
	// Left — for a project: entries of .execai/ (files, or folders that
	// could not be read: slash paths) left out of this exchange — not read,
	// not written, not removed; LeftNotes — what to tell about them.
	Left      []string
	LeftNotes []Skip
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

// Why something is left out of an exchange (Skip.Reason).
const (
	SkipTooBig        = "too_big"        // over the server's limit (Limit, KB)
	SkipUnreadable    = "unreadable"     // could not be read (Detail — the error)
	SkipLink          = "link"           // a symbolic link: sync does not follow links
	SkipLinkInside    = "link_inside"    // a project with a link inside (Detail — where)
	SkipSpecial       = "special"        // not a regular file or folder
	SkipSpecialInside = "special_inside" // a project with such a thing inside (Detail)
	SkipNoFolder      = "no_folder"      // the folder bound to a project is not here
	SkipFolderTaken   = "folder_taken"   // the folder holds another project (Detail — its name)
	SkipNameClash     = "name_clash"     // a name that is a file on one side and a folder on the other
)

// Skip — something left out of an exchange in both directions, and why. A
// file that could not be read, is too large or is a link is not «no file»:
// it is not sent, nothing from the server is written over it, it is not
// removed and no deletion of it is sent — it stays as it is here and on the
// other computers until the cause is gone.
type Skip struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
	Detail string `json:"detail,omitempty"`
	// Cause — for SkipUnreadable: causePermission or causeRead.
	Cause string `json:"cause,omitempty"`
	// Limit — for SkipTooBig, the limit in KB.
	Limit int `json:"limit,omitempty"`
	// One — only this entry of a project's .execai/ (Path) is left out: the
	// rest of the project syncs, and for this name the server's version
	// stays on the server while the one here stays here.
	One bool `json:"one,omitempty"`
	// Project — for SkipFolderTaken: the workspace id of the project left
	// out (Path is the folder, which the project that keeps it shares).
	Project string `json:"project,omitempty"`
}

// The causes of SkipUnreadable: no permission to read, or another error
// (the raw error text is not shown: it repeats the full path in English).
const (
	causePermission = "permission"
	causeRead       = "read"
	causeWrite      = "write" // could not be written (a folder in the way, no permission, a full disk)
)

// unreadable — a Skip for something that could not be read: path — what is
// left out (a project's .execai, a memory file), file — what failed in it.
func unreadable(path, file string, err error) Skip {
	sk := Skip{Path: path, Reason: SkipUnreadable, Cause: causeRead}
	if errors.Is(err, fs.ErrPermission) {
		sk.Cause = causePermission
	}
	if file != "" && file != path {
		if rel, rerr := filepath.Rel(path, file); rerr == nil && !strings.HasPrefix(rel, "..") {
			sk.Detail = filepath.ToSlash(rel)
		}
	}
	return sk
}

// scan — the local side of one exchange (collect).
type scan struct {
	items map[string]item
	// skipped — blob keys left out: a whole project, the settings, one
	// memory file.
	skipped map[string]bool
	// memSkip — memory paths (files and folders, relative, slash) left out;
	// a memory blob is matched against them once it is opened.
	memSkip []string
	// memAll — the whole personal memory is left out (its folder is a
	// link, or cannot be read).
	memAll bool
	// mem — the memory folder as found.
	mem memState
	// notes — what is left out, for the person.
	notes []Skip
}

// memState — the personal memory folder as one exchange found it.
type memState struct {
	present bool // a real folder is there
}

// memSkipped — the memory file at rel is left out of this exchange.
func (s *scan) memSkipped(rel string) bool {
	if s.memAll {
		return true
	}
	for _, p := range s.memSkip {
		if rel == p || strings.HasPrefix(rel, p+"/") {
			return true
		}
	}
	return false
}

func (s *scan) skip(key, path, reason string, err error) {
	sk := Skip{Path: path, Reason: reason}
	if reason == SkipUnreadable {
		sk = unreadable(path, path, err)
	}
	s.addSkip(key, sk)
}

func (s *scan) addSkip(key string, sk Skip) {
	if key != "" {
		s.skipped[key] = true
	}
	s.notes = append(s.notes, sk)
}

// sealedSize — a bound on the size of a blob of n bytes once sealed
// (envelope.go: the envelope line, then age: header, nonce and a tag per
// 64 KiB chunk), so a file that would not fit is left out before anything
// is decided about it.
func sealedSize(kind, p string, n int64) int64 {
	return n + int64(len(envelopeMagic)+len(kind)+len(p)+3) + 512 + 16*(n/(64<<10)+1)
}

// collect reads every local file that takes part in this exchange, and
// says what it leaves out and why (scan.skipped, scan.notes). Only a
// project exchange (opt.Only) reads only that project.
func (e *Engine) collect(projects map[string]string, opt Options) (*scan, error) {
	sc := &scan{items: map[string]item{}, skipped: map[string]bool{}}
	add := func(kind, name, p, file string, data []byte) {
		sc.items[blobKey(kind, name)] = item{Kind: kind, Name: name, Path: p, File: file, Data: data, Hash: hashOf(data)}
	}

	if opt.Only == "" || strings.HasPrefix(opt.Only, KindMemory+"/") {
		e.collectMemory(sc, add)
	}

	if opt.wants(blobKey(KindConfig, NameConfig)) {
		it, ok, sk := e.configScan()
		switch {
		case sk != nil:
			sc.addSkip(blobKey(KindConfig, NameConfig), *sk)
		case ok:
			sc.items[blobKey(it.Kind, it.Name)] = it
		}
	}

	for _, ws := range sortedKeys(projects) {
		key := blobKey(KindProjectMemory, ws)
		if !validName(ws) || !opt.wants(key) {
			continue
		}
		dir := projects[ws]
		fi, err := os.Stat(dir)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			sc.skip(key, dir, SkipNoFolder, nil)
			continue
		case err != nil:
			sc.skip(key, dir, SkipUnreadable, err)
			continue
		case !fi.IsDir():
			sc.skip(key, dir, SkipSpecial, nil)
			continue
		}
		it, ok, sk := e.projectScan(ws, dir)
		switch {
		case sk != nil:
			sc.addSkip(key, *sk)
		case ok:
			sc.items[key] = it
			sc.notes = append(sc.notes, it.LeftNotes...)
		}
	}
	return sc, nil
}

// collectMemory reads the personal memory into sc. A memory folder that is
// a link or cannot be read leaves the whole memory out; a file or folder
// inside it that is a link, cannot be read or is too large leaves out that
// path only.
func (e *Engine) collectMemory(sc *scan, add func(kind, name, p, file string, data []byte)) {
	memDir := e.Paths.MemoryDir()
	fi, err := os.Lstat(memDir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return
	case err != nil:
		sc.memAll = true
		sc.skip("", memDir, SkipUnreadable, err)
		return
	case fi.Mode()&fs.ModeSymlink != 0:
		sc.memAll = true
		sc.skip("", memDir, SkipLink, nil)
		return
	case !fi.IsDir():
		sc.memAll = true
		sc.skip("", memDir, SkipSpecial, nil)
		return
	}
	sc.mem.present = true
	_ = filepath.WalkDir(memDir, func(file string, d fs.DirEntry, err error) error {
		if file == memDir {
			if err != nil {
				sc.memAll = true
				sc.skip("", memDir, SkipUnreadable, err)
				return filepath.SkipAll
			}
			return nil
		}
		rel, rerr := filepath.Rel(memDir, file)
		if rerr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if skipMemoryFile(filepath.Base(file)) {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		leave := func(reason string, err error) error {
			sc.memSkip = append(sc.memSkip, rel)
			sc.skip(blobKey(KindMemory, memoryName(e.ID, rel)), file, reason, err)
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		switch {
		case err != nil:
			return leave(SkipUnreadable, err)
		case d.Type()&fs.ModeSymlink != 0:
			return leave(SkipLink, nil)
		case d.IsDir():
			return nil
		case !d.Type().IsRegular():
			return leave(SkipSpecial, nil)
		case !safeRel(rel):
			return nil // a name no other computer could take
		}
		info, err := d.Info()
		if err != nil {
			return leave(SkipUnreadable, err)
		}
		if sealedSize(KindMemory, rel, info.Size()) > MaxBlobBytes {
			sc.memSkip = append(sc.memSkip, rel)
			sc.addSkip(blobKey(KindMemory, memoryName(e.ID, rel)), Skip{Path: file, Reason: SkipTooBig, Limit: MaxBlobBytes >> 10})
			return nil
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return leave(SkipUnreadable, err)
		}
		add(KindMemory, memoryName(e.ID, rel), rel, file, data)
		return nil
	})
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// mergeLines — for MEMORY.md, the index, when both sides changed it or
// there is no base: the remote lines in their order, then the local entry
// lines the remote lacks (not the ones inside an HTML comment — the
// template the chat writes into a new memory folder). Two machines adding
// notes at the same time is the common case, and losing a line of the
// index hides a whole note from the agent.
func mergeLines(remote, local []byte) []byte {
	have := map[string]bool{}
	rl := strings.Split(strings.TrimRight(string(remote), "\n"), "\n")
	for _, l := range rl {
		have[strings.TrimRight(l, " \t\r")] = true
	}
	out := append([]string(nil), rl...)
	for _, l := range indexEntries(local) {
		if have[l] {
			continue
		}
		have[l] = true
		out = append(out, l)
	}
	return []byte(strings.Join(out, "\n") + "\n")
}

// isIndex — the memory index file.
func isIndex(kind, p string) bool { return kind == KindMemory && p == "MEMORY.md" }
