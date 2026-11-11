// Package welcome knows whether this is the first launch on this machine.
// The `.welcome_seen` marker lives in the config directory; it is created
// on the first launch, and later launches are not first any more. On the
// first launch the chat shows the getting-started steps (/start).
package welcome

import (
	"os"
	"path/filepath"
)

// FirstLaunch reports whether this is the first launch and records it, so
// it is true exactly once per config directory. When the directory cannot
// be found it is true every time: better to show the steps again than to
// never show them.
func FirstLaunch() bool {
	base, err := os.UserConfigDir()
	if err != nil {
		return true
	}
	dir := filepath.Join(base, "execai")
	_ = os.MkdirAll(dir, 0o700)
	marker := filepath.Join(dir, ".welcome_seen")
	if _, err := os.Stat(marker); err == nil {
		return false
	}
	_ = os.WriteFile(marker, []byte("1"), 0o644)
	return true
}
