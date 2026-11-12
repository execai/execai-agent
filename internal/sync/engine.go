package sync

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/crypto"
)

// Engine runs one exchange. It keeps no state of its own — everything
// agreed with the server lives in State (sync.json), which the caller loads
// before and saves after, and in Bases (sync-bases.json), which the engine
// reads and writes itself. At its end Run saves sync.json, then
// sync-bases.json: an exchange stopped between the two leaves the bases one
// step behind sync.json, which the next one reads as a base agreed without
// its manifest and rebuilds it from the files here or the server's (both
// are that base then, bases.go) — the other order would leave sync.json
// behind, and an edit made before the next exchange would look like a
// change on both sides.
//
// Each file is decided on its own by comparing three hashes — here, the
// base, the server (threeway.go): the settings blob (configsync.go), each
// personal memory file (memsync.go), each file of each bound project
// (projectsync.go). Nothing is decided by the time of a file.
type Engine struct {
	Client *Client
	ID     *crypto.Identity
	Paths  Paths
	Now    func() time.Time
	// hist — the history folders read in this exchange (history.go).
	hist *historyIndex
	// newKey — Options.NewKey of the exchange running.
	newKey bool
	// agreedModel — the model of the agreed settings (Bases.SettingsModel)
	// while they are the base of this exchange; nil — not known.
	agreedModel *string
}

// Options shape one exchange.
type Options struct {
	// PushOnly — send local changes without asking for the list of news (a
	// change was noticed here, or the agent exits). A file the server may
	// have changed is fetched before anything of it is decided.
	PushOnly bool
	// Join — the first exchange after this machine took the key from the
	// server: where the settings differ, the server's win — that is what the
	// person came for — and the sources only this machine had are kept.
	Join bool
	// Overwrite — this machine's key replaced the server's (overwrite.go).
	Overwrite bool
	// OverwriteSeen — blob key → the version shown to the person when they
	// chose to keep this computer's key; only these, at these versions, are
	// deleted.
	OverwriteSeen map[string]int64
	// NewKey — the first exchange after the key changed here (this
	// computer took the server's key in place of its own, kept its own over
	// the server's, or had what an earlier key left deleted): what goes up
	// again over a deletion is said as sent with the current key, which is
	// what happens.
	NewKey bool
	// Projects — workspace id → directory of the projects bound here; nil
	// takes the ones remembered in State.
	Projects map[string]string
	// Only — exchange just this blob ("kind/name"): a project right after
	// it was bound or published. Everything else is left as it is.
	Only string
	// LastProjects — the folders sync.json named for the projects at the
	// start, before this exchange refreshed them from the bindings: where an
	// older agent's base was agreed (bases.go). nil takes State.Projects.
	LastProjects map[string]string
}

func (o Options) wants(key string) bool { return o.Only == "" || o.Only == key }

// Report — what one exchange did.
type Report struct {
	Pulled, Pushed int
	// Deleted — deletions sent for files deleted here.
	Deleted int
	// Removed — local files removed because the server's version has
	// them no more.
	Removed int
	// Conflicts — files changed here and on the server since the last
	// exchange.
	Conflicts int
	// What is in sync after the exchange. Config — the settings blob
	// (settings, sources with their keys, permissions); Projects — the
	// projects bound here whose memory is in sync.
	Memory   int
	Config   bool
	Projects int
	// Unreadable — server files this key cannot open.
	Unreadable int
	// Skipped — what this exchange left out in both directions, and why
	// (local.go): too large, unreadable, a link, a bound folder not here.
	Skipped []Skip
	// SkippedNew — of Skipped, what the latest exchange did not leave out
	// (service.go: said once).
	SkippedNew []Skip
	QuotaFull  bool
	// PulledKinds — kinds whose local files changed.
	PulledKinds []string
	// Unsent — copies of content the server never saw that a version from
	// the server replaced here (keeper.go): kept for good; the person is
	// told, by reason.
	Unsent []unsentCopy
	// History — versions the server had seen, replaced here and kept in
	// <config>/sync-history (history.go): silently.
	History int
	// Restored — files missing here, not taken for deletions, written back
	// from the server; Rollback — older copies put back here, replaced by
	// the server's version; Resent — files deleted on the server, kept here
	// and sent again. By area (a project folder, the memory, the settings).
	Restored, Rollback, Resent []areaCount
	// RollbackGone — older copies put back here of files the server no
	// longer has: removed here, into the history.
	RollbackGone []areaCount
	// Changed — files that changed while the exchange decided about them:
	// left for the next exchange.
	Changed []areaCount
	// Clashes — names that are a file on one side and a folder on the
	// other: left as they are, here and on the server. The service turns
	// them into Skipped (SkipNameClash), so they are said once.
	Clashes []string
	// ProjectsKept — project folders whose memory the server marks
	// deleted and whose files are kept here.
	ProjectsKept []string
	// Collisions — names the server holds twice, differing only in case,
	// that this file system cannot keep apart: left as they are.
	Collisions []string
	// OverwriteLeft — names of what keeping this computer's key did not
	// delete because it changed or appeared after the question.
	OverwriteLeft []string
	// BasesNewer — sync-bases.json was written by a newer agent: read, not
	// changed.
	BasesNewer bool
	// DeletedThere — files removed here because another computer deleted
	// them (row 2c), their last versions in the history; ReturnSent — files
	// here equal to one of their earlier versions, sent as a return to it
	// (row 3a). By area. (A return made on another computer, row 2b, keeps
	// the newer version as a permanent copy: Unsent, reasonReverted.)
	DeletedThere, ReturnSent []areaCount
	// HistoryEvicted — history versions younger than historyDays removed to
	// stay within historyBytes.
	HistoryEvicted int
	// Hidden — what is on the server that this key cannot open (blob
	// keys: projects, memory files, the settings): encrypted with an
	// earlier key of this sync, left when a computer kept its own key or
	// written by one that had not switched yet.
	Hidden []string
	// SentNewKey — files that went up again over a deletion on the server
	// in the first exchange after the key changed here (Options.NewKey).
	SentNewKey []areaCount
	// SentNew — files of a project the server did not have that went up,
	// said when the same exchange kept a copy of that project's files (a
	// folder bound again, a computer joining): else nothing would say it.
	SentNew []areaCount
	// SettingsEarlier — settings files that came from the server in an
	// earlier version: the one here is in the history.
	SettingsEarlier []areaCount
	// CameBack — files deleted here (the deletion agreed) that came back
	// from the server: changed on another computer since.
	CameBack []areaCount
	// ProjectsBack — projects marked deleted on the server that came back
	// from here (5d): all of their files went up again.
	ProjectsBack []areaCount
	// historyOf — file → its history copy made (or found) in this
	// exchange, for the lines that say where the last version is.
	historyOf map[string]string
}

// hide counts a server file this key cannot open.
func (r *Report) hide(key string) {
	r.Unreadable++
	if !slices.Contains(r.Hidden, key) {
		r.Hidden = append(r.Hidden, key)
	}
}

// areaCount — files of one kind in one area: a project's .execai, the
// memory folder or the config folder.
type areaCount struct {
	kind, root string
	files      []string
}

func addArea(list []areaCount, kind, root, file string) []areaCount {
	for i := range list {
		if list[i].kind == kind && list[i].root == root {
			list[i].files = append(list[i].files, file)
			return list
		}
	}
	return append(list, areaCount{kind: kind, root: root, files: []string{file}})
}

func (r *Report) pulled(kind string) {
	r.Pulled++
	r.PulledKinds = appendKind(r.PulledKinds, kind)
}

func appendKind(kinds []string, kind string) []string {
	for _, k := range kinds {
		if k == kind {
			return kinds
		}
	}
	return append(kinds, kind)
}

func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

// Run performs one exchange and updates st.
func (e *Engine) Run(ctx context.Context, st *State, opt Options) (rep Report, err error) {
	if st.Blobs == nil {
		st.Blobs = map[string]BlobState{}
	}
	projects := opt.Projects
	if projects == nil {
		projects = st.Projects
	}
	prior := opt.LastProjects
	if prior == nil {
		prior = st.Projects
	}
	e.hist = &historyIndex{}
	e.newKey = opt.NewKey
	bs := loadBases(e.Paths)
	e.agreedModel = nil
	if b, had := st.Blobs[blobKey(KindConfig, NameConfig)]; had && !b.Deleted && bs.Settings != nil &&
		bs.SettingsVersion == b.Version && bs.SettingsHash == b.Hash {
		e.agreedModel = bs.SettingsModel
	}
	rep.BasesNewer = bs.readOnly
	// The bases an older agent agreed get their records — the folder
	// sync.json named at the start — before anything can fail or change
	// sync.json (bases.go).
	for ws, dir := range projects {
		key := blobKey(KindProjectMemory, ws)
		if b, had := st.Blobs[key]; had && opt.wants(key) {
			bs.projectTrust(ws, dir, b, prior)
		}
	}
	if err := saveBases(e.Paths, bs); err != nil {
		return rep, err
	}
	defer func() {
		// The history is cleaned once per exchange, not after each version.
		rep.HistoryEvicted += pruneHistory(e.Paths, e.now())
		crashPoint("save")
		if serr := st.Save(e.Paths); serr != nil && err == nil {
			err = serr
		}
		crashPoint("state")
		if serr := saveBases(e.Paths, bs); serr != nil && err == nil {
			err = serr
		}
	}()
	projects, taken := oneFolderOneProject(projects, bs, st.ProjectNames)
	sc, err := e.collect(projects, opt)
	if err != nil {
		return rep, err
	}
	rep.Skipped = append(rep.Skipped, sc.notes...)
	rep.Skipped = append(rep.Skipped, taken...)
	if opt.Only != "" {
		for k := range sc.items {
			if k != opt.Only {
				delete(sc.items, k)
			}
		}
	}

	var remote map[string]Meta // nil: a send-only round
	if !opt.PushOnly {
		list, err := e.Client.List(ctx, 0)
		if err != nil {
			return rep, err
		}
		if opt.Overwrite {
			if err := e.overwrite(ctx, st, list, sc, opt.OverwriteSeen, &rep); err != nil {
				return rep, err
			}
			if list, err = e.Client.List(ctx, 0); err != nil {
				return rep, err
			}
		}
		remote = map[string]Meta{}
		for _, m := range list {
			if knownKind(m.Kind) {
				remote[blobKey(m.Kind, m.Name)] = m
			}
		}
		// Blobs the server no longer lists were wiped there: no base for
		// them any more (what is here goes up as new).
		for k := range st.Blobs {
			if _, listed := remote[k]; !listed && opt.wants(k) && !sc.skipped[k] {
				delete(st.Blobs, k)
			}
		}
	}

	if err := e.syncConfig(ctx, st, bs, remote, sc, opt, &rep); err != nil {
		return rep, err
	}
	if err := e.syncMemory(ctx, st, bs, remote, sc, opt, &rep); err != nil {
		return rep, err
	}
	for _, ws := range sortedKeys(projects) {
		key := blobKey(KindProjectMemory, ws)
		if !validName(ws) || !opt.wants(key) {
			continue
		}
		var meta *Meta
		if m, ok := remote[key]; ok {
			meta = &m
		}
		if err := e.syncProject(ctx, st, bs, ws, projects[ws], meta, remote != nil, sc, prior, opt, &rep); err != nil {
			return rep, err
		}
	}
	if remote != nil {
		st.LastPull = e.now()
	}

	for k, it := range sc.items {
		switch it.Kind {
		case KindMemory:
			rep.Memory++
		case KindConfig:
			rep.Config = true
		}
		_ = k
	}
	// Projects in sync: bound here, not left out, with a live base (as
	// /sync counts them, State.Contents).
	for ws := range projects {
		key := blobKey(KindProjectMemory, ws)
		if b, ok := st.Blobs[key]; ok && !b.Deleted && opt.wants(key) && !sc.skipped[key] {
			rep.Projects++
		}
	}
	return rep, nil
}

// crashPoint — tests stop the world here to see that an exchange cut at
// that moment comes together on the next one (crashk_test.go): "file"
// before each file a keeper changes, "sent" after the server took a file,
// "save" before the state of the exchange is saved, "state" between
// sync.json and sync-bases.json, "prepared" when the keeper has written the
// copy and the new content, "rename" in its last moment before a
// replacement (under the writers' lock). Nothing in production.
var crashPoint = func(string) {}

// errQuotaStop — the server's space is used up: nothing more goes up in
// this exchange.
var errQuotaStop = errors.New("quota")

// upload seals and sends one file over the expected version; ok=false when
// it was not sent and that is not an error (too large: left out; no space:
// said once).
func (e *Engine) upload(ctx context.Context, kind, name, p, file string, data []byte, expected int64, rep *Report) (ver int64, ok bool, err error) {
	ct, err := seal(e.ID, kind, p, data)
	if err != nil {
		return 0, false, err
	}
	if len(ct) > blobLimit(kind) {
		rep.Skipped = append(rep.Skipped, Skip{Path: file, Reason: SkipTooBig, Limit: blobLimit(kind) >> 10})
		return 0, false, nil
	}
	v, err := e.Client.Put(ctx, kind, name, ct, expected)
	switch {
	case err == nil:
		crashPoint("sent")
		rep.Pushed++
		return v, true, nil
	case errors.Is(err, ErrTooLarge):
		rep.Skipped = append(rep.Skipped, Skip{Path: file, Reason: SkipTooBig, Limit: blobLimit(kind) >> 10})
		return 0, false, nil
	case errors.Is(err, ErrQuota):
		rep.QuotaFull = true
		return 0, false, nil
	}
	return 0, false, err
}

// fetch — the server's current content of a blob: p — its path inside the
// envelope; gone — a deletion mark (ver its version); none — no such blob.
type fetched struct {
	p      string
	data   []byte
	ver    int64
	gone   bool
	none   bool
	upd    time.Time
	hidden bool // encrypted with another key: cannot be read here
}

func (e *Engine) fetch(ctx context.Context, kind, name string, listedVer int64) (fetched, error) {
	body, ver, upd, err := e.Client.Get(ctx, kind, name)
	switch {
	case errors.Is(err, ErrNotFound):
		return fetched{none: true}, nil
	case errors.Is(err, ErrGone):
		if ver == 0 {
			ver = listedVer
		}
		return fetched{gone: true, ver: ver}, nil
	case err != nil:
		return fetched{}, err
	}
	p, data, err := e.open(body, kind, name)
	if err != nil {
		return fetched{ver: ver, hidden: true}, nil
	}
	return fetched{p: p, data: data, ver: ver, upd: upd}, nil
}

// open decrypts a server file and checks it belongs where it was found.
func (e *Engine) open(body []byte, kind, name string) (string, []byte, error) {
	k, p, data, err := unseal(e.ID, body)
	if err != nil {
		return "", nil, err
	}
	if k != kind {
		return "", nil, errEnvelope
	}
	switch kind {
	case KindMemory:
		if !safeRel(p) || memoryName(e.ID, p) != name {
			return "", nil, errEnvelope
		}
	default:
		if p != name {
			return "", nil, errEnvelope
		}
	}
	return p, data, nil
}

// isChanged — a keeper refused because the file changed meanwhile.
func isChanged(err error) bool { return errors.Is(err, errChanged) }

// memoryOnlyRound — an exchange of one project leaves the memory alone.
func memoryOnlyRound(opt Options) bool {
	return opt.Only == "" || strings.HasPrefix(opt.Only, KindMemory+"/")
}
