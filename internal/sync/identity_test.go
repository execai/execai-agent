package sync

import (
	"bytes"
	"errors"
	"os"
	"testing"
)

func TestIdentity_WrapUnwrap(t *testing.T) {
	id := mustKey(t)
	wrapped, err := WrapIdentity(id, "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(wrapped, []byte(id.String())) || bytes.Contains(wrapped, []byte(id.Public().String())) {
		t.Fatal("the key or its public part is readable in the wrapped blob")
	}
	if WrappedFingerprint(wrapped) != KeyFingerprint(id) {
		t.Fatal("fingerprint header")
	}
	got, err := UnwrapIdentity(wrapped, "correct horse battery")
	if err != nil || got.String() != id.String() {
		t.Fatalf("unwrap: %v", err)
	}
	if _, err := UnwrapIdentity(wrapped, "wrong password"); !errors.Is(err, ErrBadPassword) {
		t.Fatalf("wrong password: %v", err)
	}
	// A header claiming another key is refused, not trusted.
	other := mustKey(t)
	forged := append([]byte(identityMagic+" "+KeyFingerprint(other)+"\n"), wrapped[bytes.IndexByte(wrapped, '\n')+1:]...)
	if _, err := UnwrapIdentity(forged, "correct horse battery"); !errors.Is(err, ErrBadPassword) {
		t.Fatalf("forged header accepted: %v", err)
	}
	if _, err := UnwrapIdentity([]byte("garbage"), "x"); !errors.Is(err, ErrBadPassword) {
		t.Fatalf("garbage: %v", err)
	}
}

func TestIdentity_PasswordLength(t *testing.T) {
	if !PasswordTooShort("123456789") || PasswordTooShort("пароль1234") {
		t.Fatal("length is counted in characters, at least 10")
	}
}

func TestPhrase(t *testing.T) {
	seen := map[string]bool{}
	for _, w := range phraseWords {
		if seen[w] {
			t.Fatalf("duplicate word %q", w)
		}
		seen[w] = true
	}
	if len(phraseWords) < 512 {
		t.Fatalf("only %d words", len(phraseWords))
	}
	p, err := newPhrase(PhraseWords)
	if err != nil || len(bytes.Fields([]byte(p))) != PhraseWords {
		t.Fatalf("phrase %q: %v", p, err)
	}
	if normalizePhrase("  Apple-PEAR\tplum ") != "apple pear plum" {
		t.Fatal("normalize")
	}
}

// TestIdentity_DefaultAndOlderFactor — a key wrapped with factor 20 by the
// first builds (testdata, a throwaway test key) still opens with the
// default now 18: the factor travels inside the wrapped blob. One unwrap
// at 2^20 needs ~1 GiB, so not in -short runs.
func TestIdentity_DefaultAndOlderFactor(t *testing.T) {
	if testing.Short() {
		t.Skip("scrypt 2^20 needs ~1 GiB")
	}
	old, err := os.ReadFile("testdata/identity-wf20.bin")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(old, []byte(" 20\n")) {
		t.Fatal("fixture is not a factor-20 wrap")
	}
	id, err := UnwrapIdentity(old, "correct horse battery")
	if err != nil || KeyFingerprint(id) != WrappedFingerprint(old) {
		t.Fatalf("factor-20 wrap does not open: %v", err)
	}
}

func TestIdentity_DefaultFactorIs18(t *testing.T) {
	// TestMain lowers it for speed; the shipped default is in the source.
	src, err := os.ReadFile("identity.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(src, []byte("var ScryptWorkFactor = 18\n")) {
		t.Fatal("the default scrypt factor must be 18")
	}
}
