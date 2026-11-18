// Package writelock — the short lock shared by everything in the agent that
// writes into the folders sync carries between computers: the personal
// memory (<config>/memory) and a project's .execai/.
//
// Sync replaces a file there only after it compared what is on disk with
// what it decided on (internal/sync, keeper.go). Between that comparison
// and the replacement another writer of the agent — the model saving
// EXECAI.md through the write or edit tool, the memory index template,
// /project memory, an import — could add a line that the replacement then
// throws away. Both sides take this lock for just that moment: the keeper
// for «read again, compare, rename», a writer for its own read and write.
// Nobody holds it across the network, so a writer never waits for an
// exchange (sync.lock is the exchange lock and is held for all of it).
//
// The lock is an OS file lock on <config>/write.lock: flock on Unix,
// LockFileEx on Windows; the kernel drops it when its holder ends, however
// it ends. A file system that cannot lock at all does not stop anyone: the
// write goes on unlocked, as before the lock existed. Writers that are not
// the agent's (an editor, a script) do not take it; what that leaves open
// is described in keeper.go.
package writelock

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FileName — the lock file in the config directory.
const FileName = "write.lock"

const (
	// wait — how long a take waits for another holder. Holders keep it for
	// a read and a write of one file; a holder that does not let go within
	// this time is stuck, and the write goes on rather than hang the agent.
	wait = 5 * time.Second
	// retry — how often a waiting take tries again.
	retry = 2 * time.Millisecond
)

// Hold takes the lock of the config directory configDir and returns the
// function that lets it go. It never fails: when the lock cannot be taken
// (no such folder can be made, the file system cannot lock, a holder that
// does not let go) the caller goes on without it.
func Hold(configDir string) (release func()) {
	if configDir == "" {
		return func() {}
	}
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return func() {}
	}
	f, err := os.OpenFile(filepath.Join(configDir, FileName), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return func() {}
	}
	deadline := time.Now().Add(wait)
	for {
		ok, err := tryLock(f)
		switch {
		case err != nil:
			_ = f.Close()
			return func() {}
		case ok:
			return func() {
				unlock(f)
				_ = f.Close()
			}
		}
		if !time.Now().Before(deadline) {
			_ = f.Close()
			return func() {}
		}
		time.Sleep(retry)
	}
}

// configDir — the agent's config directory, as config.Dir names it
// (os.UserConfigDir()/execai), without making it.
func configDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "execai")
}

// Synced reports whether path lies where sync carries files between
// computers: the personal memory folder of this config directory, or
// inside a folder named .execai.
func Synced(path string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	if dir := configDir(); dir != "" {
		mem := filepath.Join(dir, "memory")
		if abs == mem || strings.HasPrefix(abs, mem+string(filepath.Separator)) {
			return true
		}
	}
	for _, part := range strings.Split(filepath.ToSlash(abs), "/") {
		if part == ".execai" {
			return true
		}
	}
	return false
}

// ForPath takes the lock when path is a synced place (Synced) and does
// nothing otherwise.
func ForPath(path string) (release func()) {
	if !Synced(path) {
		return func() {}
	}
	return Hold(configDir())
}

// WriteFile writes data to path as os.WriteFile does, holding the lock when
// path is a synced place.
func WriteFile(path string, data []byte, perm os.FileMode) error {
	release := ForPath(path)
	defer release()
	return os.WriteFile(path, data, perm)
}

// Update reads path, lets change make the new content from what it held
// (nil when the file is not there) and writes it — all under the lock when
// path is a synced place, so a replacement by sync cannot fall between the
// read and the write. When change returns an error nothing is written.
func Update(path string, perm os.FileMode, change func(old []byte) ([]byte, error)) error {
	release := ForPath(path)
	defer release()
	old, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	data, err := change(old)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, perm)
}
