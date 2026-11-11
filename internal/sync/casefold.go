package sync

import (
	"os"
	"path/filepath"
	"strings"
	gosync "sync"
)

// caseInsensitive — the file system of dir does not tell names apart by
// case: two names that differ only in case are then left as they are
// (memsync.go, projectsync.go). It is asked of the folder itself, once per
// folder per process — not guessed from the system: WSL's /mnt/c, exFAT and
// FAT drives, and ext4 folders with casefold are case-insensitive on Linux,
// while an APFS volume can be case-sensitive. Tests stand in either answer.
var caseInsensitive = probeCaseInsensitive

var caseProbes gosync.Map // folder → bool

// probeCaseInsensitive makes a temporary file in dir and asks for it by
// another case. A folder that is not there or cannot be written is guessed
// from the system (macOS and Windows usually do not tell case apart).
func probeCaseInsensitive(dir string) bool {
	if v, ok := caseProbes.Load(dir); ok {
		return v.(bool)
	}
	ans, ok := probeCase(dir)
	if !ok {
		return osName == "darwin" || osName == "windows"
	}
	caseProbes.Store(dir, ans)
	return ans
}

// probeCase — ok=false when dir cannot be probed. The probe file is named
// so that no scan takes it for content (*.tmp) and is removed at once.
func probeCase(dir string) (insensitive, ok bool) {
	f, err := os.CreateTemp(dir, ".sync-case-probe-*.tmp")
	if err != nil {
		return false, false
	}
	name := f.Name()
	_ = f.Close()
	defer os.Remove(name)
	base := filepath.Base(name)
	other := strings.ToUpper(base)
	if other == base {
		return false, false
	}
	_, err = os.Lstat(filepath.Join(dir, other))
	return err == nil, true
}
