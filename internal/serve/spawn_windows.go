//go:build windows

package serve

import (
	"os/exec"
	"syscall"
)

// detach — Windows-аналог setsid: процесс уходит в собственную группу и без
// консольного окна, поэтому закрытие терминала его не касается.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | 0x08000000, // + CREATE_NO_WINDOW
	}
}
