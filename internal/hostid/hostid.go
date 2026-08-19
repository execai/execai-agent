// Stable host-id — a unique machine ID, stable across execai reinstalls.
// Used by the agent-link reuse logic on the backend: one physical machine
// = one persistent Session per user.
//
// Source priority:
//   1. machineid.ProtectedID("execai") — reads /etc/machine-id (Linux),
//      IOPlatformUUID (macOS), the registry MachineGuid (Windows). Cross-platform.
//   2. Fallback: a UUID generated once and stored in ~/.config/execai/host_id
//      (for cases where machine-id is unavailable: docker without --host-ipc,
//      Windows Sandbox, restricted permissions).
//
// ProtectedID returns HMAC(machine-id, "execai") — the real machine-id does
// not leak into backend logs, yet stays stable for our use case.
package hostid

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"github.com/denisbrodbeck/machineid"
	"github.com/velesbsdllc/agent-vbai/internal/config"
)

const appSalt = "execai"

// Get returns the host-id. It never returns an empty string (as a last resort
// it synthesizes a random UUID and stores it in the fallback file).
func Get() (string, error) {
	if id, err := machineid.ProtectedID(appSalt); err == nil {
		trimmed := strings.TrimSpace(id)
		if trimmed != "" {
			return trimmed, nil
		}
	}
	return getOrCreateFallback()
}

// getOrCreateFallback reads/creates ~/.config/execai/host_id.
func getOrCreateFallback() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "host_id")
	if data, err := os.ReadFile(path); err == nil {
		id := strings.TrimSpace(string(data))
		if id != "" {
			return id, nil
		}
	}
	// generate 32 hex bytes, the same format as ProtectedID
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	id := hex.EncodeToString(buf)
	if err := os.WriteFile(path, []byte(id), 0o600); err != nil {
		return "", err
	}
	return id, nil
}
