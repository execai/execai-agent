//go:build !windows

package uiwindow

import (
	"os"
	"syscall"
)

// askToClose — SIGTERM: a Chromium browser closes its windows, saves the
// profile and ends.
func askToClose(p *os.Process) { _ = p.Signal(syscall.SIGTERM) }
