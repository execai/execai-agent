package sync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// The base, file by file, and which folder it belongs to.
//
// A base says what this computer last agreed with the server. sync.json
// keeps it per blob (State.Blobs: a memory file, the settings, a project's
// whole pack). Decisions are taken file by file (threeway.go), so for a
// project the base must be known per file too: its manifest — relative
// path → sha256 of the content agreed — lives here, in sync-bases.json, with
// the few earlier hashes of each file (to tell an older copy put back from
// an edit) and the folder the base was agreed in.
//
// sync-bases.json, not sync.json: older agents — 7.17 and before, and the
// one built into ExecAI Studio, which shares the config folder — rewrite
// sync.json without the fields they do not know, and never touch other
// files. Each project record carries the pack base it was written with
// (version, hash); when sync.json holds another one, an older agent agreed
// it since, in the folder sync.json named then (State.Projects as found at
// the start of the exchange) — that folder becomes the record, with no
// identity and no manifest known. A missing manifest is rebuilt when one
// side is the base as a whole: a folder whose pack hash is the base's has
// the base's files, and so has a server pack with the base's hash.
//
// A record is written only once the files it describes are in place and,
// if something was sent, the server took it (projectsync.go): an exchange
// that stops anywhere leaves the previous record, and the next exchange
// comes to the same result from what is on disk.

// basesVersion — the format of sync-bases.json this agent writes. A file
// of a later format is read and never written (Bases.readOnly).
const basesVersion = 3

// prevKeep — earlier base hashes kept per file: enough to know a return to
// an earlier version (threeway.go, row 2b) or an older copy put back.
const prevKeep = 64

// BasesFile — sync-bases.json.
func (p Paths) BasesFile() string { return filepath.Join(p.ConfigDir, "sync-bases.json") }

// Bases — sync-bases.json.
type Bases struct {
	V int `json:"v"`
	// Projects — workspace id → its base, file by file, and the folder.
	Projects map[string]ProjectBase `json:"projects,omitempty"`
	// Memory — the identity of the personal memory folder its bases were
	// last agreed in; "" — not known.
	Memory string `json:"memory,omitempty"`
	// MemoryPrev — memory blob key → earlier base hashes of that file,
	// newest first (the last one before a deletion included).
	MemoryPrev map[string][]string `json:"memory_prev,omitempty"`
	// MemoryIndex — the content of the memory index as last agreed with
	// the server: the index is merged entry by entry (memindex.go), which
	// needs the base's lines, not only its hash.
	MemoryIndex string `json:"memory_index,omitempty"`
	// Settings — the settings files as last agreed, file by file: name →
	// hash of its content as it travels (config.json without what stays
	// this computer's own); SettingsVersion and SettingsHash — the
	// settings blob that agreement was (configsync.go); SettingsPrev — the
	// earlier hashes of each file.
	Settings        map[string]string   `json:"settings,omitempty"`
	SettingsVersion int64               `json:"settings_version,omitempty"`
	SettingsHash    string              `json:"settings_hash,omitempty"`
	SettingsPrev    map[string][]string `json:"settings_prev,omitempty"`
	// SettingsModel — selected_model_id of the agreed config.json ("" —
	// none); nil — not known (recorded by an earlier agent). A model the
	// agent picked here by itself travels as this one (configScan).
	SettingsModel *string `json:"settings_model,omitempty"`
	// readOnly — written by a newer agent: read, never written.
	readOnly bool
}

// ProjectBase — one project's base: the pack, its files and the folder.
type ProjectBase struct {
	Dir string `json:"dir"`
	// ID — dirIdentity of <Dir>/.execai; "" — not known (an older agent's
	// base, or this file system cannot tell).
	ID      string `json:"id,omitempty"`
	Version int64  `json:"version"`
	Hash    string `json:"hash,omitempty"`
	Deleted bool   `json:"deleted,omitempty"`
	// Files — the manifest: relative path → sha256 of the content agreed;
	// nil — not known. An empty manifest is written as one ({}), not left
	// out: files forgotten before they are written back (forgetFiles) must
	// stay without a base, not be rebuilt from the server's pack.
	Files map[string]string `json:"files"`
	// Prev — relative path → earlier base hashes of that file, newest
	// first.
	Prev map[string][]string `json:"prev,omitempty"`
	// LastLive — while the project is marked deleted on the server (no
	// base, Files nil): the manifest of its last live base, to tell the
	// very files deleted elsewhere from content the server never saw
	// (projectsync.go, 5d). Never a base itself.
	LastLive map[string]string `json:"last_live,omitempty"`
}

func (r ProjectBase) describes(b BlobState) bool {
	return r.Version == b.Version && r.Hash == b.Hash && r.Deleted == b.Deleted
}

// loadBases reads sync-bases.json. Missing, empty or broken — no records
// (bases are rebuilt where they can be). Format 1 (the earlier candidate,
// with its applying marks) is read as it is; its marks are not needed any
// more. A later format is read but never written.
func loadBases(p Paths) *Bases {
	b := &Bases{}
	if data, err := os.ReadFile(p.BasesFile()); err == nil {
		if json.Unmarshal(data, b) != nil {
			b = &Bases{}
		}
	}
	if b.V > basesVersion {
		b.readOnly = true
	}
	if b.Projects == nil {
		b.Projects = map[string]ProjectBase{}
	}
	if b.V < 3 {
		// The earlier candidates wrote the manifest of the files kept here
		// under a deletion mark: content the server may never have seen. It
		// is not a base (I1, threeway.go) — nor known to be the last live one.
		for ws, rec := range b.Projects {
			if rec.Deleted {
				rec.Files, rec.LastLive = nil, nil
				b.Projects[ws] = rec
			}
		}
	}
	if b.MemoryPrev == nil {
		b.MemoryPrev = map[string][]string{}
	}
	return b
}

// saveBases writes sync-bases.json atomically, mode 0600 — not a file a
// newer agent wrote.
func saveBases(p Paths, b *Bases) error {
	if b.readOnly {
		return nil
	}
	b.V = basesVersion
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(p.BasesFile(), data)
}

// resetBases forgets every record, with State.reset (a newer agent's file
// stays as it is).
func resetBases(p Paths) error {
	if loadBases(p).readOnly {
		return nil
	}
	return saveBases(p, &Bases{})
}

// sameIdentity — two identities of one folder. Identities written before
// this format carried the device number first (dev:ino:btime, or
// maj.min:ino:btime on Linux); it is not part of the comparison — it
// changes across reboots on btrfs subvolumes, ZFS datasets and with the
// order disks are found in — so «device, number, creation time» and
// «number, creation time» match when the last two do.
func sameIdentity(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return identityCore(a) == identityCore(b)
}

func identityCore(id string) string {
	if osName == "windows" {
		return id
	}
	if parts := strings.Split(id, ":"); len(parts) == 3 {
		return parts[1] + ":" + parts[2]
	}
	return id
}

// projectTrust — how far the folder dir is the one project ws's base was
// agreed in (threeway.go), and the record of that base. A base the records
// do not describe was agreed by an older agent: it gets a record of the
// folder prior (sync.json's at the start) named, with no identity and no
// manifest, keeping the identity known for that folder.
func (b *Bases) projectTrust(ws, dir string, base BlobState, prior map[string]string) (trustLevel, ProjectBase) {
	rec, ok := b.Projects[ws]
	if !ok || !rec.describes(base) {
		legacy := ProjectBase{Dir: prior[ws], Version: base.Version, Hash: base.Hash, Deleted: base.Deleted, Prev: rec.Prev}
		if ok && legacy.Dir != "" && samePath(rec.Dir, legacy.Dir) {
			legacy.ID = rec.ID // what is known of that folder still holds
		}
		b.Projects[ws] = legacy
		rec = legacy
	}
	cur := dirIdentity(ProjectDir(dir))
	if rec.Dir == "" || !samePath(rec.Dir, dir) {
		if rec.Dir != "" && cur != "" && rec.ID != "" && sameIdentity(rec.ID, cur) {
			// The folder was moved (mv keeps the number and the creation
			// time): the same folder at a new path. The record takes the
			// new path at the end of the exchange (noteProject).
			return trustFull, rec
		}
		return trustNone, rec
	}
	switch {
	case cur == "" && rec.ID != "":
		// An identity was known and cannot be read now: something else is
		// mounted or linked here.
		return trustNone, rec
	case cur == "" || rec.ID == "":
		// This file system cannot tell folders apart (an NFS, sshfs or WSL
		// /mnt mount), or an older agent's base: the path decides, but a
		// missing file is not a deletion and an older copy is not an edit.
		if fi, err := os.Lstat(ProjectDir(dir)); err != nil || !fi.IsDir() {
			return trustNone, rec
		}
		return trustPath, rec
	case sameIdentity(rec.ID, cur):
		return trustFull, rec
	}
	return trustNone, rec
}

// manifestFor — the base of a project file by file: the record's, or
// rebuilt when a side is the base as a whole; nil — not known.
func manifestFor(rec ProjectBase, base BlobState, had bool, local, server map[string][]byte, localHash, serverHash string) map[string]string {
	if !had || base.Hash == "" {
		return nil
	}
	if rec.Files != nil && rec.describes(base) {
		return rec.Files
	}
	switch {
	case local != nil && localHash == base.Hash:
		return hashFiles(local)
	case server != nil && serverHash == base.Hash:
		return hashFiles(server)
	}
	return nil
}

func hashFiles(files map[string][]byte) map[string]string {
	out := make(map[string]string, len(files))
	for rel, data := range files {
		out[rel] = hashOf(data)
	}
	return out
}

// pushPrev puts h first in an earlier-hashes list (no repeats, prevKeep
// at most).
func pushPrev(list []string, h string) []string {
	if h == "" {
		return list
	}
	out := []string{h}
	for _, x := range list {
		if x != h && len(out) < prevKeep {
			out = append(out, x)
		}
	}
	return out
}

func inList(list []string, h string) bool {
	for _, x := range list {
		if x == h {
			return true
		}
	}
	return false
}

// noteProject records project ws's base after an exchange that agreed it:
// the pack, its manifest, the folder; what the previous manifest had that
// the new one does not is kept among the earlier hashes.
func (b *Bases) noteProject(ws, dir string, base BlobState, files map[string]string) {
	rec := b.Projects[ws]
	prev := rec.Prev
	if prev == nil {
		prev = map[string][]string{}
	}
	for rel, h := range rec.Files {
		if files[rel] != h {
			prev[rel] = pushPrev(prev[rel], h)
		}
	}
	nr := ProjectBase{Dir: dir, ID: dirIdentity(ProjectDir(dir)), Version: base.Version, Hash: base.Hash,
		Deleted: base.Deleted, Files: files, Prev: prev}
	if base.Deleted {
		// Marked deleted: no base. The last live manifest is kept apart.
		nr.Files = nil
		nr.LastLive = rec.LastLive
		if !rec.Deleted && rec.Files != nil {
			nr.LastLive = rec.Files
		}
	}
	b.Projects[ws] = nr
}

// forgetFiles drops rels from the manifest of ws's record before the
// server's versions of them are written back here because their absence
// was not taken for a deletion: an exchange stopped half way must find
// them without a base, not as files deleted here (projectsync.go).
func (b *Bases) forgetFiles(ws string, rec ProjectBase, files map[string]string, rels []string) {
	kept := map[string]string{}
	for rel, h := range files {
		kept[rel] = h
	}
	for _, rel := range rels {
		delete(kept, rel)
	}
	rec.Files = kept
	b.Projects[ws] = rec
}

// memoryTrust — how far the personal memory folder is the one its bases
// were agreed in: the same identity as recorded; the path alone where the
// identity cannot be read or none was recorded (it is always
// <config>/memory); another folder otherwise.
func (b *Bases) memoryTrust(memDir string) trustLevel {
	cur := dirIdentity(memDir)
	switch {
	case cur == "" || b.Memory == "":
		return trustPath
	case sameIdentity(b.Memory, cur):
		return trustFull
	}
	return trustNone
}

// SameFolder — a and b are one folder: by identity where both can be read,
// by path otherwise (one folder holds one project, projectsync.go).
func SameFolder(a, b string) bool {
	ia, ib := dirIdentity(a), dirIdentity(b)
	if ia != "" && ib != "" {
		return sameIdentity(ia, ib)
	}
	return samePath(a, b)
}
