// Стабильный host-id — уникальный ID машины, стабильный между переустановками
// execai. Используется для reuse-логики agent-link'а на бэке: одна физическая
// машина = одна persistent-Session на юзера.
//
// Приоритет источников:
//   1. machineid.ProtectedID("execai") — читает /etc/machine-id (Linux),
//      IOPlatformUUID (macOS), MachineGuid реестра (Windows). Кросс-платформенно.
//   2. Fallback: UUID генерится один раз и хранится в ~/.config/execai/host_id
//      (для случаев когда machine-id недоступен: docker без --host-ipc,
//      Windows Sandbox, порезанные права).
//
// ProtectedID возвращает HMAC(machine-id, "execai") — реальный machine-id
// не утекает в логи бэка, но остаётся стабильным для нашего use case.
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

// Get — вернуть host-id. Никогда не возвращает пустую строку (в крайнем случае
// синтезирует случайный UUID и сохраняет его в fallback-файл).
func Get() (string, error) {
	if id, err := machineid.ProtectedID(appSalt); err == nil {
		trimmed := strings.TrimSpace(id)
		if trimmed != "" {
			return trimmed, nil
		}
	}
	return getOrCreateFallback()
}

// getOrCreateFallback читает/создаёт ~/.config/execai/host_id.
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
	// генерим 32 hex-байта, тот же формат что у ProtectedID
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
