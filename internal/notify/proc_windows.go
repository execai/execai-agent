//go:build windows

package notify

import (
	"os/exec"
	"syscall"
)

// hideWindow starts PowerShell without a window and without the agent's
// console: a child on the same console could change its modes under the TUI.
func hideWindow(cmd *exec.Cmd) {
	const createNoWindow = 0x08000000
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}
