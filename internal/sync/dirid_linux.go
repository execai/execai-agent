//go:build linux

package sync

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func osDirIdentity(path string) string {
	var st unix.Statx_t
	err := unix.Statx(unix.AT_FDCWD, path, unix.AT_SYMLINK_NOFOLLOW, unix.STATX_TYPE|unix.STATX_INO|unix.STATX_BTIME, &st)
	if err != nil || st.Mode&unix.S_IFMT != unix.S_IFDIR || st.Mask&unix.STATX_INO == 0 || st.Mask&unix.STATX_BTIME == 0 {
		return ""
	}
	return fmt.Sprintf("%d:%d.%09d", st.Ino, st.Btime.Sec, st.Btime.Nsec)
}
