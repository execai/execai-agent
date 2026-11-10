package sync

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path"
	"strings"

	"github.com/velesbsdllc/agent-vbai/internal/crypto"
)

// envelopeMagic opens every plaintext before encryption:
//
//	execai-sync/1\n<kind>\n<path>\n<file bytes>
//
// Kind and path inside the ciphertext bind the content to its place: the
// server cannot hand the contents of one file back under another name.
const envelopeMagic = "execai-sync/1"

var errEnvelope = errors.New("sync: not a sync file of this key")

// seal encrypts one file to the key.
func seal(id *crypto.Identity, kind, p string, data []byte) ([]byte, error) {
	if strings.ContainsAny(kind+p, "\n\r") {
		return nil, errEnvelope
	}
	var b bytes.Buffer
	b.Grow(len(envelopeMagic) + len(kind) + len(p) + len(data) + 3)
	b.WriteString(envelopeMagic + "\n" + kind + "\n" + p + "\n")
	b.Write(data)
	return crypto.Encrypt(b.Bytes(), id.Public())
}

// unseal decrypts and splits a file.
func unseal(id *crypto.Identity, ct []byte) (kind, p string, data []byte, err error) {
	plain, err := crypto.Decrypt(ct, id)
	if err != nil {
		return "", "", nil, err
	}
	parts := bytes.SplitN(plain, []byte("\n"), 4)
	if len(parts) != 4 || string(parts[0]) != envelopeMagic {
		return "", "", nil, errEnvelope
	}
	return string(parts[1]), string(parts[2]), parts[3], nil
}

// memoryName — the server-side name of a memory file: a keyed hash of its
// path, so the server does not learn what the notes are called.
// Every machine with the same key computes the same name.
func memoryName(id *crypto.Identity, rel string) string {
	k := sha256.Sum256([]byte("execai-sync-names/1\n" + id.String()))
	m := hmac.New(sha256.New, k[:])
	m.Write([]byte(rel))
	return "m" + hex.EncodeToString(m.Sum(nil))[:40]
}

// safeRel reports whether rel is a clean relative slash path that stays
// inside its directory.
func safeRel(rel string) bool {
	if rel == "" || strings.HasPrefix(rel, "/") || strings.Contains(rel, "\\") || strings.Contains(rel, ":") {
		return false
	}
	if path.Clean(rel) != rel {
		return false
	}
	for _, part := range strings.Split(rel, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

// validName — what the server accepts as a blob name.
func validName(n string) bool {
	if len(n) == 0 || len(n) > 120 {
		return false
	}
	for i := 0; i < len(n); i++ {
		c := n[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}
