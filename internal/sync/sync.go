// Package sync keeps the agent's memory and settings the same on all of a
// person's computers.
//
// What travels: the personal memory (<config>/memory/**, file by file),
// the whole config as one blob — config.json, subscriptions.json with the
// provider keys, permissions.json (decision of the owner 01.11.2026: the
// whole config travels as one opaque encrypted blob, see config_bundle.go)
// — and, per project bound to this machine, its whole <dir>/.execai/ folder
// as one blob (project.go). Every blob is encrypted here, to the person's own key
// (internal/crypto), before it leaves; the server keeps opaque bytes and
// cannot read them. Memory file names are hidden too: the server sees a
// keyed hash, the real name travels inside the ciphertext.
//
// Unencrypted goes only the machine manifest: the NAMES of the sources set
// up here, the active one and the agent version, for the web console.
// Without sync, provider keys can still move machine to machine by hand
// with a one-time transfer code (keys.go), no server and no plan involved.
//
// The key reaches a second computer wrapped with the sync password (age
// scrypt, identity.go). The server keeps the wrapped key and never sees the
// password; a forgotten password cannot be recovered.
//
// Sync comes with the Work and Max plans (plan.FeatureSync, gate.go).
package sync

import "time"

// Blob kinds, as the server knows them.
const (
	KindMemory        = "memory"
	KindProjectMemory = "project-memory"
	KindConfig        = "config"
)

// NameConfig — the one blob of kind config.
const NameConfig = "config"

// MaxBlobBytes — the server refuses larger blobs (413); a project blob may
// be larger (MaxProjectBlobBytes).
const MaxBlobBytes = 256 << 10

// Timing of the background runner.
const (
	// PullEvery — how often a running agent asks the server for news.
	PullEvery = 10 * time.Minute
	// PushDelay — a local change is sent after this much quiet, so a burst
	// of edits becomes one upload.
	PushDelay = 10 * time.Second
	// WatchEvery — how often the local files are looked at (stat only).
	WatchEvery = 3 * time.Second
	// ExitPushTimeout bounds the last upload when the agent exits.
	ExitPushTimeout = 5 * time.Second
	// ManifestEvery — the machine manifest is re-sent at least this often,
	// so the web shows when this machine was last seen.
	ManifestEvery = 6 * time.Hour
)

func knownKind(kind string) bool {
	switch kind {
	case KindMemory, KindProjectMemory, KindConfig:
		return true
	}
	return false
}

// blobKey — the key of a blob in State.Blobs and in local item maps.
func blobKey(kind, name string) string { return kind + "/" + name }

// splitKey reverses blobKey.
func splitKey(k string) (kind, name string) {
	for i := 0; i < len(k); i++ {
		if k[i] == '/' {
			return k[:i], k[i+1:]
		}
	}
	return k, ""
}
