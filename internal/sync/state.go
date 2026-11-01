package sync

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Pause reasons kept in State.Paused.
const (
	PausedPlan  = "plan"  // the plan does not include sync (any more)
	PausedLogin = "login" // signed out, or the token is rejected
	PausedKey   = "key"   // the key on the server is not this machine's key
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
	// TooBig — files that are over the size limit and stay local.
	TooBig []string `json:"too_big,omitempty"`
	// QuotaFull — the server said the space is used up.
	QuotaFull bool `json:"quota_full,omitempty"`
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
	s.TooBig = nil
	s.QuotaFull = false
}

// writeFileAtomic writes via a temp file in the same directory, mode 0600.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
