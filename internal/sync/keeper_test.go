package sync

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// keeperAllowed — every call in this package (tests aside) that writes,
// renames or removes a file, by the function it is in, and why it may. A
// person's file — memory, settings, a project's .execai/ — is only ever
// changed by keeper.put and keeper.remove, which keep a copy first.
var keeperAllowed = map[string]map[string]string{
	"prepareFile": {
		"os.MkdirAll": "the folder of a temporary file", "os.CreateTemp": "the temporary file next to the target",
		"os.Chmod": "", "os.Remove": "the temporary file on failure",
	},
	"commitFile":      {"os.Rename": "the temporary file over the target", "os.Remove": "the temporary file on failure"},
	"writeFileAtomic": {"prepareFile": "", "commitFile": ""},
	"keeper.put": {"prepareFile": "after the copy of what was there", "commitFile": "after the second comparison, under the writers' lock",
		"os.Remove": "the temporary file of a replacement that did not happen", "os.Chtimes": ""},
	"keeper.remove":      {"os.Remove": "after the copy of what was there, under the writers' lock"},
	"keeper.undo":        {"os.Remove": "a copy just made for a replacement that did not happen (what it holds is still in the file)"},
	"keeper.projectCopy": {"writeFileAtomic": "a copy, under a name that is free"},
	"keepUnsentCopy":     {"writeFileAtomic": "a copy, under a name that is free"},
	"keepHistoryCopy":    {"writeFileAtomic": "a history version, under a name that is free"},
	"pruneHistory":       {"os.Remove": "history versions over the limits, inside sync-history only"},
	"State.Save":         {"writeFileAtomic": "sync.json, sync's own state"},
	"saveBases":          {"writeFileAtomic": "sync-bases.json, sync's own state"},
	"acquireExchange":    {"os.MkdirAll": "", "os.OpenFile": "sync.lock, the exchange lock"},
	"replaceKey":         {"os.Rename": "sync.key moved aside, kept as sync.key.old-<time>", "os.Chmod": "", "crypto.SaveIdentity": "the key taken from the server"},
	"onFlow.Answer":      {"crypto.SaveIdentity": "the sync key, under the lock"},
	"pruneEmptyDirs":     {"os.Remove": "empty folders only (fails on any other)"},
	"privateDirs":        {"os.Chmod": "the folders of sync's own copies, inside sync-unsent and sync-history only, to 0700"},
	"privateFile":        {"os.Chmod": "a copy of sync's own found in sync-unsent, to 0600"},
	"probeCase":          {"os.CreateTemp": "a probe file (*.tmp, never synced) to ask the folder about case", "os.Remove": "the probe file"},
}

// keeperWatched — the calls that write, rename or remove files.
var keeperWatched = map[string]bool{
	"os.Remove": true, "os.RemoveAll": true, "os.WriteFile": true, "os.Rename": true,
	"os.Create": true, "os.CreateTemp": true, "os.OpenFile": true, "os.Truncate": true,
	"os.Mkdir": true, "os.MkdirAll": true, "os.Chmod": true, "os.Chtimes": true,
	"os.Symlink": true, "os.Link": true, "os.Lchown": true, "os.Chown": true,
	"ioutil.WriteFile": true, "writeFileAtomic": true, "prepareFile": true, "commitFile": true,
	"crypto.SaveIdentity": true, "config.SaveCredentials": true,
}

// TestKeeper_OnlyPlaceThatTouchesFiles — the sentinel of the one place that
// destroys (keeper.go): it parses the sources of this package and fails on
// any call that writes, renames or removes a file outside the functions
// listed above, and on any call of a no-argument Save (such as
// subscriptions.Store.Save, which writes subscriptions.json past the
// keeper). State.Save takes the paths and stays allowed.
func TestKeeper_OnlyPlaceThatTouchesFiles(t *testing.T) {
	fset := token.NewFileSet()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]map[string]bool{}
	var bad []string
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		af, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range af.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			fn := fd.Name.Name
			if fd.Recv != nil && len(fd.Recv.List) == 1 {
				fn = recvName(fd.Recv.List[0].Type) + "." + fn
			}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				name := calleeName(call.Fun)
				if strings.HasSuffix(name, ".Save") && len(call.Args) == 0 {
					bad = append(bad, fset.Position(call.Pos()).String()+": "+fn+" calls "+name+"() — a file written past the keeper")
					return true
				}
				if !keeperWatched[name] {
					return true
				}
				if _, ok := keeperAllowed[fn][name]; !ok {
					bad = append(bad, fset.Position(call.Pos()).String()+": "+fn+" calls "+name+" — only the keeper may change files")
					return true
				}
				if seen[fn] == nil {
					seen[fn] = map[string]bool{}
				}
				seen[fn][name] = true
				return true
			})
		}
	}
	for fn, calls := range keeperAllowed {
		for c := range calls {
			if !seen[fn][c] {
				bad = append(bad, "allowed but not found (remove it from the list): "+fn+" → "+c)
			}
		}
	}
	sort.Strings(bad)
	for _, b := range bad {
		t.Error(b)
	}
}

func recvName(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.StarExpr:
		return recvName(x.X)
	case *ast.Ident:
		return x.Name
	case *ast.IndexExpr:
		return recvName(x.X)
	}
	return "?"
}

func calleeName(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		if id, ok := x.X.(*ast.Ident); ok {
			return id.Name + "." + x.Sel.Name
		}
		return "?." + x.Sel.Name
	}
	return ""
}

// The sentinel catches a write past the keeper: a function that calls
// os.WriteFile is reported.
func TestKeeper_SentinelCatchesAWritePastTheKeeper(t *testing.T) {
	src := `package sync
import "os"
func sneaky(p string) { _ = os.WriteFile(p, nil, 0o600); _ = os.Remove(p) }
func store() { var s interface{ Save() error }; _ = s.Save() }
`
	af, err := parser.ParseFile(token.NewFileSet(), "x.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	ast.Inspect(af, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			name := calleeName(call.Fun)
			if keeperWatched[name] || (strings.HasSuffix(name, ".Save") && len(call.Args) == 0) {
				found = append(found, name)
			}
		}
		return true
	})
	if strings.Join(found, ",") != "os.WriteFile,os.Remove,s.Save" {
		t.Fatalf("sentinel saw %v", found)
	}
}

// Every file sync replaces or removes leaves a copy of what it held (unless
// nothing is lost), of the kind the decision says: content the server had
// goes to the history, out of the way; content it never saw stays next to
// the project file (or in sync-conflicts) for good. A file that changed
// since the decision is not touched. The same content is not copied twice.
func TestKeeper_EveryReplacementKeepsACopy(t *testing.T) {
	now := time.Date(2026, 11, 11, 12, 10, 3, 0, time.Local)
	cfg := t.TempDir()
	k := &keeper{paths: Paths{ConfigDir: cfg}, now: func() time.Time { return now }, rep: &Report{}}
	pd := filepath.Join(t.TempDir(), ".execai")
	proj := place{kind: KindProjectMemory, root: pd, rel: "notes/plan.md", scope: "ws-1"}
	mem := place{kind: KindMemory, root: filepath.Join(cfg, "memory"), rel: "fact.md"}
	set := place{kind: KindConfig, root: cfg, rel: "permissions.json"}
	v1, v2, v3 := []byte(`{"v":1}`), []byte(`{"v":2}`), []byte(`{"v":3}`)
	for _, pl := range []place{proj, mem, set} {
		if _, err := k.put(pl, v1, time.Time{}, "", keepHistory, reasonNone, false); err != nil {
			t.Fatal(err)
		}
		if changed, err := k.put(pl, v1, time.Time{}, hashOf(v1), keepHistory, reasonNone, false); err != nil || changed {
			t.Fatalf("the same content written again: %v %v", changed, err)
		}
		// The server's version over one it had seen: history.
		if _, err := k.put(pl, v2, time.Time{}, hashOf(v1), keepHistory, reasonNone, false); err != nil {
			t.Fatal(err)
		}
		// Over content it never saw: next to it, told.
		if _, err := k.put(pl, v3, time.Time{}, hashOf(v2), keepUnsent, reasonBoth, false); err != nil {
			t.Fatal(err)
		}
		// Changed since the decision: untouched.
		if _, err := k.put(pl, v1, time.Time{}, hashOf(v1), keepHistory, reasonNone, false); !errors.Is(err, errChanged) {
			t.Fatalf("a file changed since the decision was replaced: %v", err)
		}
		if err := k.remove(pl, hashOf(v1), keepHistory, reasonNone); !errors.Is(err, errChanged) {
			t.Fatalf("a file changed since the decision was removed: %v", err)
		}
		if err := k.remove(pl, hashOf(v3), keepHistory, reasonNone); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(pl.file()); !os.IsNotExist(err) {
			t.Fatalf("%s not removed", pl.file())
		}
		if !historyHas(k.paths, pl, v1) || !historyHas(k.paths, pl, v3) {
			t.Errorf("%s: history lacks a replaced version", pl.rel)
		}
	}
	if k.rep.History != 6 || len(k.rep.Unsent) != 3 {
		t.Fatalf("history %d, not sent %v", k.rep.History, k.rep.Unsent)
	}
	for _, c := range k.rep.Unsent {
		if strings.HasPrefix(c.path, pd) {
			if !localOnly(filepath.Base(c.path)) {
				t.Errorf("a project copy would travel: %s", c.path)
			}
		} else if !strings.HasPrefix(c.path, filepath.Join(cfg, "sync-unsent")+string(filepath.Separator)) {
			t.Errorf("copy outside sync-unsent: %s", c.path)
		}
		if c.why != reasonBoth {
			t.Errorf("copy %s told as %v", c.path, c.why)
		}
	}
	for _, p := range []string{filepath.Join(cfg, "memory"), pd} {
		_ = filepath.WalkDir(p, func(f string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.Contains(f, "sync-history") {
				t.Errorf("history inside a synced folder: %s", f)
			}
			return nil
		})
	}
}

// A copy next to a project file in the same second as another one of the
// same file gets the next second, not a suffix older agents would send.
func TestKeeper_ProjectCopyNamesStayLocalOnly(t *testing.T) {
	now := time.Date(2026, 11, 11, 12, 10, 3, 0, time.Local)
	k := &keeper{now: func() time.Time { return now }, rep: &Report{}}
	pd := filepath.Join(t.TempDir(), ".execai")
	pl := place{kind: KindProjectMemory, root: pd, rel: "EXECAI.md"}
	prev := ""
	for _, v := range []string{"a", "b", "c", "d"} {
		if _, err := k.put(pl, []byte(v), time.Time{}, prev, keepUnsent, reasonBoth, false); err != nil {
			t.Fatal(err)
		}
		prev = hashOf([]byte(v))
	}
	if len(k.rep.Unsent) != 3 {
		t.Fatalf("copies %v", k.rep.Unsent)
	}
	for i, c := range k.rep.Unsent {
		want := "EXECAI.md.local-" + now.Add(time.Duration(i)*time.Second).Format(copyStampLayout)
		if filepath.Base(c.path) != want || !localOnly(filepath.Base(c.path)) {
			t.Errorf("copy %s, want %s and local only", filepath.Base(c.path), want)
		}
	}
}

// Sync never removes a copy of content the server never saw — its own or
// one an older agent left (review Q13): not next to project files, not in
// sync-conflicts, however old. Only its own history is pruned, to its
// limits.
func TestKeeper_CopiesAreNeverRemoved(t *testing.T) {
	_, a, _, pA, _ := lossPair(t)
	old := filepath.Join(ProjectDir(pA), "EXECAI.md.local-20200901-120000")
	_ = os.WriteFile(old, []byte("the losing side of a 7.15 conflict\n"), 0o600)
	a.use()
	conflicts := filepath.Join(a.dir(), "sync-conflicts", "2020-01-01", "fact.local-120000.md")
	_ = os.MkdirAll(filepath.Dir(conflicts), 0o700)
	_ = os.WriteFile(conflicts, []byte("a copy from 2020\n"), 0o600)
	for i := 0; i < 2; i++ {
		now(a)
	}
	for _, p := range []string{old, conflicts} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("LOSS: %s removed: %v", p, err)
		}
	}
}

// The history keeps every version of a file for historyDays days, however
// many there are; older ones go when the history is cleaned (once per
// exchange); over historyBytes in one area the oldest go first, and the
// versions that had to go younger than historyDays are counted.
func TestHistory_Limits(t *testing.T) {
	cfg := t.TempDir()
	p := Paths{ConfigDir: cfg}
	now := time.Now()
	idx := &historyIndex{}
	pl := place{kind: KindProjectMemory, root: "/x/.execai", rel: "EXECAI.md", scope: "ws-1"}
	for i := 0; i < 8; i++ {
		if _, err := keepHistoryCopy(p, idx, now.Add(time.Duration(i-10)*time.Minute), pl, []byte{byte('a' + i)}); err != nil {
			t.Fatal(err)
		}
	}
	dir := filepath.Join(historyArea(p, pl))
	if v := historyVersions(dir, "EXECAI.md"); len(v) != 8 {
		t.Errorf("versions %d, want all 8 (the history is cut by days only)", len(v))
	}
	old := place{kind: KindProjectMemory, root: "/x/.execai", rel: "old.md", scope: "ws-1"}
	if _, err := keepHistoryCopy(p, idx, now.AddDate(0, 0, -historyDays-1), old, []byte("old")); err != nil {
		t.Fatal(err)
	}
	if young := pruneHistory(p, now); young != 0 {
		t.Errorf("young versions removed: %d", young)
	}
	if v := historyVersions(dir, "old.md"); len(v) != 0 {
		t.Errorf("a version older than %d days stayed", historyDays)
	}
	if v := historyVersions(dir, "EXECAI.md"); len(v) != 8 {
		t.Errorf("young versions went: %d left", len(v))
	}
	defer func(b int64) { historyBytes = b }(historyBytes)
	historyBytes = 5
	if young := pruneHistory(p, now); young != 3 {
		t.Errorf("over the ceiling: %d young versions removed, want 3", young)
	}
	if v := historyVersions(dir, "EXECAI.md"); len(v) != 5 || string(mustRead(t, v[0].path)) != "h" {
		t.Errorf("not the oldest went first: %d left", len(v))
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// The keeper never writes through a link or into what is not a plain
// folder or file, and never replaces what it cannot read.
func TestKeeper_RefusesLinksAndUnreadable(t *testing.T) {
	k := &keeper{now: time.Now, rep: &Report{}}
	base := t.TempDir()
	real := filepath.Join(base, "real")
	if err := os.MkdirAll(filepath.Join(real, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(real, "sub", "a.md"), []byte("mine"), 0o600)
	link := filepath.Join(base, "linked")
	if err := os.Symlink(real, link); err != nil {
		t.Skip("no symlinks here:", err)
	}
	inner := filepath.Join(base, "pd")
	_ = os.MkdirAll(inner, 0o700)
	if err := os.Symlink(filepath.Join(real, "sub"), filepath.Join(inner, "sub")); err != nil {
		t.Fatal(err)
	}
	_ = os.Symlink(filepath.Join(real, "sub", "a.md"), filepath.Join(inner, "f.md"))
	for _, pl := range []place{
		{kind: KindProjectMemory, root: link, rel: "sub/a.md"},      // the root is a link
		{kind: KindMemory, root: link, rel: "sub/a.md"},             // the memory folder is a link
		{kind: KindProjectMemory, root: inner, rel: "sub/a.md"},     // a folder inside is a link
		{kind: KindProjectMemory, root: inner, rel: "f.md"},         // the file is a link
		{kind: KindConfig, root: inner, rel: "f.md"},                // a settings file that is a link
		{kind: KindProjectMemory, root: real, rel: "sub/a.md/x.md"}, // a file where a folder should be
	} {
		if _, err := k.put(pl, []byte("theirs"), time.Time{}, expectAny, keepHistory, reasonNone, false); err == nil {
			t.Errorf("put through %s/%s", pl.root, pl.rel)
		}
		if err := k.remove(pl, expectAny, keepHistory, reasonNone); err == nil {
			t.Errorf("remove through %s/%s", pl.root, pl.rel)
		}
	}
	if got, _ := os.ReadFile(filepath.Join(real, "sub", "a.md")); string(got) != "mine" {
		t.Fatalf("the file behind the link changed: %q", got)
	}
	if os.Getuid() != 0 {
		locked := filepath.Join(base, "locked")
		_ = os.MkdirAll(locked, 0o700)
		_ = os.WriteFile(filepath.Join(locked, "a.md"), []byte("mine"), 0o000)
		t.Cleanup(func() { _ = os.Chmod(filepath.Join(locked, "a.md"), 0o600) })
		if _, err := k.put(place{kind: KindProjectMemory, root: locked, rel: "a.md"}, []byte("theirs"), time.Time{}, expectAny, keepHistory, reasonNone, false); err == nil {
			t.Error("replaced a file it could not read")
		}
	}
	if k.rep.History+len(k.rep.Unsent) != 0 {
		t.Errorf("copies %d %v", k.rep.History, k.rep.Unsent)
	}
}

// Between two computers: a file replaced by a newer version from the other
// one, while it was not changed here, leaves its previous content in the
// history — out of the synced folders, silently: the server had it. A
// project folder gets no copy next to its files for it.
func TestKeeper_RoutineUpdatesLeaveHistory(t *testing.T) {
	_, a, b, pA, pB := projectPair(t)
	a.use()
	a.writeFile("memory/fact.md", "v1\n")
	a.writeFile("config.json", `{"model":"one"}`)
	writeProjectFile(t, pA, "EXECAI.md", "build: make\n", time.Time{})
	now(a)
	now(b)
	a.use()
	a.writeFile("memory/fact.md", "v2\n")
	a.writeFile("config.json", `{"model":"two"}`)
	writeProjectFile(t, pA, "EXECAI.md", "build: make all\n", time.Time{})
	now(a)
	out := now(b)
	if b.readFile("memory/fact.md") != "v2\n" || readProjectFile(pB, "EXECAI.md") != "build: make all\n" {
		t.Fatal("the update did not arrive")
	}
	if c := copiesIn(ProjectDir(pB)); len(c) != 0 {
		t.Fatalf("copies next to project files for a routine update: %v", c)
	}
	b.use()
	ps := b.svc.Paths
	if !historyHas(ps, place{kind: KindProjectMemory, root: ProjectDir(pB), rel: "EXECAI.md", scope: "ws-1"}, []byte("build: make\n")) ||
		!historyHas(ps, place{kind: KindMemory, rel: "fact.md"}, []byte("v1\n")) {
		t.Errorf("the replaced versions are not in the history")
	}
	if strings.Contains(out, "local-") || strings.Contains(out, "history") {
		t.Errorf("a routine replacement was announced: %s", out)
	}
	if st := b.svc.Status(); !strings.Contains(st, "Earlier versions of files, replaced or removed by your other computers: ") {
		t.Errorf("status does not count the history:\n%s", st)
	}
}

// crossAllowed — functions of the packages sync calls that write, rename or
// remove files (directly or through their own helpers), as sync calls them,
// and why each may: none of them writes a person's memory, settings or a
// project's .execai/ past the keeper. A new one fails
// TestKeeper_CalledPackagesAreListed until it is listed here with a reason.
var crossAllowed = map[string]string{
	"config.Dir": "makes the config folder if missing; migrateLegacy copies config.json and " +
		"credentials.json from ~/.config/agent-vbai only where the new folder has none (atomic " +
		"writes, never over a file) and moves the old folder aside (renamed, nothing deleted)",
	"config.LoadCredentials": "only through config.Dir (as above); reads credentials.json",
	"config.PaymentURLHere":  "only through config.Dir (as above), for the installation's address",
	"crypto.SaveIdentity":    "the sync key, sync.key — sync's own file (keeperAllowed: replaceKey, onFlow.Answer)",
	"Checker.Ensure":         "plan.Checker: the plan cache, not a person's file",
	"Checker.EnsureDaily":    "plan.Checker: the plan cache, not a person's file",
	"Checker.Allows":         "plan.Checker: the plan cache, not a person's file",
}

// crossPackages — the packages of this module sync imports.
var crossPackages = []string{"config", "crypto", "subscriptions", "plan", "i18n", "version"}

// writers — the functions of the package in dir that write, rename or remove
// files, directly or through other functions of the same package (by name:
// a method is «Recv.Name», and is matched by its name alone where it is
// called, which can only over-report).
func writers(t *testing.T, dir string) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
	calls := map[string][]string{}
	direct := map[string]bool{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		af, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range af.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			fn := fd.Name.Name
			if fd.Recv != nil && len(fd.Recv.List) == 1 {
				fn = recvName(fd.Recv.List[0].Type) + "." + fn
			}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				name := calleeName(call.Fun)
				if keeperWatched[name] {
					direct[fn] = true
				}
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
					calls[fn] = append(calls[fn], "."+sel.Sel.Name)
				} else if name != "" {
					calls[fn] = append(calls[fn], name)
				}
				return true
			})
		}
	}
	out := map[string]bool{}
	for fn := range direct {
		out[fn] = true
	}
	matches := func(callee, fn string) bool {
		if strings.HasPrefix(callee, ".") {
			return strings.HasSuffix(fn, callee) && strings.Contains(fn, ".")
		}
		return callee == fn
	}
	for changed := true; changed; {
		changed = false
		for fn, cs := range calls {
			if out[fn] {
				continue
			}
			for _, c := range cs {
				for w := range out {
					if matches(c, w) {
						out[fn], changed = true, true
					}
				}
			}
		}
	}
	return out
}

// Н-8 of the second loss review: the sentinel of the one place that
// destroys covers the packages sync calls too. Every function of config,
// crypto, subscriptions, plan, i18n and version that writes files and that
// sync calls — as pkg.Func, or a method of one of their types by its name —
// is listed in crossAllowed with the reason it may.
func TestKeeper_CalledPackagesAreListed(t *testing.T) {
	writing := map[string]bool{} // "pkg.Func" and "Recv.Method"
	methods := map[string]string{}
	for _, p := range crossPackages {
		for fn := range writers(t, filepath.Join("..", p)) {
			if i := strings.Index(fn, "."); i > 0 {
				writing[fn] = true
				methods[fn[i+1:]] = fn
			} else {
				writing[p+"."+fn] = true
			}
		}
	}
	fset := token.NewFileSet()
	files, _ := filepath.Glob("*.go")
	seen := map[string]bool{}
	local := localMethods()
	var bad []string
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		af, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(af, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			name := calleeName(call.Fun)
			key := ""
			switch {
			case writing[name]:
				key = name
			case methods[sel.Sel.Name] != "":
				if id, ok := sel.X.(*ast.Ident); ok && slicesContains(crossPackages, id.Name) {
					return true // a package function, not a method
				}
				key = methods[sel.Sel.Name]
				if local[sel.Sel.Name] {
					return true // a method of this package's own type
				}
			default:
				return true
			}
			seen[key] = true
			if crossAllowed[key] == "" {
				bad = append(bad, fset.Position(call.Pos()).String()+": "+key+" writes files and is not listed in crossAllowed")
			}
			return true
		})
	}
	for k := range crossAllowed {
		if !seen[k] {
			bad = append(bad, "listed but not called (remove it): "+k)
		}
	}
	sort.Strings(bad)
	for _, b := range bad {
		t.Error(b)
	}
}

func slicesContains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// localMethods — the method names declared in this package.
func localMethods() map[string]bool {
	out := map[string]bool{}
	fset := token.NewFileSet()
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		af, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			continue
		}
		for _, d := range af.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv != nil {
				out[fd.Name.Name] = true
			}
		}
	}
	return out
}

// Н-8 of the second loss review: the mutation «entriesKept always says
// yes» was not caught. Writing an index over one keeps a copy exactly when
// an entry line of the old one is missing from the new one: an index that
// only gains lines (the merge of both sides' entries) replaces the old one
// without a copy; one that loses a line does not.
func TestKeeper_IndexWithAnEntryLessKeepsACopy(t *testing.T) {
	dir := t.TempDir()
	paths := Paths{ConfigDir: dir}
	rep := &Report{}
	k := &keeper{paths: paths, now: time.Now, rep: rep}
	pl := place{kind: KindMemory, root: paths.MemoryDir(), rel: "MEMORY.md"}
	old := []byte("<!-- the template -->\n- [Role](role.md)\n- [Fb](fb.md)\n")
	if err := writeFileAtomic(pl.file(), old); err != nil {
		t.Fatal(err)
	}
	gained := []byte("- [Role](role.md)\n- [Fb](fb.md)\n- [Plan](plan.md)\n")
	if _, err := k.put(pl, gained, time.Time{}, hashOf(old), keepHistory, reasonNone, false); err != nil {
		t.Fatal(err)
	}
	if rep.History != 0 || historyHas(paths, pl, old) {
		t.Fatalf("a copy of an index whose entries all stayed: %+v", rep)
	}
	lost := []byte("- [Role](role.md)\n- [Plan](plan.md)\n")
	if _, err := k.put(pl, lost, time.Time{}, hashOf(gained), keepHistory, reasonNone, false); err != nil {
		t.Fatal(err)
	}
	if rep.History != 1 || !historyHas(paths, pl, gained) {
		t.Fatalf("an index that lost an entry replaced without a copy: %+v", rep)
	}
	if !entriesKept([]byte("- a\n\n<!-- - b -->\n"), []byte("- a\n- c\n")) || entriesKept([]byte("- a\n- b\n"), []byte("- a\n")) {
		t.Error("entriesKept: lines inside a comment and blank lines are not entries; every other line is")
	}
}
