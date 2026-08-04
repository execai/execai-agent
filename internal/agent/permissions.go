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
}

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
	data, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return out, err
	}
	return out, nil
}

func (p *Permissions) Save() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.path == "" {
		path, err := permissionsPath()
		if err != nil {
			return err
		}
		p.path = path
	}
	type onDisk struct {
		Tools []string `json:"always_allowed_tools"`
		Exact []string `json:"always_allowed_exact"`
	}
	data, err := json.MarshalIndent(onDisk{Tools: p.Tools, Exact: p.Exact}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p.path, data, 0o600)
}

func (p *Permissions) HasTool(name string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
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
	for _, e := range p.Exact {
		if e == key {
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
}
