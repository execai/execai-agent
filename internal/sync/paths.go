package sync

import (
	"path/filepath"

	"github.com/velesbsdllc/agent-vbai/internal/config"
)

// ProjectMemoryRel — the project memory file inside a bound project.
const ProjectMemoryRel = ".execai/EXECAI.md"

// Paths — where the synced files live on this machine.
type Paths struct {
	// ConfigDir — the agent's config directory (~/.config/execai).
	ConfigDir string
}

// DefaultPaths — the agent's own config directory.
func DefaultPaths() (Paths, error) {
	dir, err := config.Dir()
	if err != nil {
		return Paths{}, err
	}
	return Paths{ConfigDir: dir}, nil
}

// MemoryDir — the personal memory.
func (p Paths) MemoryDir() string { return filepath.Join(p.ConfigDir, "memory") }

// ConfigFile — config.json.
func (p Paths) ConfigFile() string { return filepath.Join(p.ConfigDir, "config.json") }

// SubscriptionsFile — subscriptions.json, the sources and their keys.
func (p Paths) SubscriptionsFile() string { return filepath.Join(p.ConfigDir, "subscriptions.json") }

// PermissionsFile — permissions.json.
func (p Paths) PermissionsFile() string { return filepath.Join(p.ConfigDir, "permissions.json") }

// StateFile — sync.json, this machine's sync state.
func (p Paths) StateFile() string { return filepath.Join(p.ConfigDir, "sync.json") }

// ProjectMemoryFile — the project memory of a project bound to dir.
func ProjectMemoryFile(dir string) string {
	return filepath.Join(dir, filepath.FromSlash(ProjectMemoryRel))
}
