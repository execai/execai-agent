package sync

// Which version of a file stays: here (l), the base (b — what this
// computer last agreed with the server) and the server (s), compared by
// the hash of their content. The time of a file — on disk or on the server
// — is never part of it: a clone, an unpacked archive, a copy from a backup
// or a wrong clock give any time at all. Times are only set on files
// written and shown to the person.
//
// Two invariants hold the table together:
//
//	I1. The base is only what certainly is on the server: content taken
//	    from the server and written here, or sent from here and taken by the
//	    server. Content the server never saw is never a base, under no mark.
//	I2. A file here is replaced or removed silently only when it is proven
//	    out of date: it is the base, the base is live (not a project marked
//	    deleted), the version that came is not a return to an earlier one,
//	    and it is a replacement, not a deletion. Everything else is said.
//
// Besides the three sides: how far the folder here is the one the base was
// agreed in (trust, bases.go), whether a missing file here is a deletion,
// whether the content here is one of the file's earlier base versions
// (rollback), and whether the server's is (a return).
//
//	 #   l, b, s                          what is done                          copy of this side
//	 1   l = s                            nothing; base := s                    —
//	 2a  l = b, s ≠ b, s there, no return the server's version here             history, silently
//	 2b  l = b, s an earlier base version the server's version here             next to it for good, told
//	 2c  l = b, s gone                    removed here                          history (30 days), told
//	 3a  s = b, l ≠ b, folder trusted     this side's goes up (a deletion too)  — (a return: told)
//	 3b  s = b, l ≠ b, not trusted        l missing: back from the server;      next to it for good, told
//	                                      l there: the server's stays
//	 4   l ≠ b, s ≠ b, l ≠ s              the server's stays                    next to it for good, told
//	 4a  s deleted, l ≠ b                 this side's stays and goes up again   — (told)
//	 4b  l missing, s ≠ b                 the server's version here             —
//	 5a  no base, only l                  goes up                               —
//	 5b  no base, only s                  taken                                 —
//	 5c  no base, l ≠ s                   the server's stays                    next to it for good, told
//	 5d  no base, s deleted, l there      this side's stays and goes up again   — (told)
//
// After the whole project is marked deleted on the server there is no base
// (projectsync.go): any live version that comes later is merged by rows 5.
//
// An older snapshot: a version of a project in which at least one file is
// a return to an earlier version (2b) is taken for an older copy of the
// whole project (an older agent sent an old checkout, a version restored on
// the server). The files it lacks that are here as agreed (2c) were not
// deleted by anyone: they are removed here, as that version has it, but
// kept for good next to their place, and the project is said in one line
// (projectsync.go).
//
// A file deleted here, with the deletion agreed, that the server has again
// (changed on another computer, so it went up again there): it comes here
// (2a for memory, 5b for a project), and the person is told.
//
// Rollback: l ≠ b, but l is one of the file's earlier base versions, and
// the folder is not trusted by its identity — an older copy put back (a
// backup, an old checkout), not an edit: the server's stays, l goes to the
// history, the person is told once. With a trusted identity going back to
// an older version is an ordinary edit (3a) and goes up, and this side says
// so; the other computers take it by row 2b.
//
// A folder trusted by its path only (the file system cannot tell folders
// apart, or an older agent's base): edits go up (3a) unless they are a
// rollback, but a missing file is not a deletion (3b).

// fside — one side of one file.
type fside struct {
	// known — for the base: there is one for this file (it may be a
	// deletion). Always true for l and s.
	known bool
	// exists — the file is there.
	exists bool
	// deleted — the server holds a deletion mark for it (memory, settings);
	// a file missing from a project's pack is merely not there.
	deleted bool
	hash    string
}

func present(hash string) fside { return fside{known: true, exists: true, hash: hash} }
func absent() fside             { return fside{known: true} }
func noBase() fside             { return fside{} }

func (x fside) same(y fside) bool { return x.exists == y.exists && (!x.exists || x.hash == y.hash) }

// trustLevel — how far the folder here is the one the base was agreed in.
type trustLevel int

const (
	// trustNone — another folder: the binding moved, the repository was
	// cloned again, the folder was made again, or nothing is known.
	trustNone trustLevel = iota
	// trustPath — the same path; its identity cannot be read (or an older
	// agent's base knows none).
	trustPath
	// trustFull — the same path and the same folder.
	trustFull
)

// verdict — what happens to one file.
type verdict int

const (
	vKeep   verdict = iota // nothing: l = s
	vTake                  // the server's version here (written, or removed)
	vSend                  // this side's version to the server (or its deletion)
	vResend                // this side's stays and goes up over a deletion there
)

// reason — what the person is told about a file (one line per kind per
// exchange).
type reason int

const (
	reasonNone            reason = iota
	reasonUntrusted              // 3b: not this folder — the server's stays
	reasonBoth                   // 4: changed here and on the server
	reasonNoBase                 // 5c: nothing to compare with
	reasonRollback               // an older copy here
	reasonResent                 // 4a, 5d: deleted on the server, kept and sent again
	reasonRestored               // missing here, not a deletion: back from the server
	reasonJoin                   // the settings of a computer joining sync, replaced by the server's
	reasonDeletedThere           // 2c: deleted on another computer, removed here
	reasonReverted               // 2b: put back to an earlier version on another computer
	reasonReturnSent             // 3a: equal to an earlier version here, sent as a return
	reasonOlderBack              // 2b inside an older snapshot of a project (projectsync.go)
	reasonOlderGone              // a file an older snapshot of a project lacks: removed, kept for good
	reasonCameBack               // deleted here and agreed; changed elsewhere since: back here
	reasonSettingsEarlier        // 2b for a settings file: the history, said, no permanent copy
	reasonIndex                  // the memory index: text around the note lines changed on both sides, or nothing to compare
)

// keepKind — what becomes of the content here that a vTake replaces.
type keepKind int

const (
	keepNone          keepKind = iota // nothing there, or nothing of it is lost
	keepHistory                       // a version the server had: sync-history, 30 days
	keepUnsent                        // a permanent copy: next to it, told, never removed by sync
	keepNothing                       // no copy: the new content carries everything (a merged index)
	keepAlways                        // a permanent copy even when the new content holds all of the old (permissions)
	keepHistoryAlways                 // the history even when the new content holds all of the old (a settings file in an earlier version)
)

type decision struct {
	v      verdict
	keep   keepKind
	reason reason
}

// dctx — what a decision needs besides the three sides.
type dctx struct {
	trust trustLevel
	// absenceDeletes — a file missing here is a deletion: the folder is
	// trusted by its identity and holds the rest of its content (a folder
	// emptied, or a memory folder without its index, is not a deletion).
	absenceDeletes bool
	// rollback — l is one of the file's earlier base versions.
	rollback bool
	// sReturn — s is one of the file's earlier base versions: another
	// computer went back to it (or an older agent wrote an old one).
	sReturn bool
}

// decide — the table at the top of the file.
func decide(l, b, s fside, c dctx) decision {
	if l.same(s) {
		return decision{v: vKeep}
	}
	if l.exists && c.rollback && c.trust < trustFull && !l.same(b) {
		return decision{v: vTake, keep: keepHistory, reason: reasonRollback}
	}
	if !b.known {
		switch {
		case !l.exists:
			return decision{v: vTake} // 5b
		case !s.exists && s.deleted:
			return decision{v: vResend, reason: reasonResent} // 5d
		case !s.exists:
			return decision{v: vSend} // 5a
		}
		return decision{v: vTake, keep: keepUnsent, reason: reasonNoBase} // 5c
	}
	if l.same(b) {
		switch {
		case !s.exists:
			return decision{v: vTake, keep: keepHistory, reason: reasonDeletedThere} // 2c
		case c.sReturn:
			return decision{v: vTake, keep: keepUnsent, reason: reasonReverted} // 2b
		}
		return decision{v: vTake, keep: keepHistory} // 2a
	}
	if s.same(b) {
		switch {
		case !l.exists && c.absenceDeletes:
			return decision{v: vSend} // 3a, a deletion
		case !l.exists:
			return decision{v: vTake, reason: reasonRestored} // 3b, back from the server
		case c.trust >= trustPath && c.rollback:
			return decision{v: vSend, reason: reasonReturnSent} // 3a, a return
		case c.trust >= trustPath:
			return decision{v: vSend} // 3a
		}
		return decision{v: vTake, keep: keepUnsent, reason: reasonUntrusted} // 3b
	}
	switch {
	case !s.exists:
		return decision{v: vResend, reason: reasonResent} // 4a
	case !l.exists:
		return decision{v: vTake} // 4b
	}
	return decision{v: vTake, keep: keepUnsent, reason: reasonBoth} // 4
}
