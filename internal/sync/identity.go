package sync

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"filippo.io/age"

	"github.com/velesbsdllc/agent-vbai/internal/crypto"
)

// ScryptWorkFactor — log2 of the scrypt cost for the sync password and the
// key transfer phrase: 18 is about 256 MiB and a second per try — expensive
// for anyone guessing, still fine on a weak laptop (20 needed 1 GiB and ran
// out of memory there). The factor is written into each wrapped blob, so
// blobs made with another factor (up to age's limit of 22) still open.
// Tests lower it.
var ScryptWorkFactor = 18

// MinPasswordLen — the shortest sync password accepted, in characters.
const MinPasswordLen = 10

// identityMagic opens the wrapped key: "<magic> <key fingerprint>\n" and
// then the age scrypt ciphertext of the private key. The fingerprint lets a
// machine that already holds a key tell, without the password, whether the
// server's key is the same one. It is a hash of the PUBLIC key, never the
// public key itself: without the public key nobody, the server included,
// can make a file this key would accept.
const identityMagic = "execai-identity/1"

// ErrBadPassword — the password does not open the wrapped key. Deliberately
// without details: the person needs one message, «the password did not fit».
var ErrBadPassword = errors.New("sync: wrong password")

// KeyFingerprint — a short hash of the public key.
func KeyFingerprint(id *crypto.Identity) string {
	sum := sha256.Sum256([]byte("execai-key-fp/1\n" + id.Public().String()))
	return hex.EncodeToString(sum[:8])
}

// WrapIdentity encrypts the private key with the sync password.
func WrapIdentity(id *crypto.Identity, password string) ([]byte, error) {
	r, err := age.NewScryptRecipient(password)
	if err != nil {
		return nil, err
	}
	r.SetWorkFactor(ScryptWorkFactor)
	var out bytes.Buffer
	out.WriteString(identityMagic + " " + KeyFingerprint(id) + "\n")
	w, err := age.Encrypt(&out, r)
	if err != nil {
		return nil, err
	}
	if _, err := io.WriteString(w, id.String()); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// splitWrapped separates the header from the ciphertext.
func splitWrapped(wrapped []byte) (fp string, ct []byte, ok bool) {
	i := bytes.IndexByte(wrapped, '\n')
	if i < 0 {
		return "", nil, false
	}
	head := strings.Fields(string(wrapped[:i]))
	if len(head) != 2 || head[0] != identityMagic {
		return "", nil, false
	}
	return head[1], wrapped[i+1:], true
}

// WrappedFingerprint — the key fingerprint written in a wrapped key, "" when
// the blob is not one of ours.
func WrappedFingerprint(wrapped []byte) string {
	fp, _, _ := splitWrapped(wrapped)
	return fp
}

// UnwrapIdentity opens the wrapped key with the password. Any failure is
// ErrBadPassword.
func UnwrapIdentity(wrapped []byte, password string) (*crypto.Identity, error) {
	fp, ct, ok := splitWrapped(wrapped)
	if !ok {
		return nil, ErrBadPassword
	}
	sid, err := age.NewScryptIdentity(password)
	if err != nil {
		return nil, ErrBadPassword
	}
	r, err := age.Decrypt(bytes.NewReader(ct), sid)
	if err != nil {
		return nil, ErrBadPassword
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, ErrBadPassword
	}
	id, err := crypto.ParseIdentity(string(data))
	if err != nil || KeyFingerprint(id) != fp {
		return nil, ErrBadPassword
	}
	return id, nil
}

// PasswordTooShort reports whether a new password is under MinPasswordLen.
func PasswordTooShort(p string) bool { return utf8.RuneCountInString(p) < MinPasswordLen }

// loadKey reads this machine's key; nil without an error when there is none.
func loadKey(dir string) (*crypto.Identity, error) {
	id, err := crypto.LoadIdentity(dir)
	if errors.Is(err, crypto.ErrNoKey) {
		return nil, nil
	}
	return id, err
}

// replaceKey puts id in place of this machine's key. The old key is not
// lost: it is moved next to it as sync.key.old-<time> (0600), because files
// encrypted with it can only be opened with it.
func replaceKey(dir string, id *crypto.Identity, now time.Time) (backup string, err error) {
	path := crypto.KeyPath(dir)
	if _, err := os.Stat(path); err == nil {
		backup = fmt.Sprintf("%s.old-%s", path, now.Format("20060102-150405"))
		if err := os.Rename(path, backup); err != nil {
			return "", err
		}
		_ = os.Chmod(backup, 0o600)
	}
	if err := crypto.SaveIdentity(dir, id); err != nil {
		return backup, err
	}
	return backup, nil
}
