package sync

import (
	"bytes"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// The history: versions the server had, replaced here by a newer one or
// removed because another computer deleted the file (threeway.go, rows 2a
// and 2c, and an older copy put back). The server keeps no versions of its
// own, so for a while this may be the only copy left: it is kept for
// historyDays days, by time alone, however many versions a file gets.
//
//	<config>/sync-history/projects/<workspace id>/<path>.<YYYYMMDD-HHMMSS>
//	<config>/sync-history/memory/<path>.<YYYYMMDD-HHMMSS>
//	<config>/sync-history/settings/<name>.<YYYYMMDD-HHMMSS>
//
// Besides the days, a ceiling of historyBytes per project (and for the
// memory, and for the settings): the oldest go first, and when a version
// younger than historyDays has to go the person is told. The history is
// cleaned once per exchange (Engine.Run), not after every version: a new
// version of a project of hundreds of files is taken in a moment. Each
// history folder is read once per exchange (historyIndex). Nothing outside
// sync-history is ever removed by this: permanent copies stay for good.

const historyDays = 30

// historyBytes — the ceiling of one area's history (a var: tests lower it).
var historyBytes int64 = 256 << 20

// readDir — os.ReadDir, counted by tests.
var readDir = os.ReadDir

// HistoryDir — <config>/sync-history.
func (p Paths) HistoryDir() string { return filepath.Join(p.ConfigDir, "sync-history") }

// historyArea — the folder of one project, the memory or the settings.
func historyArea(p Paths, pl place) string {
	switch pl.kind {
	case KindProjectMemory:
		return filepath.Join(p.HistoryDir(), "projects", pl.scope)
	case KindMemory:
		return filepath.Join(p.HistoryDir(), "memory")
	}
	return filepath.Join(p.HistoryDir(), "settings")
}

// historyNameRe — <name>.<YYYYMMDD-HHMMSS>[-n].
var historyNameRe = regexp.MustCompile(`^(.+)\.(\d{8}-\d{6})(-\d+)?$`)

type historyVersion struct {
	path  string
	stamp time.Time
	size  int64
}

// historyIndex — the history folders one exchange has read: folder → file
// name → versions, newest first. Each folder is read once.
type historyIndex struct {
	dirs map[string]map[string][]historyVersion
}

func (h *historyIndex) versions(dir, base string) []historyVersion {
	if h.dirs == nil {
		h.dirs = map[string]map[string][]historyVersion{}
	}
	m, ok := h.dirs[dir]
	if !ok {
		m = readHistoryDir(dir)
		h.dirs[dir] = m
	}
	return m[base]
}

func (h *historyIndex) add(dir, base string, v historyVersion) {
	h.versions(dir, base)
	h.dirs[dir][base] = append([]historyVersion{v}, h.dirs[dir][base]...)
}

// forget drops a version written for a replacement that did not happen.
func (h *historyIndex) forget(p string) {
	if h == nil || h.dirs == nil {
		return
	}
	dir, name := filepath.Split(p)
	m := historyNameRe.FindStringSubmatch(name)
	if m == nil {
		return
	}
	dir = filepath.Clean(dir)
	list := h.dirs[dir][m[1]]
	for i, v := range list {
		if v.path == p {
			h.dirs[dir][m[1]] = append(list[:i:i], list[i+1:]...)
			return
		}
	}
}

// readHistoryDir — the versions in one folder, by file name, newest first.
func readHistoryDir(dir string) map[string][]historyVersion {
	out := map[string][]historyVersion{}
	entries, err := readDir(dir)
	if err != nil {
		return out
	}
	for _, de := range entries {
		m := historyNameRe.FindStringSubmatch(de.Name())
		if m == nil || !de.Type().IsRegular() {
			continue
		}
		t, err := time.ParseInLocation(copyStampLayout, m[2], time.Local)
		if err != nil {
			continue
		}
		var size int64
		if info, err := de.Info(); err == nil {
			size = info.Size()
		}
		out[m[1]] = append(out[m[1]], historyVersion{path: filepath.Join(dir, de.Name()), stamp: t, size: size})
	}
	for _, list := range out {
		sortVersions(list)
	}
	return out
}

func sortVersions(list []historyVersion) {
	sort.Slice(list, func(i, j int) bool {
		if list[i].stamp.Equal(list[j].stamp) {
			return list[i].path > list[j].path
		}
		return list[i].stamp.After(list[j].stamp)
	})
}

// historyPlace — the folder and the file name of pl's history versions;
// ok=false when pl has none (a name that is not safe, no project id).
func historyPlace(p Paths, pl place) (dir, base string, ok bool) {
	if !safeRel(pl.rel) || (pl.kind == KindProjectMemory && !validName(pl.scope)) {
		return "", "", false
	}
	return filepath.Join(historyArea(p, pl), filepath.FromSlash(path.Dir(pl.rel))), path.Base(pl.rel), true
}

// newestHistory — the newest history version of pl's file, "" when none.
func newestHistory(p Paths, idx *historyIndex, pl place) string {
	dir, base, ok := historyPlace(p, pl)
	if !ok || idx == nil {
		return ""
	}
	if versions := idx.versions(dir, base); len(versions) > 0 {
		return versions[0].path
	}
	return ""
}

// keepHistoryCopy writes data as the newest history version of pl's file,
// flushed to disk; "" when the newest one already holds exactly this.
func keepHistoryCopy(p Paths, idx *historyIndex, now time.Time, pl place, data []byte) (string, error) {
	dir, base, ok := historyPlace(p, pl)
	if !ok {
		return "", nil
	}
	if idx == nil {
		idx = &historyIndex{}
	}
	now = now.Local()
	if versions := idx.versions(dir, base); len(versions) > 0 {
		if b, err := os.ReadFile(versions[0].path); err == nil && bytes.Equal(b, data) {
			return "", nil
		}
	}
	target := filepath.Join(dir, base+"."+now.Format(copyStampLayout))
	for i := 2; fileExists(target); i++ {
		target = filepath.Join(dir, base+"."+now.Format(copyStampLayout)+"-"+strconv.Itoa(i))
	}
	if err := writeFileAtomic(target, data); err != nil {
		return "", err
	}
	privateDirs(p.HistoryDir(), dir)
	idx.add(dir, base, historyVersion{path: target, stamp: now, size: int64(len(data))})
	return target, nil
}

// historyVersions — the versions of base in dir, newest first.
func historyVersions(dir, base string) []historyVersion {
	return readHistoryDir(dir)[base]
}

// pruneHistory keeps every area (a project, the memory, the settings)
// within the limits: versions older than historyDays go; then, oldest
// first, what is over historyBytes. Returns how many versions younger than
// historyDays had to go for the ceiling. Only history files are touched,
// and only inside sync-history; the tree is walked once.
func pruneHistory(p Paths, now time.Time) (young int) {
	root := p.HistoryDir()
	areas := map[string][]historyVersion{}
	_ = filepath.WalkDir(root, func(fp string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		m := historyNameRe.FindStringSubmatch(d.Name())
		if m == nil {
			return nil
		}
		t, err := time.ParseInLocation(copyStampLayout, m[2], time.Local)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, fp)
		parts := strings.SplitN(filepath.ToSlash(rel), "/", 3)
		area := parts[0]
		if area == "projects" && len(parts) > 2 {
			area += "/" + parts[1]
		}
		var size int64
		if info, err := d.Info(); err == nil {
			size = info.Size()
		}
		areas[area] = append(areas[area], historyVersion{path: fp, stamp: t, size: size})
		return nil
	})
	cutoff := now.AddDate(0, 0, -historyDays)
	for _, list := range areas {
		sortVersions(list)
		var total int64
		for _, v := range list {
			if v.stamp.Before(cutoff) {
				_ = os.Remove(v.path)
				continue
			}
			total += v.size
			if total > historyBytes {
				_ = os.Remove(v.path)
				young++
			}
		}
	}
	return young
}

// historyCount — the history versions kept (for /sync).
func historyCount(p Paths) int {
	n := 0
	_ = filepath.WalkDir(p.HistoryDir(), func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && historyNameRe.MatchString(d.Name()) {
			n++
		}
		return nil
	})
	return n
}

// historyHas — a history version of pl's file holds exactly data (tests,
// and the message about an older copy).
func historyHas(p Paths, pl place, data []byte) bool {
	dir := filepath.Join(historyArea(p, pl), filepath.FromSlash(path.Dir(pl.rel)))
	for _, v := range historyVersions(dir, path.Base(pl.rel)) {
		if b, err := os.ReadFile(v.path); err == nil && bytes.Equal(b, data) {
			return true
		}
	}
	return false
}
