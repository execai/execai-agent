package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/crypto"
)

// The table of threeway.go, row by row: once as decide() itself over every
// combination of the three sides, the folder's trust and the rest, against
// the table written out again here from the task (not from the code); then
// end to end, a project file and a memory note in each combination of the
// three sides × trust, through real exchanges against the stub.

// tableWant — what the table says for one file. Written from the table of
// the task (round 4: rows 2a, 2b, 2c and the return told by its sender),
// in its own words: it must not be read off decide().
func tableWant(l, b, s fside, trust trustLevel, absenceDeletes, rollback, sReturn bool) decision {
	eq := func(x, y fside) bool { // both missing (or deleted) or the same content
		if !x.exists || !y.exists {
			return !x.exists && !y.exists
		}
		return x.hash == y.hash
	}
	switch {
	case eq(l, s): // 1
		return decision{v: vKeep}
	case rollback && l.exists && trust != trustFull && !(b.known && eq(l, b)): // an older copy put back
		return decision{v: vTake, keep: keepHistory, reason: reasonRollback}
	case !b.known && !l.exists: // 5b
		return decision{v: vTake}
	case !b.known && s.deleted: // 5d
		return decision{v: vResend, reason: reasonResent}
	case !b.known && !s.exists: // 5a
		return decision{v: vSend}
	case !b.known: // 5c
		return decision{v: vTake, keep: keepUnsent, reason: reasonNoBase}
	case eq(l, b) && !s.exists: // 2c: deleted elsewhere — removed, history, said
		return decision{v: vTake, keep: keepHistory, reason: reasonDeletedThere}
	case eq(l, b) && sReturn: // 2b: a return elsewhere — the newer kept for good, said
		return decision{v: vTake, keep: keepUnsent, reason: reasonReverted}
	case eq(l, b): // 2a: proven out of date — silently, history
		return decision{v: vTake, keep: keepHistory}
	case eq(s, b) && !l.exists && trust == trustFull && absenceDeletes: // 3a, a deletion
		return decision{v: vSend}
	case eq(s, b) && !l.exists: // 3b (and a trusted path): not a deletion
		return decision{v: vTake, reason: reasonRestored}
	case eq(s, b) && trust == trustFull && rollback: // 3a, a return here: sent, said
		return decision{v: vSend, reason: reasonReturnSent}
	case eq(s, b) && trust != trustNone: // 3a
		return decision{v: vSend}
	case eq(s, b): // 3b
		return decision{v: vTake, keep: keepUnsent, reason: reasonUntrusted}
	case !s.exists: // 4a
		return decision{v: vResend, reason: reasonResent}
	case !l.exists: // 4b
		return decision{v: vTake}
	}
	return decision{v: vTake, keep: keepUnsent, reason: reasonBoth} // 4
}

func TestTable_DecideEveryCombination(t *testing.T) {
	hashes := []string{"h1", "h2", "h3"}
	var ls, bs, ss []fside
	ls = append(ls, absent())
	bs = append(bs, noBase(), absent())
	ss = append(ss, absent(), fside{known: true, deleted: true})
	for _, h := range hashes {
		ls = append(ls, present(h))
		bs = append(bs, present(h))
		ss = append(ss, present(h))
	}
	n := 0
	for _, l := range ls {
		for _, b := range bs {
			for _, s := range ss {
				for _, trust := range []trustLevel{trustNone, trustPath, trustFull} {
					for _, del := range []bool{false, true} {
						if del && trust != trustFull {
							continue // the callers say so only of a folder trusted by its identity
						}
						for _, rb := range []bool{false, true} {
							for _, ret := range []bool{false, true} {
								if ret && !(s.exists && b.exists && s.hash != b.hash) {
									continue // a return is an earlier base version the server has now
								}
								n++
								got := decide(l, b, s, dctx{trust: trust, absenceDeletes: del, rollback: rb, sReturn: ret})
								want := tableWant(l, b, s, trust, del, rb, ret)
								if got != want {
									t.Errorf("l=%+v b=%+v s=%+v trust=%d absenceDeletes=%v rollback=%v return=%v: got %+v, want %+v",
										l, b, s, trust, del, rb, ret, got, want)
								}
							}
						}
					}
				}
			}
		}
	}
	if n != 992 {
		t.Fatalf("%d combinations, want 992", n)
	}
}

// What the person is told, by reason (the English lines).
var toldFor = map[reason]string{
	reasonUntrusted:    "this folder is not the one sync compared last time",
	reasonBoth:         "changed here and on the server",
	reasonNoBase:       "there was no agreed earlier version",
	reasonRestored:     "missing here",
	reasonResent:       "deleted on the server but changed here",
	reasonDeletedThere: "deleted on another computer",
}

// toldForProject — toldFor for a project file: a file a version lacks is
// said without a cause (deleted elsewhere, or an older version).
func toldForProject() map[reason]string {
	out := map[reason]string{}
	for r, line := range toldFor {
		out[r] = line
	}
	out[reasonDeletedThere] = "not in the version that came from the server"
	return out
}

// serverPack — the files of the project's pack on the server.
func serverPack(t *testing.T, stub *stubServer, key *crypto.Identity, ws string) map[string]string {
	t.Helper()
	bl := stub.blob(KindProjectMemory, ws)
	if bl == nil || bl.deleted {
		return nil
	}
	_, _, data, err := unseal(key, bl.content)
	if err != nil {
		t.Fatal(err)
	}
	files, err := unpackFiles(data)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for k, v := range files {
		out[k] = string(v)
	}
	return out
}

// serverNote — a memory note on the server: its content, or "<deleted>",
// or "" when the server has none.
func serverNote(t *testing.T, stub *stubServer, key *crypto.Identity, rel string) string {
	t.Helper()
	bl := stub.blob(KindMemory, memoryName(key, rel))
	switch {
	case bl == nil:
		return ""
	case bl.deleted:
		return "<deleted>"
	}
	_, _, data, err := unseal(key, bl.content)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func trustName(tr trustLevel) string {
	return map[trustLevel]string{trustNone: "another folder", trustPath: "path only", trustFull: "same folder"}[tr]
}

// A project file, end to end: f.md in every combination of here (l), the
// base (b) and the server (s), in the same folder, in a folder trusted by
// its path only (no identity), and in another folder (the binding moved to
// a copy). "" — no file. For a project a file the base manifest does not
// name has no base.
func TestTable_ProjectFileEndToEnd(t *testing.T) {
	type cell struct{ b, s, l string }
	var cells []cell
	for _, s := range []string{"", "server"} {
		for _, l := range []string{"", "local", "server"} {
			cells = append(cells, cell{"", s, l})
		}
	}
	for _, s := range []string{"base", "server", ""} {
		for _, l := range []string{"", "base", "local", "server"} {
			cells = append(cells, cell{"base", s, l})
		}
	}
	side := func(v string) fside {
		if v == "" {
			return absent()
		}
		return present(v)
	}
	for _, tr := range []trustLevel{trustFull, trustPath, trustNone} {
		for _, c := range cells {
			c, tr := c, tr
			t.Run(fmt.Sprintf("%s/b=%q s=%q l=%q", trustName(tr), c.b, c.s, c.l), func(t *testing.T) {
				stub, a, b, pA, pB := projectPair(t)
				nowA, nowB := identityFor(t, tr, a, b)
				put := func(dir, v string) {
					if v == "" {
						_ = os.Remove(filepath.Join(ProjectDir(dir), "f.md"))
						return
					}
					writeProjectFile(t, dir, "f.md", v+"\n", time.Time{})
				}
				writeProjectFile(t, pA, "EXECAI.md", "anchor\n", time.Time{})
				put(pA, c.b)
				nowA()
				nowB()
				if c.s != c.b {
					put(pA, c.s)
					nowA()
				}
				put(pB, c.l)
				dirB := pB
				if tr == trustNone {
					dirB = t.TempDir()
					writeProjectFile(t, dirB, "EXECAI.md", "anchor\n", time.Time{})
					put(dirB, c.l)
					rebind(stub, 1, dirB)
				}
				a.use()
				key, err := crypto.LoadIdentity(a.dir())
				if err != nil {
					t.Fatal(err)
				}
				v := serverVersion(stub)
				out := nowB()
				assertI1(t, stub, b)

				bb := noBase()
				if c.b != "" {
					bb = present(c.b)
				}
				want := tableWant(side(c.l), bb, side(c.s), tr, tr == trustFull, false, false)
				gotHere := strings.TrimSuffix(readProjectFile(dirB, "f.md"), "\n")
				if gotHere == "<missing>" {
					gotHere = ""
				}
				gotServer := strings.TrimSuffix(serverPack(t, stub, key, "ws-1")["f.md"], "\n")
				wantHere, wantServer := c.l, c.s
				switch want.v {
				case vTake:
					wantHere = c.s
				case vSend, vResend:
					wantServer = c.l
				}
				if gotHere != wantHere || gotServer != wantServer {
					t.Errorf("row %+v: here %q server %q, want here %q server %q\n%s", want, gotHere, gotServer, wantHere, wantServer, out)
				}
				if (want.v == vSend || want.v == vResend) != (serverVersion(stub) != v) {
					t.Errorf("row %+v: server version %d → %d", want, v, serverVersion(stub))
				}
				copies := copiesIn(ProjectDir(dirB))
				b.use()
				hist := historyHas(b.svc.Paths, place{kind: KindProjectMemory, rel: "f.md", scope: "ws-1"}, []byte(c.l+"\n"))
				switch want.keep {
				case keepUnsent:
					if len(copies) != 1 || !hasCopyOf(ProjectDir(dirB), c.l+"\n") {
						t.Errorf("row %+v: the unsent version is not kept next to the file: %v", want, copies)
					}
				case keepHistory:
					if len(copies) != 0 || (c.l != "" && !hist) {
						t.Errorf("row %+v: copies %v, in the history %v", want, copies, hist)
					}
				default:
					if len(copies) != 0 || hist {
						t.Errorf("row %+v: copies %v, in the history %v", want, copies, hist)
					}
				}
				for r, line := range toldForProject() {
					if strings.Contains(out, line) != (want.reason == r) {
						t.Errorf("row %+v: told %q: %v\n%s", want, line, !(want.reason == r), out)
					}
				}
			})
		}
	}
}

// A memory note, end to end: f.md in every combination of here, the base
// (none, a deletion agreed, a version) and the server (none, a deletion
// mark, a version), in the same memory folder, with the path trusted only
// (no identity), and in a memory folder made again (another identity).
func TestTable_MemoryNoteEndToEnd(t *testing.T) {
	const del = "<deleted>"
	type cell struct{ b, s, l string }
	var cells []cell
	for _, s := range []string{"", del, "server"} {
		for _, l := range []string{"", "local", "server"} {
			cells = append(cells, cell{"", s, l})
		}
	}
	for _, s := range []string{del, "server"} {
		for _, l := range []string{"", "local", "server"} {
			cells = append(cells, cell{del, s, l})
		}
	}
	for _, s := range []string{"base", "server", del} {
		for _, l := range []string{"", "base", "local", "server"} {
			cells = append(cells, cell{"base", s, l})
		}
	}
	side := func(v string) fside {
		switch v {
		case "":
			return absent()
		case del:
			return fside{known: true, deleted: true}
		}
		return present(v)
	}
	for _, tr := range []trustLevel{trustFull, trustPath, trustNone} {
		for _, c := range cells {
			c, tr := c, tr
			t.Run(fmt.Sprintf("%s/b=%q s=%q l=%q", trustName(tr), c.b, c.s, c.l), func(t *testing.T) {
				stub := newStub(t)
				a := newMachine(t, stub, "a")
				a.converse(a.svc.OnFlow(), password, password)
				b := newMachine(t, stub, "b")
				b.converse(b.svc.OnFlow(), password)
				nowA, nowB := identityFor(t, tr, a, b)
				put := func(m *machine, v string) {
					m.use()
					if v == "" || v == del {
						_ = os.Remove(filepath.Join(m.dir(), "memory", "f.md"))
						return
					}
					m.writeFile("memory/f.md", v+"\n")
				}
				a.use()
				a.writeFile("memory/MEMORY.md", "- [Anchor](anchor.md)\n- [F](f.md)\n")
				a.writeFile("memory/anchor.md", "anchor\n")
				switch c.b {
				case "base":
					put(a, "base")
				case del:
					put(a, "gone")
					nowA()
					nowB()
					put(a, "")
				}
				nowA()
				nowB() // B agrees the base (for "": the index and the anchor only)
				switch {
				case c.s == c.b:
				case c.s == del:
					if c.b == "" {
						put(a, "never seen by B")
						nowA()
					}
					put(a, "")
					nowA()
				case c.s != "":
					put(a, c.s)
					nowA()
				}
				put(b, c.l)
				if tr == trustNone {
					b.use()
					mem := filepath.Join(b.dir(), "memory")
					tmp := mem + ".away"
					if err := os.Rename(mem, tmp); err != nil {
						t.Fatal(err)
					}
					if err := os.MkdirAll(mem, 0o700); err != nil {
						t.Fatal(err)
					}
					entries, _ := os.ReadDir(tmp)
					for _, e := range entries {
						data, _ := os.ReadFile(filepath.Join(tmp, e.Name()))
						_ = os.WriteFile(filepath.Join(mem, e.Name()), data, 0o600)
					}
					_ = os.RemoveAll(tmp)
				}
				a.use()
				key, err := crypto.LoadIdentity(a.dir())
				if err != nil {
					t.Fatal(err)
				}
				before := serverNote(t, stub, key, "f.md")
				if want := c.s; strings.TrimSuffix(before, "\n") != want {
					t.Fatalf("setup: the server holds %q, want %q", before, want)
				}
				out := nowB()
				assertI1(t, stub, b)

				bb := noBase()
				switch c.b {
				case del:
					bb = absent()
				case "base":
					bb = present("base")
				}
				want := tableWant(side(c.l), bb, side(c.s), tr, tr == trustFull, false, false)
				b.use()
				gotHere := strings.TrimSuffix(b.readFile("memory/f.md"), "\n")
				if gotHere == "<missing>" {
					gotHere = ""
				}
				gotServer := strings.TrimSuffix(serverNote(t, stub, key, "f.md"), "\n")
				wantHere, wantServer := c.l, c.s
				switch want.v {
				case vTake:
					wantHere = c.s
					if c.s == del {
						wantHere = ""
					}
				case vSend:
					wantServer = c.l
					if c.l == "" {
						wantServer = del
					}
				case vResend:
					wantServer = c.l
				}
				if gotHere != wantHere || gotServer != wantServer {
					t.Errorf("row %+v: here %q server %q, want here %q server %q\n%s", want, gotHere, gotServer, wantHere, wantServer, out)
				}
				var unsent []string
				for p, content := range b.conflictFiles() {
					if content == c.l+"\n" {
						unsent = append(unsent, p)
					}
				}
				hist := historyHas(b.svc.Paths, place{kind: KindMemory, rel: "f.md"}, []byte(c.l+"\n"))
				switch want.keep {
				case keepUnsent:
					if len(unsent) != 1 {
						t.Errorf("row %+v: the unsent version is not kept: %v", want, b.conflictFiles())
					}
				case keepHistory:
					if len(unsent) != 0 || (c.l != "" && !hist) {
						t.Errorf("row %+v: unsent copies %v, in the history %v", want, unsent, hist)
					}
				default:
					if len(unsent) != 0 || hist {
						t.Errorf("row %+v: unsent copies %v, in the history %v", want, unsent, hist)
					}
				}
				for r, line := range toldFor {
					if strings.Contains(out, line) != (want.reason == r) {
						t.Errorf("row %+v: told %q: %v\n%s", want, line, !(want.reason == r), out)
					}
				}
			})
		}
	}
}

// identityFor — exchanges of A and B for a trust level: for «path only» B
// reads no folder identity (its records then hold none), while A's own
// exchanges — the setup, its deletions — keep theirs.
func identityFor(t *testing.T, tr trustLevel, a, b *machine) (nowA func() string, nowB func() string) {
	t.Helper()
	old := dirIdentityFunc
	on := true
	dirIdentityFunc = func(p string) string {
		if !on {
			return ""
		}
		return old(p)
	}
	t.Cleanup(func() { dirIdentityFunc = old })
	nowA = func() string { on = true; return now(a) }
	nowB = func() string { on = tr != trustPath; return now(b) }
	return
}

// conflictFiles — the permanent copies in <config>/sync-unsent: path →
// content.
func (m *machine) conflictFiles() map[string]string {
	out := map[string]string{}
	_ = filepath.WalkDir(filepath.Join(m.dir(), "sync-unsent"), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			data, _ := os.ReadFile(p)
			out[p] = string(data)
		}
		return nil
	})
	return out
}
