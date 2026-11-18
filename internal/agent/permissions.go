// Persistent tool permissions — which tools the user has allowed once and
// forever. Stored in os.UserConfigDir()/execai/permissions.json so they
// survive CLI restarts.
//
// Format:
//
//	{
//	  "always_allowed_tools": ["Read", "Bash"],
//	  "always_allowed_exact": ["Bash|{\"command\":\"git status\"}"]
//	}
package agent

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

type Permissions struct {
	Tools []string `json:"always_allowed_tools"`
	Exact []string `json:"always_allowed_exact"`

	mu   sync.Mutex
	path string

	// addedTools/addedExact — what this instance granted since it was
	// loaded. Save merges exactly these into whatever is on disk at that
	// moment: several agents (the main one and its subagents, or two
	// processes) each hold their own copy, and writing a whole stale copy
	// back would erase what the others granted in the meantime.
	addedTools []string
	addedExact []string
	// stamp — size and mtime of the file this copy reflects; a changed file
	// is re-read before answering, so a grant saved by another agent counts
	// at once instead of after a restart.
	stamp fileStamp
}

// fileStamp identifies a version of permissions.json cheaply.
type fileStamp struct {
	size    int64
	modTime int64
}

// permFileMu serializes read-merge-write cycles of permissions.json inside
// the process; rename makes each write atomic for other processes.
var permFileMu sync.Mutex

func permissionsPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "execai")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "permissions.json"), nil
}

// LoadPermissions reads permissions.json. If the file does not exist, it
// returns empty permissions with the path prepared for a subsequent Save.
func LoadPermissions() (*Permissions, error) {
	p, err := permissionsPath()
	if err != nil {
		return nil, err
	}
	out := &Permissions{path: p}
	err = out.readLocked()
	return out, err
}

// readLocked replaces Tools/Exact with the file's content and re-applies the
// grants this copy made itself. Caller holds p.mu (or owns p exclusively).
func (p *Permissions) readLocked() error {
	st, data, err := readPermFile(p.path)
	if errors.Is(err, os.ErrNotExist) {
		p.Tools, p.Exact = appendNew(nil, p.addedTools), appendNew(nil, p.addedExact)
		p.stamp = fileStamp{}
		return nil
	}
	if err != nil {
		return err
	}
	var disk struct {
		Tools []string `json:"always_allowed_tools"`
		Exact []string `json:"always_allowed_exact"`
	}
	if err := json.Unmarshal(data, &disk); err != nil {
		return err
	}
	p.Tools = appendNew(disk.Tools, p.addedTools)
	p.Exact = appendNew(disk.Exact, p.addedExact)
	p.stamp = st
	return nil
}

func readPermFile(path string) (fileStamp, []byte, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return fileStamp{}, nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fileStamp{}, nil, err
	}
	return fileStamp{size: fi.Size(), modTime: fi.ModTime().UnixNano()}, data, nil
}

// refreshLocked re-reads the file when it changed since this copy saw it.
// Errors keep the copy as it is: a half-written or broken file must not
// take away permissions the person already gave.
func (p *Permissions) refreshLocked() {
	if p.path == "" {
		return
	}
	fi, err := os.Stat(p.path)
	if err != nil {
		return
	}
	if (fileStamp{size: fi.Size(), modTime: fi.ModTime().UnixNano()}) == p.stamp {
		return
	}
	tools, exact, stamp := p.Tools, p.Exact, p.stamp
	if p.readLocked() != nil {
		p.Tools, p.Exact, p.stamp = tools, exact, stamp
	}
}

// appendNew returns base plus the entries of extra it does not contain yet.
func appendNew(base, extra []string) []string {
	out := append([]string(nil), base...)
	for _, e := range extra {
		found := false
		for _, b := range out {
			if b == e {
				found = true
				break
			}
		}
		if !found {
			out = append(out, e)
		}
	}
	return out
}

// Save writes the grants of this copy into permissions.json without losing
// anyone else's: the file is re-read under a lock, this copy's own grants
// are merged in, and the result replaces the file atomically (tmp + rename).
// Parallel agents saying "always" at the same moment both end up in the file.
func (p *Permissions) Save() error {
	permFileMu.Lock()
	defer permFileMu.Unlock()
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.path == "" {
		path, err := permissionsPath()
		if err != nil {
			return err
		}
		p.path = path
	}
	if err := p.readLocked(); err != nil && !errors.Is(err, os.ErrNotExist) {
		// A broken file is not merged into: rewriting it from memory is the
		// old behaviour and keeps at least this copy's view.
		p.Tools = appendNew(p.Tools, p.addedTools)
		p.Exact = appendNew(p.Exact, p.addedExact)
	}
	type onDisk struct {
		Tools []string `json:"always_allowed_tools"`
		Exact []string `json:"always_allowed_exact"`
	}
	data, err := json.MarshalIndent(onDisk{Tools: p.Tools, Exact: p.Exact}, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p.path), ".permissions-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, p.path); err != nil {
		os.Remove(tmpName)
		return err
	}
	if fi, err := os.Stat(p.path); err == nil {
		p.stamp = fileStamp{size: fi.Size(), modTime: fi.ModTime().UnixNano()}
	}
	return nil
}

func (p *Permissions) HasTool(name string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.refreshLocked()
	for _, t := range p.Tools {
		if t == name {
			return true
		}
	}
	return false
}

func (p *Permissions) HasExact(key string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.refreshLocked()
	// Канонизируем обе стороны: записи в файле могли быть сделаны руками или
	// старой версией — с description и своим порядком полей.
	want := canonicalExisting(key)
	for _, e := range p.Exact {
		if canonicalExisting(e) == want {
			return true
		}
	}
	return false
}

func (p *Permissions) AddTool(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, t := range p.Tools {
		if t == name {
			return
		}
	}
	p.Tools = append(p.Tools, name)
	p.addedTools = append(p.addedTools, name)
}

func (p *Permissions) AddExact(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, e := range p.Exact {
		if e == key {
			return
		}
	}
	p.Exact = append(p.Exact, key)
	p.addedExact = append(p.addedExact, key)
}
