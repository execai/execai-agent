package config

// Per-project state on this machine: what execai has already asked or done in
// a given directory. It lives in the config folder, not in the project, so a
// repository never gets execai files it did not ask for.
//
//	~/.config/execai/projects.json
//	{
//	  "projects": {
//	    "/home/u/work/app": {"import_offer": "declined", "import_offer_at": "2026-11-01T10:00:00Z"}
//	  }
//	}

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Answers to the one-time offer to import other agents' instructions.
const (
	ImportOfferAccepted = "accepted"
	ImportOfferDeclined = "declined"
)

// ProjectState is what execai remembers about one project directory.
type ProjectState struct {
	// ImportOffer is the answer to the first-run import offer; empty = never asked.
	ImportOffer   string `json:"import_offer,omitempty"`
	ImportOfferAt string `json:"import_offer_at,omitempty"`
	// MemoryHintAt — when the person was told that the project memory in
	// the root EXECAI.md does not travel between computers; empty = never.
	MemoryHintAt string `json:"memory_hint_at,omitempty"`
}

type projectsFile struct {
	Projects map[string]ProjectState `json:"projects"`
}

func projectsPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "execai", "projects.json"), nil
}

// projectKey normalizes a directory so "app" and "app/" are one project.
func projectKey(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	return filepath.Clean(abs)
}

func loadProjects() (projectsFile, string, error) {
	pf := projectsFile{Projects: map[string]ProjectState{}}
	path, err := projectsPath()
	if err != nil {
		return pf, "", err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return pf, path, nil
	}
	if err != nil {
		return pf, path, err
	}
	if err := json.Unmarshal(data, &pf); err != nil {
		return pf, path, fmt.Errorf("%s: %w", path, err)
	}
	if pf.Projects == nil {
		pf.Projects = map[string]ProjectState{}
	}
	return pf, path, nil
}

// LoadProjectState returns what is remembered about dir. It never writes:
// a missing or unreadable file means "nothing remembered".
func LoadProjectState(dir string) ProjectState {
	pf, _, err := loadProjects()
	if err != nil {
		return ProjectState{}
	}
	return pf.Projects[projectKey(dir)]
}

// SaveProjectState remembers st for dir, keeping the other projects.
func SaveProjectState(dir string, st ProjectState) error {
	pf, path, err := loadProjects()
	if err != nil {
		// A broken file is not overwritten: it may hold answers the user
		// wants to keep, and asking once more is the lesser harm.
		return err
	}
	pf.Projects[projectKey(dir)] = st
	data, err := json.MarshalIndent(pf, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
