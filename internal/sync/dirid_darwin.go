//go:build darwin

package sync

import (
	"fmt"
	"syscall"
)

func osDirIdentity(path string) string {
	var st syscall.Stat_t
	if err := syscall.Lstat(path, &st); err != nil || st.Mode&syscall.S_IFMT != syscall.S_IFDIR {
		return ""
	}
	b := st.Birthtimespec
	if b.Sec == 0 && b.Nsec == 0 {
		return ""
	}
	return fmt.Sprintf("%d:%d.%09d", st.Ino, b.Sec, b.Nsec)
}
