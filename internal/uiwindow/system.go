package uiwindow

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// RealSystem — the computer the agent runs on.
func RealSystem() System {
	home, _ := os.UserHomeDir()
	return System{
		GOOS:     runtime.GOOS,
		Home:     home,
		Getenv:   os.Getenv,
		LookPath: exec.LookPath,
		Exists: func(path string) bool {
			st, err := os.Stat(path)
			return err == nil && !st.IsDir()
		},
		Resolve: func(path string) string {
			if real, err := filepath.EvalSymlinks(path); err == nil {
				return real
			}
			return path
		},
		DefaultBrowser: defaultBrowser,
		AppPath:        appPath,
	}
}

// output — what a short system command prints; "" when it fails or takes
// longer than a moment (the search then goes by its list alone).
func output(name string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
