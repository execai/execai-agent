package sync

import (
	"context"
	"errors"
	"path"
	"sort"
	"strings"
	"time"
)

// The personal memory: one blob per file, its base per blob in State.Blobs,
// decided by threeway.go. What is particular to it:
//
//   - The folder is <config>/memory: trusted by its identity as recorded
//     after the last full exchange (bases.go), by the path where the
//     identity cannot be read or none is recorded.
//   - The index MEMORY.md is what the agent reads; a note without its line
//     is invisible. It is not decided as a file: it is merged entry by
//     entry after the notes (memindex.go), and an index without a single
//     entry never goes up. An index missing here, or one with no entry left
//     while its base had them (a template written over it), is «the index
//     is gone»: nothing missing here is taken for a deletion in that
//     exchange.
//   - A deletion mark on the server and the file here as the base had it is
//     the very file deleted elsewhere: it goes here too (into the history,
//     and the person is told). Content the server never saw stays and goes
//     up again.
//   - A note the server lists but does not give (404 on reading it) is a
//     failure of the moment, not a deletion: it is not decided in this
//     exchange.
//
// What a missing memory file means, by the folder and the index:
//
//	memory folder                         MEMORY.md here         missing files              send-only exchange
//	the same as recorded (identity)       there, with entries    deleted everywhere;        sends the deletions
//	                                                             the others keep history
//	the same                              gone, or no entries    back from the server       sends nothing of the memory
//	another folder                        —                      back from the server       sends nothing of what changed
//	the path only (no identity, none      —                      back from the server;      sends edits, not deletions
//	recorded)                                                    older copies replaced
//	not there                             —                      back from the server       sends nothing
//	a link, cannot be read                —                      nothing happens            nothing happens
//
// A send-only exchange (on a change, on exit) never takes anything from the
// server: what the server has decides that, in the next full exchange.

type memJob struct {
	k, name, path string
	li            item
	here, had     bool
	base          BlobState
	l, b, s       fside
	sVer          int64
	sData         []byte
	upd           time.Time
	d             decision
	rollback      bool
}

func (e *Engine) syncMemory(ctx context.Context, st *State, bs *Bases, remote map[string]Meta, sc *scan, opt Options, rep *Report) error {
	if !memoryOnlyRound(opt) || sc.memAll {
		return nil
	}
	memDir := e.Paths.MemoryDir()
	trust := trustNone
	if sc.mem.present {
		trust = bs.memoryTrust(memDir)
	}
	idx := blobKey(KindMemory, memoryName(e.ID, "MEMORY.md"))
	ib, ihad := st.Blobs[idx]
	il, ihere := sc.items[idx]
	indexGone := ihad && !ib.Deleted && !sc.skipped[idx] &&
		(!ihere || (len(indexEntries(il.Data)) == 0 && il.Hash != ib.Hash))
	absenceDeletes := trust == trustFull && sc.mem.present && !indexGone
	if remote == nil && indexGone {
		return nil // the next full exchange takes the index back first
	}

	keys := map[string]bool{}
	for k, it := range sc.items {
		if it.Kind == KindMemory {
			keys[k] = true
		}
	}
	for k := range st.Blobs {
		if kind, _ := splitKey(k); kind == KindMemory {
			keys[k] = true
		}
	}
	for k := range remote {
		if kind, _ := splitKey(k); kind == KindMemory {
			keys[k] = true
		}
	}
	delete(keys, idx) // merged entry by entry after the notes (memindex.go)

	// The notes sync knew before this exchange and finds here: one without
	// a line in the index gets one (memindex.go).
	var known []string
	for k, it := range sc.items {
		if b, had := st.Blobs[k]; it.Kind == KindMemory && k != idx && had && !b.Deleted &&
			strings.EqualFold(path.Ext(it.Path), ".md") && it.Path != "MEMORY.md" {
			known = append(known, it.Path)
		}
	}
	sort.Strings(known)

	var jobs []*memJob
	for _, k := range sortedKeys(keys) {
		if sc.skipped[k] {
			continue
		}
		_, name := splitKey(k)
		b, had := st.Blobs[k]
		li, here := sc.items[k]
		j := &memJob{k: k, name: name, li: li, here: here, had: had, base: b}
		j.path = b.Path
		if here {
			j.path = li.Path
		}
		if j.path != "" && sc.memSkipped(j.path) {
			continue
		}
		j.l = absent()
		if here {
			j.l = present(li.Hash)
		}
		switch {
		case !had:
			j.b = noBase()
		case b.Deleted:
			j.b = absent()
		default:
			j.b = present(b.Hash)
		}
		if remote == nil {
			if !here && !had {
				continue
			}
			j.s, j.sVer = j.b, b.Version
			if !had {
				j.s = absent()
			}
		} else {
			m, listed := remote[k]
			switch {
			case !listed:
				j.s = absent()
			case had && m.Version == b.Version:
				j.s, j.sVer = j.b, b.Version
				if b.Deleted {
					j.s.deleted = true
				}
			case m.Deleted:
				j.s, j.sVer = fside{known: true, deleted: true}, m.Version
			default:
				f, err := e.fetch(ctx, KindMemory, name, m.Version)
				if err != nil {
					return err
				}
				switch {
				case f.hidden:
					rep.hide(k)
					continue
				case f.none:
					continue // listed but not given: not a deletion; next time
				case f.gone:
					j.s, j.sVer = fside{known: true, deleted: true}, f.ver
				default:
					if sc.memSkipped(f.p) {
						continue // left out here (local.go)
					}
					j.path, j.s, j.sVer, j.sData, j.upd = f.p, present(hashOf(f.data)), f.ver, f.data, f.upd
				}
			}
		}
		if j.path == "" || j.path == "MEMORY.md" {
			continue
		}
		jobs = append(jobs, j)
	}

	if caseInsensitive(memDir) {
		// Names differing only in case — here and on the server, or twice
		// on the server: this file system keeps one file for them, so none
		// of them is written, removed or sent; left as they are, both ways.
		names := map[string]map[string]bool{}
		add := func(p string) {
			f := strings.ToLower(p)
			if names[f] == nil {
				names[f] = map[string]bool{}
			}
			names[f][p] = true
		}
		for _, it := range sc.items {
			if it.Kind == KindMemory {
				add(it.Path)
			}
		}
		for _, j := range jobs {
			add(j.path)
		}
		kept := jobs[:0]
		for _, j := range jobs {
			if len(names[strings.ToLower(j.path)]) > 1 {
				rep.Collisions = append(rep.Collisions, j.path)
				continue
			}
			kept = append(kept, j)
		}
		jobs = kept
	}

	decided := jobs[:0]
	for _, j := range jobs {
		e.decideMemory(j, bs, trust, absenceDeletes)
		if remote == nil && j.d.v == vTake {
			continue // a send-only exchange takes nothing: the next full one decides
		}
		if j.d.v == vTake && j.s.exists && j.sData == nil {
			// The server's version is the base's: its content is needed now.
			f, err := e.fetch(ctx, KindMemory, j.name, j.sVer)
			if err != nil {
				return err
			}
			if f.hidden || f.none || f.gone || hashOf(f.data) != j.s.hash {
				continue // not what the base says: the next exchange decides
			}
			j.sData, j.upd = f.data, f.upd
		}
		decided = append(decided, j)
	}
	jobs = decided

	// Files about to be written back because their absence is not a
	// deletion lose their base first, on disk: an exchange that stops half
	// way must find them without a base, not as deleted here.
	forgot := false
	for _, j := range jobs {
		if j.d.v == vTake && j.d.reason == reasonRestored && j.had {
			delete(st.Blobs, j.k)
			j.had, j.b = false, noBase()
			forgot = true
		}
	}
	if forgot {
		if err := st.Save(e.Paths); err != nil {
			return err
		}
	}
	for _, j := range jobs {
		if err := e.memoryJob(ctx, st, bs, j, trust, absenceDeletes, remote == nil, rep); err != nil {
			return err
		}
	}
	if !sc.skipped[idx] && !sc.memSkipped("MEMORY.md") {
		if err := e.syncMemoryIndex(ctx, st, bs, remote, idx, known, rep); err != nil {
			return err
		}
	}
	if remote != nil {
		bs.Memory = dirIdentity(memDir)
	}
	return nil
}

// decideMemory — the row of the table for one note (threeway.go).
func (e *Engine) decideMemory(j *memJob, bs *Bases, trust trustLevel, absenceDeletes bool) {
	j.rollback = j.l.exists && inList(bs.MemoryPrev[j.k], j.l.hash)
	sReturn := j.s.exists && j.b.exists && j.s.hash != j.b.hash && inList(bs.MemoryPrev[j.k], j.s.hash)
	j.d = decide(j.l, j.b, j.s, dctx{trust: trust, absenceDeletes: absenceDeletes, rollback: j.rollback, sReturn: sReturn})
	if j.d.v == vTake && !j.l.exists && j.had && j.base.Deleted && j.s.exists {
		j.d.reason = reasonCameBack // deleted here and agreed; on the server again
	}
}

// setMemBase records the new base of a memory file; the content of the
// base it replaces joins the file's earlier hashes.
func setMemBase(st *State, bs *Bases, k string, nb BlobState) {
	if old, ok := st.Blobs[k]; ok && !old.Deleted && old.Hash != "" && old.Hash != nb.Hash {
		bs.MemoryPrev[k] = pushPrev(bs.MemoryPrev[k], old.Hash)
	}
	st.Blobs[k] = nb
}

func (e *Engine) memoryJob(ctx context.Context, st *State, bs *Bases, j *memJob, trust trustLevel, absenceDeletes, sendOnly bool, rep *Report) error {
	k := e.keeper(rep)
	mp := place{kind: KindMemory, root: e.Paths.MemoryDir(), rel: j.path}
	expect := ""
	if j.here {
		expect = j.li.Hash
	}
	for attempt := 0; attempt < 3; attempt++ {
		var send []byte // what goes up, nil for a deletion
		switch j.d.v {
		case vKeep:
			switch {
			case j.s.exists:
				setMemBase(st, bs, j.k, BlobState{Path: j.path, Version: j.sVer, Hash: j.s.hash})
			case j.s.deleted:
				setMemBase(st, bs, j.k, BlobState{Path: j.path, Version: j.sVer, Deleted: true})
			}
			return nil
		case vTake:
			var err error
			if j.s.exists {
				_, err = k.put(mp, j.sData, j.upd, expect, j.d.keep, j.d.reason, false)
			} else {
				err = k.remove(mp, expect, j.d.keep, j.d.reason)
			}
			if isChanged(err) {
				rep.Changed = addArea(rep.Changed, KindMemory, mp.root, mp.file())
				return nil
			}
			if err != nil && isLocalErr(err) {
				rep.Skipped = append(rep.Skipped, Skip{Path: mp.file(), Reason: SkipUnreadable, Cause: causeWrite})
				return nil
			}
			if err != nil {
				return err
			}
			if j.d.reason == reasonBoth {
				rep.Conflicts++
			}
			if j.s.exists {
				setMemBase(st, bs, j.k, BlobState{Path: j.path, Version: j.sVer, Hash: j.s.hash})
				rep.pulled(KindMemory)
			} else {
				if j.sVer > 0 || j.s.deleted {
					setMemBase(st, bs, j.k, BlobState{Path: j.path, Version: j.sVer, Deleted: true})
				} else {
					delete(st.Blobs, j.k)
				}
				if j.here {
					rep.Removed++
					rep.PulledKinds = appendKind(rep.PulledKinds, KindMemory)
				}
			}
			switch j.d.reason {
			case reasonRestored:
				rep.Restored = addArea(rep.Restored, KindMemory, mp.root, mp.file())
			case reasonRollback:
				if j.s.exists {
					rep.Rollback = addArea(rep.Rollback, KindMemory, mp.root, mp.file())
				} else {
					rep.RollbackGone = addArea(rep.RollbackGone, KindMemory, mp.root, mp.file())
				}
			case reasonDeletedThere:
				if j.here {
					rep.DeletedThere = addArea(rep.DeletedThere, KindMemory, mp.root, mp.file())
				}
			case reasonCameBack:
				rep.CameBack = addArea(rep.CameBack, KindMemory, mp.root, mp.file())
			}
			return nil
		case vSend, vResend:
			if j.l.exists {
				send = j.li.Data
			}
		}
		// Up: this side's content (or its deletion) over the server's
		// version as seen; a version written meanwhile makes the decision
		// again on what is there now.
		var err error
		var v int64
		ok := true
		if send == nil {
			v, err = e.Client.Delete(ctx, KindMemory, j.name, j.sVer)
			if err == nil {
				crashPoint("sent")
			}
			if errors.Is(err, ErrNotFound) || errors.Is(err, ErrGone) {
				err = nil
				if v == 0 {
					v = j.sVer
				}
			}
		} else {
			v, ok, err = e.upload(ctx, KindMemory, j.name, j.path, mp.file(), send, j.sVer, rep)
		}
		var ce *ConflictError
		if errors.As(err, &ce) {
			f, ferr := e.fetch(ctx, KindMemory, j.name, ce.Version)
			if ferr != nil {
				return ferr
			}
			switch {
			case f.hidden:
				rep.hide(j.k)
				return nil
			case f.none:
				return nil // not there to read: the next exchange decides
			case f.gone:
				j.s, j.sVer, j.sData = fside{known: true, deleted: true}, f.ver, nil
			default:
				j.s, j.sVer, j.sData, j.upd = present(hashOf(f.data)), f.ver, f.data, f.upd
			}
			e.decideMemory(j, bs, trust, absenceDeletes)
			if sendOnly && j.d.v == vTake {
				return nil // a send-only exchange takes nothing: the next full one decides
			}
			continue
		}
		if err != nil || !ok {
			return err
		}
		if send == nil {
			setMemBase(st, bs, j.k, BlobState{Path: j.path, Version: v, Deleted: true})
			rep.Deleted++
		} else {
			setMemBase(st, bs, j.k, BlobState{Path: j.path, Version: v, Hash: hashOf(send)})
		}
		switch j.d.reason {
		case reasonResent:
			if e.newKey {
				rep.SentNewKey = addArea(rep.SentNewKey, KindMemory, mp.root, mp.file())
			} else {
				rep.Resent = addArea(rep.Resent, KindMemory, mp.root, mp.file())
			}
		case reasonReturnSent:
			rep.ReturnSent = addArea(rep.ReturnSent, KindMemory, mp.root, mp.file())
		}
		return nil
	}
	return nil
}
