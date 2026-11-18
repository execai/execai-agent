//go:build windows

package uiwindow

import (
	"os"
	"os/exec"
	"strconv"
)

// askToClose — taskkill without /F posts a close message to the windows of
// the process, which a browser takes as closing them by hand.
func askToClose(p *os.Process) {
	_ = exec.Command("taskkill", "/PID", strconv.Itoa(p.Pid)).Run()
}
