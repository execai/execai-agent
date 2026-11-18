//go:build !windows

package sync

import (
	"errors"
	"os"
	"syscall"
)

// tryLockFile takes an exclusive flock on f without waiting: false when
// another open file holds it. flock, not fcntl: fcntl locks belong to the
// process and would let two Services of one process in at once.
func tryLockFile(f *os.File) (bool, error) {
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		switch {
		case err == nil:
			return true, nil
		case errors.Is(err, syscall.EWOULDBLOCK):
			return false, nil
		case errors.Is(err, syscall.EINTR):
			continue
		}
		return false, err
	}
}

func unlockFile(f *os.File) { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }
