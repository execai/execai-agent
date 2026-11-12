package sync

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/crypto"
)

// assertI1 — the guard of invariant I1 (threeway.go): every base machine m
// keeps describes what is on the server now. For each project record that
// describes sync.json's base, every hash of its manifest is the hash of the
// file of that name in the server's pack; a live memory base is the
// server's content of that note; a deletion base is a deletion mark there;
// the settings' manifest is the server's settings, file by file; the stored
// index is the server's index. Content the server never saw is never there.
// A project file left out of the latest exchange keeps the base it had: a
// version the server held earlier.
func assertI1(t *testing.T, stub *stubServer, m *machine) {
	t.Helper()
	m.use()
	key, err := crypto.LoadIdentity(m.dir())
	if err != nil || key == nil {
		return // sync not set up on m
	}
	st, err := LoadState(m.svc.Paths)
	if err != nil || !st.Enabled {
		return
	}
	bs := loadBases(m.svc.Paths)
	// The server's blob at the version a base names: what the server held
	// when this computer agreed it (the server may have moved on since).
	open := func(kind, name string, ver int64) (data []byte, live bool, ok bool) {
		v, found := stub.at(kind, name, ver)
		if !found {
			return nil, false, false
		}
		bl := &v
		if bl.deleted {
			return nil, false, true
		}
		_, _, data, err := unseal(key, bl.content)
		if err != nil {
			t.Fatalf("I1: %s/%s does not open: %v", kind, name, err)
		}
		return data, true, true
	}
	for ws, rec := range bs.Projects {
		b, had := st.Blobs[blobKey(KindProjectMemory, ws)]
		if !had || !rec.describes(b) || rec.Files == nil {
			continue
		}
		data, live, ok := open(KindProjectMemory, ws, b.Version)
		if !ok || !live {
			t.Errorf("I1: project %s has a manifest, the server has no live pack", ws)
			continue
		}
		files, err := unpackFiles(data)
		if err != nil {
			t.Fatal(err)
		}
		for rel, h := range rec.Files {
			if v, ok := files[rel]; ok && hashOf(v) == h {
				continue
			}
			// A name left out of this exchange (cannot be read, a link,
			// too big, a file here and a folder there) keeps the base it
			// had: a version of that file the server held earlier. Any
			// other name's base is what the server holds now.
			if !leftOutName(st, ws, rel) {
				t.Errorf("I1: project %s: the base of %s is not what the server holds", ws, rel)
				continue
			}
			held := false
			for ver := b.Version - 1; ver > 0 && !held; ver-- {
				if old, live, ok := open(KindProjectMemory, ws, ver); ok && live {
					if of, err := unpackFiles(old); err == nil {
						if v, ok := of[rel]; ok && hashOf(v) == h {
							held = true
						}
					}
				}
			}
			if !held {
				t.Errorf("I1: project %s: the base of %s is not what the server holds or held", ws, rel)
			}
		}
	}
	for k, b := range st.Blobs {
		kind, name := splitKey(k)
		if kind != KindMemory {
			continue
		}
		data, live, ok := open(kind, name, b.Version)
		switch {
		case !ok:
			t.Errorf("I1: memory base %s, nothing on the server", name)
		case b.Deleted && live:
			t.Errorf("I1: memory base %s is a deletion, the server has it live", name)
		case !b.Deleted && (!live || hashOf(data) != b.Hash):
			t.Errorf("I1: memory base %s is not what the server holds", name)
		}
	}
	if bs.MemoryIndex != "" {
		idx := blobKey(KindMemory, memoryName(key, "MEMORY.md"))
		if b, had := st.Blobs[idx]; had && !b.Deleted && hashOf([]byte(bs.MemoryIndex)) == b.Hash {
			_, name := splitKey(idx)
			if data, live, ok := open(KindMemory, name, b.Version); !ok || !live || !bytes.Equal(data, []byte(bs.MemoryIndex)) {
				t.Errorf("I1: the stored index is not the server's")
			}
		}
	}
	if b, had := st.Blobs[blobKey(KindConfig, NameConfig)]; had && !b.Deleted && bs.Settings != nil &&
		bs.SettingsVersion == b.Version && bs.SettingsHash == b.Hash {
		data, live, ok := open(KindConfig, NameConfig, b.Version)
		if !ok || !live {
			t.Errorf("I1: settings have a base, the server has none")
		} else {
			files, _ := unpackFiles(data)
			for name, h := range bs.Settings {
				if v, ok := files[name]; !ok || settingHash(name, v) != h {
					t.Errorf("I1: the base of the settings file %s is not what the server holds", name)
				}
			}
		}
	}
}

// leftOutName — rel of the project ws was left out of the latest exchange
// (State.Skipped names it, or a folder above it).
func leftOutName(st *State, ws, rel string) bool {
	dir, ok := st.Projects[ws]
	if !ok {
		return false
	}
	file := filepath.Join(ProjectDir(dir), filepath.FromSlash(rel))
	for _, sk := range st.Skipped {
		if sk.Path == file || strings.HasPrefix(file, sk.Path+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
