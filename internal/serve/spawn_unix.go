//go:build !windows

package serve

import (
	"os/exec"
	"syscall"
)

// detach отвязывает процесс от сессии терминала: Setsid делает его лидером
// новой сессии, поэтому SIGHUP при закрытии терминала до него не долетит.
// Без этого демон умирал бы вместе с TUI, из которого запущен.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
