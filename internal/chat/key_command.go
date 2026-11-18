// Команда /key — управление ключом шифрования памяти.
//
// Ключ живёт только у пользователя. У нас его нет, поэтому восстановления не
// существует: об этом надо сказать в момент создания, а не когда память уже
// потеряна.
package chat

import (
	"fmt"
	"strings"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/crypto"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// handleKeyCommand обрабатывает /key, /key show, /key export, /key import <ключ>.
func (m *tuiModel) handleKeyCommand(cmd string) string {
	dir, err := config.Dir()
	if err != nil {
		return i18n.Tf("key.error", err)
	}

	arg := strings.TrimSpace(strings.TrimPrefix(cmd, "/key"))
	switch {
	case arg == "" || arg == "show":
		return m.keyStatus(dir)
	case arg == "export":
		return m.keyExport(dir)
	case strings.HasPrefix(arg, "import "):
		return m.keyImport(dir, strings.TrimSpace(strings.TrimPrefix(arg, "import ")))
	case arg == "new":
		return m.keyNew(dir)
	default:
		return i18n.T("key.usage")
	}
}

// keyStatus — есть ли ключ, и если есть, показать ПУБЛИЧНУЮ часть.
// Приватную не печатаем: она уедет в scrollback, в логи терминала и в историю.
func (m *tuiModel) keyStatus(dir string) string {
	id, err := crypto.LoadIdentity(dir)
	if err != nil {
		if strings.Contains(err.Error(), crypto.ErrNoKey.Error()) {
			return i18n.Tf("key.absent", crypto.KeyPath(dir))
		}
		return i18n.Tf("key.error", err)
	}
	return i18n.Tf("key.present", id.Public().String(), crypto.KeyPath(dir))
}

// keyNew создаёт ключ, если его ещё нет. Существующий не трогаем: перезапись
// означала бы потерю доступа ко всему зашифрованному.
func (m *tuiModel) keyNew(dir string) string {
	id, created, err := crypto.LoadOrGenerate(dir)
	if err != nil {
		return i18n.Tf("key.error", err)
	}
	if !created {
		return i18n.Tf("key.alreadyExists", id.Public().String())
	}
	return i18n.Tf("key.created", id.Public().String(), crypto.KeyPath(dir)) +
		"\n\n" + i18n.T("key.noRecoveryWarning")
}

// keyExport печатает ПРИВАТНЫЙ ключ — единственное место, где это делается.
// Пользователь сам попросил: ему нужно перенести ключ на вторую машину или
// положить в менеджер паролей.
func (m *tuiModel) keyExport(dir string) string {
	id, err := crypto.LoadIdentity(dir)
	if err != nil {
		if strings.Contains(err.Error(), crypto.ErrNoKey.Error()) {
			return i18n.Tf("key.absent", crypto.KeyPath(dir))
		}
		return i18n.Tf("key.error", err)
	}
	return i18n.T("key.exportWarning") + "\n\n" + id.String() + "\n\n" +
		i18n.Tf("key.exportHint", id.Public().String())
}

// keyImport принимает ключ с другой машины.
func (m *tuiModel) keyImport(dir, raw string) string {
	if raw == "" {
		return i18n.T("key.usage")
	}
	id, err := crypto.ParseIdentity(raw)
	if err != nil {
		return i18n.Tf("key.invalid", err)
	}
	if err := crypto.SaveIdentity(dir, id); err != nil {
		// Чаще всего — ключ уже есть. Подсказываем, что делать, вместо
		// молчаливой перезаписи.
		return i18n.Tf("key.importFailed", err, crypto.KeyPath(dir))
	}
	return i18n.Tf("key.imported", id.Public().String())
}

// keyFingerprint — короткая форма публичного ключа для статус-строк.
// Полный age1... длинный и в узкие места не влезает.
func keyFingerprint(pub string) string {
	if len(pub) <= 16 {
		return pub
	}
	return fmt.Sprintf("%s…%s", pub[:10], pub[len(pub)-6:])
}
