// Package crypto — шифрование памяти агента ключом пользователя.
//
// Модель — «на получателей», а не «под паролем». Это решение принято до первой
// строчки кода и стоит объяснения: память привязана к проекту, а в проект
// когда-нибудь пригласят второго человека. Пароль в таком мире означает общий
// секрет на всех, и разграничить доступ уже нечем. Схема age (X25519 +
// ChaCha20-Poly1305) шифрует блоб сразу на N публичных ключей, каждый читает
// своим приватным — приглашение сводится к перешифровке записи на двоих, без
// обмена секретами.
//
// Ключ живёт только у пользователя: ~/.config/execai/sync.key, режим 0600.
// У нас его нет и не будет, поэтому восстановления не существует — потеря
// ключа означает сбор памяти заново (для этого есть импорт из CLAUDE.md и
// подобных файлов, см. этап 2 плана).
package crypto

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"filippo.io/age"
)

// KeyFileName — имя файла с приватным ключом внутри каталога конфига.
const KeyFileName = "sync.key"

var (
	// ErrNoKey — ключа ещё нет. Не ошибка сама по себе: вызывающий решает,
	// сгенерировать новый или сообщить пользователю.
	ErrNoKey = errors.New("ключ шифрования не найден")
	// ErrNotForYou — блоб зашифрован не на наш ключ. Самый частый случай —
	// чужая запись или ключ, замененный после потери.
	ErrNotForYou = errors.New("этот блоб зашифрован не на ваш ключ")
)

// Identity — приватный ключ пользователя вместе с его публичной частью.
type Identity struct {
	id *age.X25519Identity
}

// Recipient — публичный ключ: кому шифруем. Свой собственный, участника
// проекта или второй машины того же человека.
type Recipient struct {
	r *age.X25519Recipient
}

// Generate создаёт новую пару ключей. Приватная часть остаётся у пользователя.
func Generate() (*Identity, error) {
	id, err := age.GenerateX25519Identity()
	if err != nil {
		return nil, fmt.Errorf("генерация ключа: %w", err)
	}
	return &Identity{id: id}, nil
}

// Public — публичный ключ этой личности (age1...). Его можно спокойно
// показывать и передавать: он нужен, чтобы шифровать ДЛЯ нас.
func (i *Identity) Public() *Recipient {
	return &Recipient{r: i.id.Recipient()}
}

// String — приватный ключ в текстовом виде (AGE-SECRET-KEY-1...).
// Это и есть тот самый секрет: показывать один раз при создании, дальше
// хранить в менеджере паролей.
func (i *Identity) String() string { return i.id.String() }

// String — публичный ключ в текстовом виде (age1...).
func (r *Recipient) String() string { return r.r.String() }

// ParseIdentity читает приватный ключ из текста.
func ParseIdentity(s string) (*Identity, error) {
	id, err := age.ParseX25519Identity(strings.TrimSpace(s))
	if err != nil {
		return nil, fmt.Errorf("некорректный приватный ключ: %w", err)
	}
	return &Identity{id: id}, nil
}

// ParseRecipient читает публичный ключ из текста.
func ParseRecipient(s string) (*Recipient, error) {
	r, err := age.ParseX25519Recipient(strings.TrimSpace(s))
	if err != nil {
		return nil, fmt.Errorf("некорректный публичный ключ: %w", err)
	}
	return &Recipient{r: r}, nil
}

// Encrypt шифрует данные на перечисленных получателей. Каждый из них сможет
// расшифровать своим приватным ключом; никто другой — нет.
//
// Свой ключ включать в список обязан вызывающий: забыть себя и потерять доступ
// к собственной записи — ошибка, которую тут поймать нельзя, поэтому пустой
// список отвергается явно.
func Encrypt(plaintext []byte, to ...*Recipient) ([]byte, error) {
	if len(to) == 0 {
		return nil, errors.New("не указан ни один получатель — расшифровать блоб будет некому")
	}
	recips := make([]age.Recipient, 0, len(to))
	for _, r := range to {
		if r == nil || r.r == nil {
			return nil, errors.New("пустой получатель в списке")
		}
		recips = append(recips, r.r)
	}

	var out bytes.Buffer
	w, err := age.Encrypt(&out, recips...)
	if err != nil {
		return nil, fmt.Errorf("шифрование: %w", err)
	}
	if _, err := w.Write(plaintext); err != nil {
		return nil, fmt.Errorf("запись данных: %w", err)
	}
	// Close досыпает финальный блок и MAC — без него блоб не расшифруется.
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("финализация: %w", err)
	}
	return out.Bytes(), nil
}

// Decrypt расшифровывает блоб нашим приватным ключом.
func Decrypt(ciphertext []byte, id *Identity) ([]byte, error) {
	if id == nil || id.id == nil {
		return nil, ErrNoKey
	}
	r, err := age.Decrypt(bytes.NewReader(ciphertext), id.id)
	if err != nil {
		// age не различает «не наш ключ» и «битые данные» в типе ошибки,
		// но для пользователя это разные истории, поэтому подсказываем.
		return nil, fmt.Errorf("%w (или блоб повреждён): %v", ErrNotForYou, err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("чтение расшифрованных данных: %w", err)
	}
	return out, nil
}

// KeyPath — путь к файлу ключа.
func KeyPath(configDir string) string {
	return filepath.Join(configDir, KeyFileName)
}

// LoadIdentity читает ключ с диска. Возвращает ErrNoKey, если файла нет.
func LoadIdentity(configDir string) (*Identity, error) {
	data, err := os.ReadFile(KeyPath(configDir))
	if os.IsNotExist(err) {
		return nil, ErrNoKey
	}
	if err != nil {
		return nil, fmt.Errorf("чтение ключа: %w", err)
	}
	return ParseIdentity(string(data))
}

// SaveIdentity пишет ключ на диск с режимом 0600.
//
// Существующий файл НЕ перезаписывается: перезапись ключа означает потерю
// доступа ко всему, что им зашифровано, и молча делать это нельзя.
func SaveIdentity(configDir string, id *Identity) error {
	path := KeyPath(configDir)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("ключ уже существует: %s — перезапись означала бы потерю доступа к зашифрованным данным", path)
	}
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return fmt.Errorf("создание каталога конфига: %w", err)
	}
	// 0600: ключ читает только владелец.
	if err := os.WriteFile(path, []byte(id.String()+"\n"), 0o600); err != nil {
		return fmt.Errorf("запись ключа: %w", err)
	}
	return nil
}

// LoadOrGenerate возвращает существующий ключ либо создаёт новый и сохраняет.
// Второе значение — true, если ключ был только что создан: вызывающий обязан
// в этом случае показать пользователю предупреждение о том, что
// восстановления не существует.
func LoadOrGenerate(configDir string) (*Identity, bool, error) {
	id, err := LoadIdentity(configDir)
	switch {
	case err == nil:
		return id, false, nil
	case !errors.Is(err, ErrNoKey):
		return nil, false, err
	}
	id, err = Generate()
	if err != nil {
		return nil, false, err
	}
	if err := SaveIdentity(configDir, id); err != nil {
		return nil, false, err
	}
	return id, true, nil
}
