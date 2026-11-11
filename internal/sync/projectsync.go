package sync

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"strings"
)

// A bound project: its whole .execai/ travels as one pack, but each file is
// decided on its own (threeway.go) against the base manifest of the project
// (bases.go). The result — this side's files where they stay, the server's
// where it wins — is the pack that goes up, and only if it differs from the
// server's. So A editing one file and B another gives both of them both
// edits.
//
// The order: decide every file; send the merged pack if it differs from the
// server's; only then write here what came from the server; and record the
// base only once the files are in place. A version written on the server
// meanwhile (409) makes the exchange start over from the comparison with
// nothing written here yet, so no copy is left from the attempt that did
// not go through. An exchange that stops anywhere — a crash, a full disk, a
// file that cannot be written — leaves the previous base, and the next one
// comes to the same result from what is on disk and on the server: a file
// already written is the server's (row 1), one not yet written is still the
// base (row 2), a pack sent but not recorded is the server's (row 1). The
// one case that needs a step before writing: files missing here that are
// not taken for deletions (an emptied .execai/, a folder not trusted) lose
// their base first, on disk, so a half-done restore is not then read as
// files deleted here.
//
// A deletion mark of the whole project (an older agent sends it when its
// folder moves) never deletes the files here, and leaves no base: a live
// version coming later is merged as with nothing to compare (rows 5). While
// the mark stands, the files here that are all the very ones the mark
// deleted (the last live base) stay where they are, unsent, and the person
// is told once; content the server never saw — a new note, an edit, the
// .execai/ of a computer that joins — brings the project back: everything
// here goes up again (5d).
//
// A file changed while the exchange decided about it (the model saves
// EXECAI.md meanwhile) is not touched; then the base stays: the next
// exchange decides again. Entries left out here (a link, a file that cannot
// be read), names that are a file on one side and a folder on the other,
// and names that differ only in case where the file system does not tell
// them apart are left as they are, both here and on the server.

func (e *Engine) syncProject(ctx context.Context, st *State, bs *Bases, ws, dir string, meta *Meta, full bool,
	sc *scan, prior map[string]string, opt Options, rep *Report) error {
	key := blobKey(KindProjectMemory, ws)
	if sc.skipped[key] {
		return nil
	}
	pd := ProjectDir(dir)
	li, here := sc.items[key]
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			it, ok, sk := e.projectScan(ws, dir)
			if sk != nil {
				rep.Skipped = append(rep.Skipped, *sk)
				return nil
			}
			li, here = it, ok
		}
		b, had := st.Blobs[key]
		trust, rec := bs.projectTrust(ws, dir, b, prior)
		if !full && (!here || (had && !b.Deleted && b.Hash != "" && b.Hash == li.Hash && len(li.Left) == 0)) {
			return nil // a send-only round: nothing changed here
		}
		if full && attempt == 0 && meta != nil && had && !b.Deleted && !meta.Deleted && meta.Version == b.Version &&
			here && li.Hash == b.Hash && len(li.Left) == 0 && trust == trustFull && rec.Files != nil && rec.describes(b) &&
			samePath(rec.Dir, dir) {
			return nil // nothing changed anywhere
		}

		var f fetched
		switch {
		case full && attempt == 0 && meta == nil:
			f.none = true
		case full && attempt == 0 && meta.Deleted:
			f = fetched{gone: true, ver: meta.Version}
		default:
			var err error
			lv := int64(0)
			if meta != nil {
				lv = meta.Version
			}
			if f, err = e.fetch(ctx, KindProjectMemory, ws, lv); err != nil {
				return err
			}
			if f.none && (meta != nil || attempt > 0 || (!full && had)) {
				// Listed (or just written) but not there to read: a failure
				// of the moment, not a deletion. Nothing is decided now.
				return nil
			}
		}
		if f.hidden {
			rep.hide(key)
			return nil
		}
		var L map[string][]byte
		if here {
			var err error
			if L, err = unpackFiles(li.Data); err != nil {
				return err
			}
		}

		if f.gone {
			return e.projectMarkedDeleted(ctx, st, bs, ws, dir, key, li, here, L, b, had, rec, f.ver, rep)
		}

		var S map[string][]byte
		sHash := ""
		if !f.none {
			var err error
			if S, err = unpackFiles(f.data); err != nil {
				return err
			}
			sHash = hashOf(f.data)
		}
		var B map[string]string
		if !f.none && had && !b.Deleted {
			// After a deletion mark there is no base: rows 5.
			B = manifestFor(rec, b, had, L, S, li.Hash, sHash)
		}

		absenceDeletes := trust == trustFull && here && hasContent(L)
		fold := caseInsensitive(pd)
		rels := map[string]bool{}
		for rel := range L {
			rels[rel] = true
		}
		for rel := range S {
			rels[rel] = true
		}
		for rel := range B {
			rels[rel] = true
		}
		for _, rel := range li.Left {
			rels[rel] = true
		}
		folded := map[string]int{}
		if fold {
			for rel := range rels {
				folded[strings.ToLower(rel)]++
			}
		}
		clash := clashingNames(rels)
		M := map[string][]byte{}
		unbased := map[string]bool{} // names left out: they keep the base they had
		type job struct {
			rel string
			d   decision
		}
		var jobs []job
		var restoring []string
		var newHere []string // files only here, going up (5a)
		for _, rel := range sortedKeys(rels) {
			sv, sHas := S[rel]
			if !safeRel(rel) || localOnlyRel(rel) {
				if sHas {
					M[rel] = sv // not this side's to change
				}
				continue
			}
			left := isLeft(li.Left, rel)
			collide := fold && folded[strings.ToLower(rel)] > 1
			if left || clash[rel] || collide {
				// Left as it is here and on the server: the server's
				// version goes back up unchanged, nothing is written here,
				// and the name keeps the base it had (below).
				if sHas {
					M[rel] = sv
				}
				unbased[rel] = true
				switch {
				case clash[rel] && (sHas || L[rel] != nil):
					rep.Clashes = appendKind(rep.Clashes, relFile(pd, clashRoot(rel, clash)))
				case collide && !left:
					if _, onDisk := L[rel]; !onDisk || sHas {
						rep.Collisions = append(rep.Collisions, rel)
					}
				}
				continue
			}
			l := absent()
			if v, ok := L[rel]; ok {
				l = present(hashOf(v))
			}
			bb := noBase() // a file the manifest does not name has no base
			if h, ok := B[rel]; ok {
				bb = present(h)
			}
			s := absent()
			if sHas {
				s = present(hashOf(sv))
			}
			rollback := l.exists && inList(rec.Prev[rel], l.hash)
			sReturn := s.exists && bb.exists && s.hash != bb.hash && inList(rec.Prev[rel], s.hash)
			d := decide(l, bb, s, dctx{trust: trust, absenceDeletes: absenceDeletes, rollback: rollback, sReturn: sReturn})
			if d.v == vSend && !bb.known && !s.exists && l.exists {
				newHere = append(newHere, rel)
			}
			if d.v == vTake && !l.exists && !bb.known && sHas && B != nil && len(rec.Prev[rel]) > 0 {
				// Deleted here, the deletion agreed (the name has earlier
				// versions and no base), and on the server again.
				d.reason = reasonCameBack
			}
			switch d.v {
			case vKeep, vSend, vResend:
				if v, ok := L[rel]; ok {
					M[rel] = v
				} else if d.v == vKeep && sHas {
					M[rel] = sv
				}
			case vTake:
				if sHas {
					M[rel] = sv
				}
				if d.reason == reasonRestored && bb.exists {
					restoring = append(restoring, rel)
				}
			}
			jobs = append(jobs, job{rel, d})
		}

		// An older snapshot (threeway.go): a version with a return in it
		// removes nothing silently — what it lacks is kept for good, and
		// the project is said in one line.
		back, gone := false, false
		for _, j := range jobs {
			back = back || j.d.reason == reasonReverted
			gone = gone || j.d.reason == reasonDeletedThere
		}
		if back && gone {
			for i := range jobs {
				switch jobs[i].d.reason {
				case reasonReverted:
					jobs[i].d.reason = reasonOlderBack
				case reasonDeletedThere:
					jobs[i].d.keep, jobs[i].d.reason = keepUnsent, reasonOlderGone
				}
			}
		}

		mData, err := packFiles(M)
		if err != nil {
			return err
		}
		mHash := hashOf(mData)

		// Up first: nothing is written here before the server took the
		// merge, so a version written meanwhile starts the comparison over
		// with nothing to undo.
		newVer := f.ver
		switch {
		case !f.none && mHash == sHash:
		case len(M) == 0:
			return nil // never a pack with nothing in it
		default:
			v, ok, err := e.upload(ctx, KindProjectMemory, ws, ws, pd, mData, f.ver, rep)
			var ce *ConflictError
			if errors.As(err, &ce) {
				continue // written meanwhile: compare again with what is there now
			}
			if err != nil || !ok {
				return err
			}
			newVer = v
		}

		if len(restoring) > 0 && B != nil {
			r := rec
			r.Version, r.Hash, r.Deleted = b.Version, b.Hash, b.Deleted
			bs.forgetFiles(ws, r, B, restoring)
			if err := saveBases(e.Paths, bs); err != nil {
				return err
			}
		}

		k := e.keeper(rep)
		changed, wrote := false, false
		var removed []string
		for _, j := range jobs {
			if j.d.v != vTake {
				switch j.d.reason {
				case reasonResent:
					if e.newKey {
						rep.SentNewKey = addArea(rep.SentNewKey, KindProjectMemory, pd, relFile(pd, j.rel))
					} else {
						rep.Resent = addArea(rep.Resent, KindProjectMemory, pd, relFile(pd, j.rel))
					}
				case reasonReturnSent:
					rep.ReturnSent = addArea(rep.ReturnSent, KindProjectMemory, pd, relFile(pd, j.rel))
				}
				continue
			}
			pl := place{kind: KindProjectMemory, root: pd, rel: j.rel, scope: ws}
			expect := ""
			if v, ok := L[j.rel]; ok {
				expect = hashOf(v)
			}
			var err error
			if sv, ok := S[j.rel]; ok {
				_, err = k.put(pl, sv, f.upd, expect, j.d.keep, j.d.reason, false)
				wrote = true
			} else {
				err = k.remove(pl, expect, j.d.keep, j.d.reason)
				if expect != "" && err == nil {
					removed = append(removed, j.rel)
				}
			}
			if isChanged(err) {
				changed = true
				rep.Changed = addArea(rep.Changed, KindProjectMemory, pd, pl.file())
				continue
			}
			if err != nil && isLocalErr(err) {
				// Cannot be written here (no permission, a full disk): this
				// project waits, the base stays.
				changed = true
				rep.Skipped = append(rep.Skipped, Skip{Path: pl.file(), Reason: SkipUnreadable, Cause: causeWrite})
				continue
			}
			if err != nil {
				return err
			}
			switch j.d.reason {
			case reasonRestored:
				rep.Restored = addArea(rep.Restored, KindProjectMemory, pd, pl.file())
			case reasonRollback:
				if _, ok := S[j.rel]; ok {
					rep.Rollback = addArea(rep.Rollback, KindProjectMemory, pd, pl.file())
				} else {
					rep.RollbackGone = addArea(rep.RollbackGone, KindProjectMemory, pd, pl.file())
				}
			case reasonDeletedThere:
				if expect != "" {
					rep.DeletedThere = addArea(rep.DeletedThere, KindProjectMemory, pd, pl.file())
				}
			case reasonCameBack:
				rep.CameBack = addArea(rep.CameBack, KindProjectMemory, pd, pl.file())
			case reasonBoth:
				rep.Conflicts++
			}
		}
		if len(removed) > 0 {
			pruneEmptyDirs(pd, removed)
			rep.Removed++
		}
		copied := false
		for _, j := range jobs {
			copied = copied || (j.d.keep == keepUnsent && (j.d.reason == reasonUntrusted || j.d.reason == reasonNoBase))
		}
		if copied && len(newHere) > 0 {
			// Said next to the copy: the same folder sent what the server
			// did not have.
			sent := areaCount{kind: KindProjectMemory, root: pd}
			for _, rel := range newHere {
				sent.files = append(sent.files, relFile(pd, rel))
			}
			rep.SentNew = append(rep.SentNew, sent)
		}
		if changed {
			return nil // decided again next time; the base stays
		}
		nb := BlobState{Path: ws, Version: newVer, Hash: mHash}
		st.Blobs[key] = nb
		files := hashFiles(M)
		for rel := range unbased {
			// A name left out keeps the base it had — the version this
			// computer and the server last agreed on for that file, content
			// the server had (I1) — so once it is in the exchange again the
			// table decides it as it would have: an edit made here goes up,
			// a deletion elsewhere removes it here.
			if h, ok := B[rel]; ok {
				files[rel] = h
			} else {
				delete(files, rel)
			}
		}
		bs.noteProject(ws, dir, nb, files)
		if wrote || len(removed) > 0 {
			rep.pulled(KindProjectMemory)
		}
		return nil
	}
	return nil
}

// clashingNames — names that are a file in one place and a folder in
// another (notes.md here, notes.md/x.md on the server): the name and
// everything under it.
func clashingNames(rels map[string]bool) map[string]bool {
	out := map[string]bool{}
	for rel := range rels {
		for d := parentOf(rel); d != ""; d = parentOf(d) {
			if rels[d] {
				out[d] = true
				out[rel] = true
			}
		}
	}
	if len(out) == 0 {
		return out
	}
	// Everything under a clashing file name clashes too.
	for rel := range rels {
		for c := range out {
			if strings.HasPrefix(rel, c+"/") {
				out[rel] = true
			}
		}
	}
	return out
}

// relFile — the file of rel (a slash path) under root.
func relFile(root, rel string) string { return filepath.Join(root, filepath.FromSlash(rel)) }

func parentOf(rel string) string {
	i := strings.LastIndex(rel, "/")
	if i < 0 {
		return ""
	}
	return rel[:i]
}

// clashRoot — the clashing name rel is or is under.
func clashRoot(rel string, clash map[string]bool) string {
	roots := []string{}
	for c := range clash {
		if rel == c || strings.HasPrefix(rel, c+"/") {
			roots = append(roots, c)
		}
	}
	sort.Strings(roots)
	if len(roots) == 0 {
		return rel
	}
	return roots[0]
}

// projectMarkedDeleted — the server holds a deletion mark for the whole
// project. The files here stay; there is no base any more. If every file
// here is the very one the mark deleted (the last live base), nothing goes
// up and the person is told once; if anything here is content the server
// never saw, the project comes back: all of this side's files go up again.
func (e *Engine) projectMarkedDeleted(ctx context.Context, st *State, bs *Bases, ws, dir, key string, li item, here bool,
	L map[string][]byte, b BlobState, had bool, rec ProjectBase, ver int64, rep *Report) error {
	nb := BlobState{Path: ws, Version: ver, Deleted: true}
	if !here || !hasContent(L) {
		st.Blobs[key] = nb
		bs.noteProject(ws, dir, nb, nil)
		return nil
	}
	lastLive := rec.LastLive
	if had && !b.Deleted && rec.Files != nil && rec.describes(b) {
		lastLive = rec.Files
	}
	var unseen []string
	for _, rel := range sortedKeys(L) {
		if lastLive == nil || lastLive[rel] != hashOf(L[rel]) {
			unseen = append(unseen, rel)
		}
	}
	if len(unseen) > 0 {
		// Content the server never saw: the project comes back (5d).
		v, ok, err := e.upload(ctx, KindProjectMemory, ws, ws, ProjectDir(dir), li.Data, ver, rep)
		var ce *ConflictError
		if errors.As(err, &ce) {
			return nil // the next exchange compares with what is there now
		}
		if err != nil || !ok {
			return err
		}
		live := BlobState{Path: ws, Version: v, Hash: li.Hash}
		st.Blobs[key] = live
		bs.noteProject(ws, dir, live, hashFiles(L))
		// One line for the project: all of it went up, not just what
		// the server never had.
		back := areaCount{kind: KindProjectMemory, root: ProjectDir(dir)}
		for _, rel := range sortedKeys(L) {
			back.files = append(back.files, relFile(ProjectDir(dir), rel))
		}
		if e.newKey {
			// The key changed here: the project goes up with it, which is
			// all that happened.
			rep.SentNewKey = append(rep.SentNewKey, back)
		} else {
			rep.ProjectsBack = append(rep.ProjectsBack, back)
		}
		return nil
	}
	if !(had && b.Deleted && b.Version == ver) {
		rep.ProjectsKept = append(rep.ProjectsKept, dir)
	}
	st.Blobs[key] = nb
	bs.noteProject(ws, dir, nb, nil)
	return nil
}

// oneFolderOneProject — one folder holds one project's memory: of two
// projects bound here to one folder (by identity, or by path), the one whose
// base was agreed in that folder stays (else the first by id); the other is
// left out and the person told which project has the folder.
func oneFolderOneProject(projects map[string]string, bs *Bases, names map[string]string) (map[string]string, []Skip) {
	out := map[string]string{}
	var notes []Skip
	ids := sortedKeys(projects)
	// Projects whose base was agreed in their folder go first.
	owner := func(ws string) bool { r, ok := bs.Projects[ws]; return ok && samePath(r.Dir, projects[ws]) }
	ordered := make([]string, 0, len(ids))
	for _, ws := range ids {
		if owner(ws) {
			ordered = append(ordered, ws)
		}
	}
	for _, ws := range ids {
		if !owner(ws) {
			ordered = append(ordered, ws)
		}
	}
	label := func(ws string) string {
		if n := names[ws]; n != "" {
			return n
		}
		return ws
	}
	for _, ws := range ordered {
		dir := projects[ws]
		taken := ""
		for other, odir := range out {
			if SameFolder(odir, dir) {
				taken = other
			}
		}
		if taken != "" {
			notes = append(notes, Skip{Path: dir, Reason: SkipFolderTaken, Detail: label(taken) + "\x00" + label(ws), Project: ws})
			continue
		}
		out[ws] = dir
	}
	return out, notes
}
