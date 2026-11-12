//go:build !windows

package notify

import "os/exec"

// hideWindow — nothing to hide outside Windows.
func hideWindow(*exec.Cmd) {}
