//go:build linux

package uiwindow

import (
	"os"
	"path/filepath"
)

// defaultBrowser — the desktop file that opens http links: what
// xdg-settings says, or the person's mimeapps.list when it is not there.
func defaultBrowser() string {
	if id := output("xdg-settings", "get", "default-web-browser"); id != "" {
		return id
	}
	cfg := os.Getenv("XDG_CONFIG_HOME")
	if cfg == "" {
		if home, err := os.UserHomeDir(); err == nil {
			cfg = filepath.Join(home, ".config")
		}
	}
	if data, err := os.ReadFile(filepath.Join(cfg, "mimeapps.list")); err == nil {
		return parseMimeapps(string(data))
	}
	return ""
}

func appPath(string) string { return "" }
