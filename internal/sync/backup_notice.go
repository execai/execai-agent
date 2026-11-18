package sync

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// homePath — a path for the person: under the home folder as ~/…
func homePath(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || p == "" {
		return p
	}
	home = filepath.Clean(home)
	switch {
	case p == home:
		return "~"
	case strings.HasPrefix(p, home+string(filepath.Separator)):
		return "~" + p[len(home):]
	}
	return p
}

// HomePath — a path for the person, as sync's own lines show it: under the
// home folder as ~/… (the chat and the editor panel say «bound to» and
// «unbound from» the same way).
func HomePath(p string) string { return homePath(p) }

// homePaths — homePath of each, in alphabetical order, joined for one line
// (the order an exchange met the files in means nothing to the person).
func homePaths(ps []string) string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = homePath(p)
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

// subject — the area of a group of files, for a line: a project's folder,
// the memory or the settings.
func subject(kind, root string, n int) string {
	switch kind {
	case KindProjectMemory:
		return tn("sync.subject.project", n, homePath(filepath.Dir(root)))
	case KindMemory:
		return tn("sync.subject.memory", n)
	}
	return tn("sync.subject.settings", n)
}

// tn — the message key for n things: key.one when n is 1 and the catalog
// has it (the singular reads differently in most languages), key otherwise.
func tn(key string, n int, args ...any) string {
	if n == 1 {
		if one := key + ".one"; i18n.T(one) != one {
			return i18n.Tf(one, args...)
		}
	}
	return i18n.Tf(key, args...)
}

// countPhrase — a phrase with a number in it: key.one (no number in it) for
// one, key with the number otherwise.
func countPhrase(key string, n int) string {
	if one := key + ".one"; n == 1 && i18n.T(one) != one {
		return i18n.T(one)
	}
	return i18n.Tf(key, n)
}

// keepNotices — the lines about what one exchange kept, took back, removed
// or sent again: one per kind of event per area, with the number of files.
// Versions replaced by newer ones from the server (the history) are not
// announced; /sync counts them. A file removed because another computer
// deleted it is announced, with where its last version is kept.
func keepNotices(r Report, p Paths) []string {
	var out []string
	type group struct {
		kind, root string
		why        reason
	}
	var order []group
	byGroup := map[group][]string{}
	var permissions, keys []string
	// An older snapshot of a project: one line per project for its files
	// that went back and the ones it lacks.
	type older struct{ back, gone []string }
	var olderOrder []string
	olderOf := map[string]*older{}
	for _, c := range r.Unsent {
		if c.kind == KindConfig && filepath.Base(c.path) != "" && strings.HasPrefix(filepath.Base(c.path), "permissions.") {
			permissions = append(permissions, c.path)
			continue
		}
		if c.kind == KindConfig && c.why == reasonBoth && strings.HasPrefix(filepath.Base(c.path), "subscriptions.") {
			// The copy may hold a key removed on another computer.
			keys = append(keys, c.path)
			continue
		}
		if c.why == reasonOlderBack || c.why == reasonOlderGone {
			o := olderOf[c.root]
			if o == nil {
				o = &older{}
				olderOf[c.root] = o
				olderOrder = append(olderOrder, c.root)
			}
			if c.why == reasonOlderBack {
				o.back = append(o.back, c.path)
			} else {
				o.gone = append(o.gone, c.path)
			}
			continue
		}
		g := group{c.kind, c.root, c.why}
		if _, ok := byGroup[g]; !ok {
			order = append(order, g)
		}
		byGroup[g] = append(byGroup[g], c.path)
	}
	for _, g := range order {
		paths := byGroup[g]
		key := "sync.unsent.kept"
		switch g.why {
		case reasonJoin:
			out = append(out, i18n.Tf("sync.config.joined", homePaths(paths)))
			continue
		case reasonUntrusted:
			key = "sync.unsent.untrusted"
		case reasonBoth:
			key = "sync.unsent.both"
		case reasonNoBase:
			key = "sync.unsent.noBase"
		case reasonReverted:
			key = "sync.reverted"
		case reasonIndex:
			key = "sync.indexKept"
		}
		out = append(out, tn(key, len(paths), subject(g.kind, g.root, len(paths)), len(paths), homePaths(paths)))
	}
	for _, root := range olderOrder {
		o := olderOf[root]
		out = append(out, i18n.Tf("sync.olderSnapshot", homePath(filepath.Dir(root)),
			countPhrase("sync.olderSnapshot.back", len(o.back)), countPhrase("sync.olderSnapshot.gone", len(o.gone)),
			homePaths(append(append([]string(nil), o.back...), o.gone...))))
	}
	if len(permissions) > 0 {
		out = append(out, i18n.Tf("sync.unsent.permissions", homePaths(permissions)))
	}
	if len(keys) > 0 {
		out = append(out, i18n.Tf("sync.unsent.keys", homePaths(keys)))
	}
	for _, a := range r.Restored {
		// One file by its path; several by their number and the folder
		// that holds them.
		where := ""
		if len(a.files) == 1 {
			where = homePath(a.files[0])
		} else {
			where = homePath(commonDir(a.files))
		}
		out = append(out, tn("sync.restored", len(a.files), subject(a.kind, a.root, len(a.files)), len(a.files), where))
	}
	for _, a := range r.Rollback {
		out = append(out, tn("sync.rollback", len(a.files), subject(a.kind, a.root, len(a.files)), len(a.files), homePaths(a.files),
			homePath(historyWhere(p, a, r)), historyDays))
	}
	for _, a := range r.RollbackGone {
		out = append(out, tn("sync.rollbackGone", len(a.files), subject(a.kind, a.root, len(a.files)), len(a.files), homePaths(a.files),
			homePath(historyWhere(p, a, r)), historyDays))
	}
	for _, a := range r.DeletedThere {
		key := "sync.deletedThere"
		if a.kind == KindProjectMemory {
			// A file a version of the project lacks: deleted elsewhere,
			// or that version is an older one — the cause is not known.
			key = "sync.notInVersion"
		}
		out = append(out, tn(key, len(a.files), subject(a.kind, a.root, len(a.files)), len(a.files), homePaths(a.files),
			homePath(historyWhere(p, a, r)), historyDays))
	}
	for _, a := range r.SentNew {
		out = append(out, tn("sync.sentNew", len(a.files), homePath(filepath.Dir(a.root)), len(a.files), homePaths(a.files)))
	}
	for _, a := range r.SettingsEarlier {
		for _, f := range a.files {
			key := "sync.keysEarlier"
			if strings.HasPrefix(filepath.Base(f), "permissions.") {
				key = "sync.permissionsEarlier"
			}
			one := areaCount{kind: a.kind, root: a.root, files: []string{f}}
			out = append(out, i18n.Tf(key, homePath(f), homePath(historyWhere(p, one, r)), historyDays))
		}
	}
	for _, a := range r.SentNewKey {
		out = append(out, tn("sync.sentNewKey", len(a.files), subject(a.kind, a.root, len(a.files)), len(a.files), homePaths(a.files)))
	}
	for _, a := range r.CameBack {
		out = append(out, tn("sync.cameBack", len(a.files), subject(a.kind, a.root, len(a.files)), len(a.files), homePaths(a.files)))
	}
	for _, a := range r.Resent {
		out = append(out, tn("sync.resent", len(a.files), subject(a.kind, a.root, len(a.files)), len(a.files), homePaths(a.files)))
	}
	for _, a := range r.ReturnSent {
		out = append(out, tn("sync.returnSent", len(a.files), subject(a.kind, a.root, len(a.files)), len(a.files), homePaths(a.files)))
	}
	for _, dir := range r.ProjectsKept {
		out = append(out, i18n.Tf("sync.project.keptDeleted", homePath(dir)))
	}
	for _, a := range r.ProjectsBack {
		out = append(out, tn("sync.projectBack", len(a.files), homePath(filepath.Dir(a.root)), len(a.files)))
	}
	for _, a := range r.Changed {
		out = append(out, tn("sync.changedDuring", len(a.files), subject(a.kind, a.root, len(a.files)), len(a.files), homePaths(a.files)))
	}
	if len(r.Collisions) > 0 {
		names := slices.Clone(r.Collisions)
		sort.Strings(names)
		out = append(out, i18n.Tf("sync.caseCollision", strings.Join(names, ", ")))
	}
	if r.HistoryEvicted > 0 {
		out = append(out, i18n.Tf("sync.historyEvicted", r.HistoryEvicted, homePath(p.HistoryDir()), historyBytes>>20))
	}
	if r.BasesNewer {
		out = append(out, i18n.T("sync.basesNewer"))
	}
	return out
}

// historyWhere — where the last versions of an area's files went: for one
// file its history copy, for several the deepest folder that holds all of
// their copies; the area's history folder when a copy is not known.
func historyWhere(p Paths, a areaCount, r Report) string {
	var copies []string
	for _, f := range a.files {
		c := r.historyOf[f]
		if c == "" {
			return historyFolder(p, a)
		}
		copies = append(copies, c)
	}
	if len(copies) == 0 {
		return historyFolder(p, a)
	}
	if len(copies) == 1 {
		return copies[0]
	}
	dir := filepath.Dir(copies[0])
	for _, c := range copies[1:] {
		for !strings.HasPrefix(c, dir+string(filepath.Separator)) {
			up := filepath.Dir(dir)
			if up == dir {
				return historyFolder(p, a)
			}
			dir = up
		}
	}
	return dir
}

// commonDir — the deepest folder that holds all of files.
func commonDir(files []string) string {
	if len(files) == 0 {
		return ""
	}
	dir := filepath.Dir(files[0])
	for _, f := range files[1:] {
		for !strings.HasPrefix(f, dir+string(filepath.Separator)) {
			up := filepath.Dir(dir)
			if up == dir {
				return dir
			}
			dir = up
		}
	}
	return dir
}

// historyFolder — where the history of an area's files is kept.
func historyFolder(p Paths, a areaCount) string {
	switch a.kind {
	case KindProjectMemory:
		return filepath.Join(p.HistoryDir(), "projects")
	case KindMemory:
		return filepath.Join(p.HistoryDir(), "memory")
	}
	return filepath.Join(p.HistoryDir(), "settings")
}

// skipLine — one line about something left out of an exchange (local.go).
func skipLine(sk Skip) string {
	var why string
	switch sk.Reason {
	case SkipNoFolder:
		return i18n.Tf("sync.skip.noFolder", homePath(sk.Path))
	case SkipTooBig:
		why = i18n.Tf("sync.skip.why.tooBig", sk.Limit)
	case SkipUnreadable:
		if sk.Cause == causeWrite {
			return i18n.Tf("sync.skip.unwritable", homePath(sk.Path))
		}
		key := "sync.skip.why.unreadable"
		if sk.Cause == causePermission {
			key = "sync.skip.why.noPermission"
		}
		if sk.Detail == "" {
			why = i18n.T(key + "Here")
		} else {
			why = i18n.Tf(key, sk.Detail)
		}
	case SkipFolderTaken:
		taken, this, _ := strings.Cut(sk.Detail, "\x00")
		return i18n.Tf("sync.skip.folderTaken", homePath(sk.Path), taken, this)
	case SkipNameClash:
		return i18n.Tf("sync.nameClash", homePath(sk.Path))
	case SkipLink:
		why = i18n.T("sync.skip.why.link")
	case SkipLinkInside:
		why = i18n.Tf("sync.skip.why.linkInside", homePath(sk.Detail))
	case SkipSpecialInside:
		why = i18n.Tf("sync.skip.why.specialInside", homePath(sk.Detail))
	default:
		why = i18n.T("sync.skip.why.special")
	}
	if sk.One {
		return i18n.Tf("sync.skip.one", homePath(sk.Path), why)
	}
	return i18n.Tf("sync.skip.line", homePath(sk.Path), why)
}

// skipLines — the lines for everything an exchange left out.
func skipLines(skips []Skip) []string {
	var out []string
	for _, sk := range skips {
		out = append(out, skipLine(sk))
	}
	return out
}
