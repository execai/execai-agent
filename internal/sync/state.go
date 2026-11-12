package sync

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Pause reasons kept in State.Paused.
const (
	PausedPlan  = "plan"  // the plan does not include sync (any more)
	PausedLogin = "login" // signed out, or the token is rejected
	PausedKey   = "key"   // the key on the server is not this machine's key
	// PausedNoKey — the server answered that it has no sync key: maybe sync
	// was deleted from another computer, maybe a gateway answered for it.
	// Nothing is reset or switched off: the person decides (/sync).
	PausedNoKey = "nokey"
)

// BlobState — what this machine last agreed with the server about one blob.
type BlobState struct {
	// Path — the file the blob holds: a path relative to the memory
	// directory, a workspace id, or the fixed name of a settings file.
	Path    string `json:"path"`
	Version int64  `json:"version"`
	// Hash — sha256 of the plaintext both sides had at Version; a local file
	// with another hash has changed here since.
	Hash    string `json:"hash,omitempty"`
	Deleted bool   `json:"deleted,omitempty"`
}

// State — sync.json. Never holds keys, passwords or file contents.
type State struct {
	Enabled bool `json:"enabled"`
	// Blobs by "kind/name".
	Blobs    map[string]BlobState `json:"blobs,omitempty"`
	LastPull time.Time            `json:"last_pull,omitempty"`
	LastPush time.Time            `json:"last_push,omitempty"`
	// LastError — the error of the latest exchange, "" when it went fine.
	LastError string `json:"last_error,omitempty"`
	// MachineManifestHash — what was last sent as this machine's manifest.
	MachineManifestHash string    `json:"machine_manifest_hash,omitempty"`
	ManifestAt          time.Time `json:"manifest_at,omitempty"`
	// Projects — workspace id → directory of the projects bound to this
	// machine, as of the latest full exchange.
	Projects map[string]string `json:"projects,omitempty"`
	// ProjectNames — workspace id → the project's name in the web console,
	// for the status line.
	ProjectNames map[string]string `json:"project_names,omitempty"`
	// Paused — why sync waits ("" = it does not): PausedPlan, PausedLogin,
	// PausedKey. The person is told once, when it starts.
	Paused string `json:"paused,omitempty"`
	// Unreadable — server files this key cannot open (encrypted to another
	// key), as of the latest exchange.
	Unreadable int `json:"unreadable,omitempty"`
	// Hidden — the projects (workspace ids) on the server the last full
	// exchange could not open: encrypted with a key no computer has now
	// (after «keep this computer's key»). /sync on asks about them.
	Hidden []string `json:"hidden,omitempty"`
	// Skipped — what the latest exchange left out in both directions, and
	// why (local.go).
	Skipped []Skip `json:"skipped,omitempty"`
	// QuotaFull — the server said the space is used up.
	QuotaFull bool `json:"quota_full,omitempty"`
	// OverwriteLeft — blobs keeping this computer's key did not delete
	// because they changed or appeared after the question; /sync on asks
	// about them again (overwrite.go).
	OverwriteLeft []string `json:"overwrite_left,omitempty"`
}

// Contents — what is in sync by the saved state: the personal memory files,
// whether the config blob is there, the projects bound here, not left out
// at the latest exchange, whose memory is in sync. Only Memory, Config and
// Projects of the report are filled.
func (s *State) Contents() Report {
	var r Report
	if s == nil {
		return r
	}
	left := map[string]bool{}
	leftOut := map[string]bool{} // workspace ids
	for _, sk := range s.Skipped {
		if sk.Reason == SkipFolderTaken {
			// The folder stays with the other project: only this one is out.
			leftOut[sk.Project] = true
			continue
		}
		left[sk.Path] = true
	}
	for k, b := range s.Blobs {
		if b.Deleted {
			continue
		}
		switch kind, name := splitKey(k); kind {
		case KindMemory:
			r.Memory++
		case KindConfig:
			r.Config = true
		case KindProjectMemory:
			if dir, bound := s.Projects[name]; bound && !leftOut[name] && !left[dir] && !left[ProjectDir(dir)] {
				r.Projects++
			}
		}
	}
	return r
}

// LoadState reads sync.json; a missing file is a disabled sync.
func LoadState(p Paths) (*State, error) {
	st := &State{}
	data, err := os.ReadFile(p.StateFile())
	if errors.Is(err, os.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	if err := json.Unmarshal(data, st); err != nil {
		return &State{}, fmt.Errorf("sync.json: %w", err)
	}
	for i, k := range st.Hidden {
		if !strings.Contains(k, "/") {
			st.Hidden[i] = blobKey(KindProjectMemory, k) // a candidate that listed projects only
		}
	}
	return st, nil
}

// Save writes sync.json atomically, mode 0600.
func (s *State) Save(p Paths) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(p.StateFile(), data)
}

// reset forgets everything agreed with the server; the next exchange starts
// from scratch.
func (s *State) reset() {
	s.Blobs = nil
	s.LastPull = time.Time{}
	s.LastPush = time.Time{}
	s.LastError = ""
	s.MachineManifestHash = ""
	s.ManifestAt = time.Time{}
	s.Unreadable = 0
	s.Hidden = nil
	s.Skipped = nil
	s.OverwriteLeft = nil
	s.QuotaFull = false
}

// writeFileAtomic writes data to path through a temporary file in the same
// folder renamed over it, mode 0600: path holds either all of the old
// content or all of the new, also after a power cut — the temporary file is
// flushed to disk before the rename, the folder after it.
func writeFileAtomic(path string, data []byte) error {
	tmp, err := prepareFile(path, data)
	if err != nil {
		return err
	}
	return commitFile(tmp, path)
}

// prepareFile writes data into a new temporary file next to path, flushed to
// disk, and returns its name; the folder is made if missing.
func prepareFile(path string, data []byte) (string, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return "", err
	}
	tmp := f.Name()
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp, 0o600)
	}
	if err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return tmp, nil
}

// privateDirs makes every folder from root down to dir (both included)
// reachable by this user only (0700) where it is not: copies of memory and
// settings may hold provider keys, and a folder an older agent or a loose
// umask made may be open to others. Folders outside root are not touched;
// on Windows a folder keeps the access of the config folder it is in.
func privateDirs(root, dir string) {
	if runtime.GOOS == "windows" {
		return
	}
	rel, err := filepath.Rel(root, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return
	}
	cur := root
	parts := []string{""}
	if rel != "." {
		parts = append(parts, strings.Split(rel, string(filepath.Separator))...)
	}
	for _, part := range parts {
		cur = filepath.Join(cur, part)
		if fi, err := os.Lstat(cur); err == nil && fi.IsDir() && fi.Mode().Perm()&0o077 != 0 {
			_ = os.Chmod(cur, 0o700)
		}
	}
}

// privateFile makes a copy found in place readable by this user only.
func privateFile(p string) {
	if runtime.GOOS == "windows" {
		return
	}
	if fi, err := os.Lstat(p); err == nil && fi.Mode().IsRegular() && fi.Mode().Perm()&0o077 != 0 {
		_ = os.Chmod(p, 0o600)
	}
}

// commitFile renames the temporary file tmp over path and flushes the
// folder, so the rename itself is on disk.
func commitFile(tmp, path string) error {
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	syncDir(filepath.Dir(path))
	return nil
}

// syncDir flushes a folder's entries to disk; where a folder cannot be
// flushed (Windows) nothing is done.
func syncDir(dir string) {
	if f, err := os.Open(dir); err == nil {
		_ = f.Sync()
		_ = f.Close()
	}
}
