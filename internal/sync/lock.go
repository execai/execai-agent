package sync

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// One exchange at a time per config directory, across processes. The chat,
// `execai serve` and `execai ide` each run their own background sync over
// the same files: sync.json, the memory, the bound projects. Two exchanges
// at once would each save the state they loaded at the start, and the later
// save undoes the earlier one — a /sync off typed in the chat comes back on
// after the daemon's exchange, versions agreed with the server go back and
// the next exchange sees conflicts that are not there.
//
// The lock is an OS file lock on <config>/sync.lock (flock on Unix,
// LockFileEx on Windows), not a pid file like the inbox's serve.pid: that
// one marks a long-lived owner and is checked by asking whether its pid is
// alive, while this one is taken for a few seconds many times an hour. The
// kernel drops a file lock when its process ends, however it ends, so a
// crash never leaves sync blocked and a pid reused after a reboot cannot
// pass for the holder. The file itself stays; it is never removed —
// removing a locked file would let a third process lock a new file of the
// same name while a second one waits on the old.
//
// Within one process the lock also separates two Services: each take opens
// the file anew, and both kinds of lock are per open file, not per process.

const (
	// lockWaitBackground — how long a background exchange waits for another
	// process's one before it skips this round (the runner tries again a
	// little later; nothing is shown to the person).
	lockWaitBackground = 3 * time.Second
	// lockWaitCommand — how long a command the person typed (/sync now, on,
	// off, forget, a project just bound) waits before it says sync is busy.
	lockWaitCommand = 30 * time.Second
	// lockRetry — how often a waiting take tries again.
	lockRetry = 50 * time.Millisecond
)

// errBusy — another process holds the exchange lock longer than we waited.
var errBusy = errors.New("sync: another ExecAI process on this computer is syncing")

// LockFile — sync.lock, the exchange lock of this config directory.
func (p Paths) LockFile() string { return filepath.Join(p.ConfigDir, "sync.lock") }

// acquireExchange takes the exchange lock at path, waiting up to wait (and
// no longer than ctx allows). errBusy when it stays taken. A file system
// that cannot lock at all (some network mounts) does not stop sync: the
// exchange goes on unlocked, as it did before the lock existed.
func acquireExchange(ctx context.Context, path string, wait time.Duration) (release func(), err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(wait)
	for {
		ok, err := tryLockFile(f)
		switch {
		case err != nil:
			_ = f.Close()
			return func() {}, nil
		case ok:
			return func() {
				unlockFile(f)
				_ = f.Close()
			}, nil
		}
		if !time.Now().Before(deadline) {
			_ = f.Close()
			return nil, errBusy
		}
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, errBusy
		case <-time.After(lockRetry):
		}
	}
}

// lock takes the Service for one critical section — load sync.json, change
// it, save it: the mutex of this process, then the lock shared with the
// other processes of this config directory. errBusy when another process
// keeps it longer than wait.
func (s *Service) lock(ctx context.Context, wait time.Duration) (unlock func(), err error) {
	s.mu.Lock()
	release, err := acquireExchange(ctx, s.Paths.LockFile(), wait)
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	return func() {
		release()
		s.mu.Unlock()
	}, nil
}

// locked runs fn — a write into the config directory that a command the
// person typed makes outside an exchange (the sync key, the sources of a
// key import) — under the same lock, so it never lands in the middle of
// another process's exchange.
func (s *Service) locked(ctx context.Context, fn func() error) error {
	unlock, err := s.lock(ctx, s.commandWait())
	if err != nil {
		return err
	}
	defer unlock()
	return fn()
}

// localErr — the text for a failed local step: busy, or the error itself.
func (s *Service) localErr(err error) string {
	if errors.Is(err, errBusy) {
		s.busy.Store(true)
		return i18n.T("sync.busy")
	}
	return i18n.Tf("sync.error", err)
}

// backgroundWait and commandWait — how long the background runner and a
// command the person typed wait for the lock.
func (s *Service) backgroundWait() time.Duration { return orDefault(s.busyWait, lockWaitBackground) }
func (s *Service) commandWait() time.Duration    { return orDefault(s.CommandWait, lockWaitCommand) }

// Busy reports whether a command of this Service gave up because another
// ExecAI process on this computer held the sync lock longer than it waited
// — so a shell command can fail with a non-zero exit, while the chat just
// shows the text.
func (s *Service) Busy() bool { return s.busy.Load() }

// Refused reports whether a command of this Service was refused — no plan
// for sync, not signed in — so a shell command can exit non-zero; the chat
// only shows the text.
func (s *Service) Refused() bool { return s.refused.Load() }

// refuse records a refusal of a command and returns its text.
func (s *Service) refuse(text string) string {
	s.refused.Store(true)
	return text
}
