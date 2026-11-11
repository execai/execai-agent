//go:build windows

package sync

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func osDirIdentity(path string) string {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return ""
	}
	// A folder opens with FILE_FLAG_BACKUP_SEMANTICS; OPEN_REPARSE_POINT so
	// that a link or junction is seen as itself and refused below.
	h, err := windows.CreateFile(p, 0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil,
		windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	var fi windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &fi); err != nil {
		return ""
	}
	if fi.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 || fi.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return ""
	}
	return fmt.Sprintf("%08x:%08x%08x:%d", fi.VolumeSerialNumber, fi.FileIndexHigh, fi.FileIndexLow, fi.CreationTime.Nanoseconds())
}
