package sync

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/crypto"
)

// Engine runs one exchange: pull, then push. It keeps no state of its own —
// everything agreed with the server lives in State, which the caller loads
// before and saves after.
type Engine struct {
	Client *Client
	ID     *crypto.Identity
	Paths  Paths
	Now    func() time.Time
}

// Options shape one exchange.
type Options struct {
	// PushOnly — send local changes without asking for news (a change was
	// noticed here, or the agent exits).
	PushOnly bool
	// Join — the first exchange after this machine took the key from the
	// server: where the config differs, the server's wins — that is what the
	// person came for — and the sources only this machine had are kept.
	// Memory follows the usual rules.
	Join bool
	// Overwrite — this machine's key replaced the server's: every server
	// file is replaced by the local one or deleted.
	Overwrite bool
	// Projects — workspace id → directory of the projects bound here; nil
	// takes the ones remembered in State.
	Projects map[string]string
	// Only — exchange just this blob ("kind/name"): a project right after
	// it was bound or published. Everything else is left as it is.
	Only string
}

func (o Options) wants(key string) bool { return o.Only == "" || o.Only == key }

// Report — what one exchange did.
type Report struct {
	Pulled, Pushed int
	// Deleted — tombstones sent for files deleted here.
	Deleted int
	// Removed — local files deleted because they were deleted elsewhere.
	Removed int
	// Conflicts — files changed on both sides since the last exchange.
	Conflicts int
	// What is in sync after the exchange. Config — the config blob
	// (settings, sources with their keys, permissions).
	Memory   int
	Config   bool
	Projects int
	// Unreadable — server files this key cannot open.
	Unreadable int
	TooBig     []string
	QuotaFull  bool
	// PulledKinds — kinds whose local files changed.
	PulledKinds []string
	// Backups — copies of the losing side of a project conflict, kept next
	// to the files (<name>.local-<time>, <name>.server-<time>).
	Backups []string
}

func (r *Report) pulled(kind string) {
	r.Pulled++
	for _, k := range r.PulledKinds {
		if k == kind {
			return
		}
	}
	r.PulledKinds = append(r.PulledKinds, kind)
}

func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

// Run performs one exchange and updates st.
func (e *Engine) Run(ctx context.Context, st *State, opt Options) (Report, error) {
	var rep Report
	if st.Blobs == nil {
		st.Blobs = map[string]BlobState{}
	}
	projects := opt.Projects
	if projects == nil {
		projects = st.Projects
	}
	local, memDirOK, err := e.collect(projects, &rep.TooBig)
	if err != nil {
		return rep, err
	}
	if opt.Only != "" {
		for k := range local {
			if k != opt.Only {
				delete(local, k)
			}
		}
	}
	if !opt.PushOnly {
		remote, err := e.Client.List(ctx, 0)
		if err != nil {
			return rep, err
		}
		if opt.Overwrite {
			err = e.overwrite(ctx, st, remote, local, &rep)
		} else {
			err = e.pull(ctx, st, remote, local, projects, opt, &rep)
		}
		if err != nil {
			return rep, err
		}
		st.LastPull = e.now()
	}
	if err := e.push(ctx, st, local, projects, memDirOK, opt, &rep); err != nil {
		return rep, err
	}
	for _, it := range local {
		switch it.Kind {
		case KindMemory:
			rep.Memory++
		case KindConfig:
			rep.Config = true
		case KindProjectMemory:
			rep.Projects++
		}
	}
	return rep, nil
}

// pull takes the server's news.
func (e *Engine) pull(ctx context.Context, st *State, remote []Meta, local map[string]item,
	projects map[string]string, opt Options, rep *Report) error {
	seen := map[string]bool{}
	for _, m := range remote {
		if !knownKind(m.Kind) {
			continue
		}
		k := blobKey(m.Kind, m.Name)
		if !opt.wants(k) {
			continue
		}
		seen[k] = true
		prev, had := st.Blobs[k]
		if had && m.Version <= prev.Version {
			continue // nothing new
		}
		if m.Kind == KindProjectMemory {
			if _, bound := projects[m.Name]; !bound {
				continue // not bound here; taken once it is
			}
		}
		li, exists := local[k]
		changedHere := exists && (!had || prev.Deleted || li.Hash != prev.Hash)

		if m.Deleted {
			if changedHere && li.ModTime.After(m.UpdatedAt.Time) {
				// Edited here after it was deleted elsewhere: the edit wins
				// and push brings the file back.
				rep.Conflicts++
				st.Blobs[k] = BlobState{Path: li.Path, Version: m.Version, Deleted: true}
				continue
			}
			if exists && (m.Kind == KindMemory || m.Kind == KindProjectMemory) {
				var err error
				if m.Kind == KindProjectMemory {
					err = removeProject(li.File)
				} else {
					err = os.Remove(li.File)
				}
				if err != nil && !errors.Is(err, os.ErrNotExist) {
					return err
				}
				delete(local, k)
				rep.Removed++
				rep.PulledKinds = appendKind(rep.PulledKinds, m.Kind)
			}
			st.Blobs[k] = BlobState{Path: prev.Path, Version: m.Version, Deleted: true}
			continue
		}

		body, ver, upd, err := e.Client.Get(ctx, m.Kind, m.Name)
		if errors.Is(err, ErrNotFound) || errors.Is(err, ErrGone) {
			continue
		}
		if err != nil {
			return err
		}
		if upd.IsZero() {
			upd = m.UpdatedAt.Time
		}
		p, data, err := e.open(body, m.Kind, m.Name)
		if err != nil {
			rep.Unreadable++
			continue
		}
		rhash := hashOf(data)
		if exists && li.Hash == rhash {
			st.Blobs[k] = BlobState{Path: p, Version: ver, Hash: rhash}
			continue
		}
		if changedHere {
			rep.Conflicts++
			switch {
			case isIndex(m.Kind, p):
				merged := mergeLines(data, li.Data)
				if _, err := e.apply(m.Kind, li.File, merged, time.Time{}, false); err != nil {
					return err
				}
				li.Data, li.Hash, li.ModTime = merged, hashOf(merged), e.now()
				local[k] = li
				// Based on the server's version; push sends the merge.
				st.Blobs[k] = BlobState{Path: p, Version: ver, Hash: rhash}
				rep.pulled(m.Kind)
				continue
			case opt.Join && m.Kind == KindConfig:
				// The server's config wins on joining.
			case li.ModTime.After(upd):
				// Changed here later: this side wins, push overwrites. A
				// project keeps the server's copy next to its files.
				if m.Kind == KindProjectMemory {
					if err := e.keepLoser(li.File, data, li.Data, "server", rep); err != nil {
						return err
					}
				}
				st.Blobs[k] = BlobState{Path: li.Path, Version: ver, Hash: rhash}
				continue
			}
			if m.Kind == KindProjectMemory {
				if err := e.keepLoser(li.File, li.Data, data, "local", rep); err != nil {
					return err
				}
			}
		}
		file, ok := e.fileFor(m.Kind, p, projects)
		if !ok {
			continue
		}
		pushBack, err := e.apply(m.Kind, file, data, upd, opt.Join)
		if err != nil {
			return err
		}
		fresh, err := e.reread(m.Kind, m.Name, p, file)
		if err != nil {
			return err
		}
		local[k] = fresh
		// The local hash, not the remote one: the config keeps this
		// machine's api_base, and push compares against the local form —
		// unless the result has to go back (pushBack).
		h := fresh.Hash
		if pushBack {
			h = rhash
		}
		st.Blobs[k] = BlobState{Path: p, Version: ver, Hash: h}
		rep.pulled(m.Kind)
	}
	// Blobs the server no longer lists were wiped there: forget them, so
	// push puts the local files back.
	for k := range st.Blobs {
		if !seen[k] && opt.wants(k) {
			delete(st.Blobs, k)
		}
	}
	return nil
}

// keepLoser saves the losing side of a project conflict next to the files.
func (e *Engine) keepLoser(pd string, loser, winner []byte, tag string, rep *Report) error {
	paths, err := backupLosing(pd, loser, winner, tag, e.now())
	rep.Backups = append(rep.Backups, paths...)
	return err
}

func appendKind(kinds []string, kind string) []string {
	for _, k := range kinds {
		if k == kind {
			return kinds
		}
	}
	return append(kinds, kind)
}

// overwrite — this machine's key replaced the server's. Every server file
// with a local counterpart is overwritten by push; the rest is deleted.
func (e *Engine) overwrite(ctx context.Context, st *State, remote []Meta, local map[string]item, rep *Report) error {
	st.Blobs = map[string]BlobState{}
	for _, m := range remote {
		if !knownKind(m.Kind) {
			continue
		}
		k := blobKey(m.Kind, m.Name)
		if m.Deleted {
			st.Blobs[k] = BlobState{Version: m.Version, Deleted: true}
			continue
		}
		if li, ok := local[k]; ok {
			st.Blobs[k] = BlobState{Path: li.Path, Version: m.Version}
			continue
		}
		v, err := e.Client.Delete(ctx, m.Kind, m.Name, 0)
		if err != nil && !errors.Is(err, ErrNotFound) && !errors.Is(err, ErrGone) {
			return err
		}
		if v == 0 {
			v = m.Version
		}
		st.Blobs[k] = BlobState{Version: v, Deleted: true}
		rep.Deleted++
	}
	return nil
}

// errQuotaStop ends push without failing the exchange.
var errQuotaStop = errors.New("quota")

// push sends local changes and deletions.
func (e *Engine) push(ctx context.Context, st *State, local map[string]item, projects map[string]string, memDirOK bool, opt Options, rep *Report) error {
	pushed := false
	for _, k := range sortedKeys(local) {
		li := local[k]
		prev, had := st.Blobs[k]
		if had && !prev.Deleted && prev.Hash == li.Hash {
			continue
		}
		var expected int64
		if had {
			expected = prev.Version
		}
		err := e.pushOne(ctx, st, li, expected, rep)
		if errors.Is(err, errQuotaStop) {
			rep.QuotaFull = true
			break
		}
		if err != nil {
			return err
		}
		pushed = true
	}
	for _, k := range sortedKeys(st.Blobs) {
		b := st.Blobs[k]
		if b.Deleted || !opt.wants(k) {
			continue
		}
		if _, ok := local[k]; ok {
			continue
		}
		kind, name := splitKey(k)
		switch kind {
		case KindMemory:
			if !memDirOK {
				continue // the whole directory is gone: not a deletion
			}
		case KindProjectMemory:
			dir, bound := projects[name]
			if !bound {
				continue // unbound here: the project's memory stays
			}
			if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
				continue // project not reachable (unmounted disk): not a deletion
			}
		default:
			continue // settings files are never deleted through sync
		}
		v, err := e.Client.Delete(ctx, kind, name, b.Version)
		var ce *ConflictError
		switch {
		case errors.As(err, &ce) && ce.Deleted:
			st.Blobs[k] = BlobState{Path: b.Path, Version: ce.Version, Deleted: true}
			continue
		case errors.As(err, &ce):
			// Changed on another machine after this one last saw it: the
			// edit there outweighs the deletion here. Forget the blob, the
			// next pull brings the file back.
			delete(st.Blobs, k)
			rep.Conflicts++
			continue
		case errors.Is(err, ErrNotFound), errors.Is(err, ErrGone):
			st.Blobs[k] = BlobState{Path: b.Path, Version: b.Version, Deleted: true}
			continue
		case err != nil:
			return err
		}
		st.Blobs[k] = BlobState{Path: b.Path, Version: v, Deleted: true}
		rep.Deleted++
		pushed = true
	}
	if pushed {
		st.LastPush = e.now()
	}
	return nil
}

// pushOne uploads one file; a conflict is resolved on the spot.
func (e *Engine) pushOne(ctx context.Context, st *State, li item, expected int64, rep *Report) error {
	ct, err := seal(e.ID, li.Kind, li.Path, li.Data)
	if err != nil {
		return err
	}
	if len(ct) > blobLimit(li.Kind) {
		rep.TooBig = append(rep.TooBig, li.Kind+"/"+li.Path)
		return nil
	}
	k := blobKey(li.Kind, li.Name)
	v, err := e.Client.Put(ctx, li.Kind, li.Name, ct, expected)
	var ce *ConflictError
	switch {
	case err == nil:
		st.Blobs[k] = BlobState{Path: li.Path, Version: v, Hash: li.Hash}
		rep.Pushed++
		return nil
	case errors.As(err, &ce):
		return e.resolve(ctx, st, li, ct, ce, rep)
	case errors.Is(err, ErrTooLarge):
		rep.TooBig = append(rep.TooBig, li.Kind+"/"+li.Path)
		return nil
	case errors.Is(err, ErrQuota):
		return errQuotaStop
	}
	return err
}

// resolve — PUT said 409: someone wrote the file meanwhile. Read theirs and
// decide by last write wins (the index merges instead). A second conflict
// in a row is left for the next exchange.
func (e *Engine) resolve(ctx context.Context, st *State, li item, ct []byte, ce *ConflictError, rep *Report) error {
	k := blobKey(li.Kind, li.Name)
	rep.Conflicts++
	retry := func(data []byte, ct []byte, expected int64) error {
		v, err := e.Client.Put(ctx, li.Kind, li.Name, ct, expected)
		if err != nil {
			var again *ConflictError
			if errors.As(err, &again) {
				return nil // still moving; next exchange
			}
			if errors.Is(err, ErrQuota) {
				return errQuotaStop
			}
			return err
		}
		st.Blobs[k] = BlobState{Path: li.Path, Version: v, Hash: hashOf(data)}
		rep.Pushed++
		return nil
	}

	body, ver, upd, err := e.Client.Get(ctx, li.Kind, li.Name)
	if errors.Is(err, ErrGone) || errors.Is(err, ErrNotFound) {
		// Deleted elsewhere meanwhile; the edit here brings it back.
		return retry(li.Data, ct, ce.Version)
	}
	if err != nil {
		return err
	}
	if upd.IsZero() {
		upd = ce.UpdatedAt
	}
	p, data, err := e.open(body, li.Kind, li.Name)
	if err != nil {
		// Written with another key. Never overwrite what cannot be read.
		rep.Unreadable++
		return nil
	}
	rhash := hashOf(data)
	switch {
	case rhash == li.Hash:
		st.Blobs[k] = BlobState{Path: p, Version: ver, Hash: rhash}
		return nil
	case isIndex(li.Kind, p):
		merged := mergeLines(data, li.Data)
		if _, err := e.apply(li.Kind, li.File, merged, time.Time{}, false); err != nil {
			return err
		}
		mct, err := seal(e.ID, li.Kind, li.Path, merged)
		if err != nil {
			return err
		}
		rep.pulled(li.Kind)
		return retry(merged, mct, ver)
	case li.ModTime.After(upd):
		if li.Kind == KindProjectMemory {
			if err := e.keepLoser(li.File, data, li.Data, "server", rep); err != nil {
				return err
			}
		}
		return retry(li.Data, ct, ver)
	}
	// Theirs is newer.
	if li.Kind == KindProjectMemory {
		if err := e.keepLoser(li.File, li.Data, data, "local", rep); err != nil {
			return err
		}
	}
	if _, err := e.apply(li.Kind, li.File, data, upd, false); err != nil {
		return err
	}
	fresh, err := e.reread(li.Kind, li.Name, p, li.File)
	if err != nil {
		return err
	}
	st.Blobs[k] = BlobState{Path: p, Version: ver, Hash: fresh.Hash}
	rep.pulled(li.Kind)
	return nil
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

// fileFor — where a server file lands on this machine.
func (e *Engine) fileFor(kind, p string, projects map[string]string) (string, bool) {
	switch kind {
	case KindMemory:
		return filepath.Join(e.Paths.MemoryDir(), filepath.FromSlash(p)), true
	case KindConfig:
		return e.Paths.ConfigFile(), true
	case KindProjectMemory:
		dir, ok := projects[p]
		if !ok {
			return "", false
		}
		return ProjectDir(dir), true
	}
	return "", false
}

// apply writes a server file locally. mtime — when it was written on the
// server: the local file gets it, so a later comparison of «who wrote last»
// compares the moments of writing, not of copying. join — see
// Options.Join.
// pushBack — the applied result differs from the blob in a way the others
// must get (a joining machine's own sources).
func (e *Engine) apply(kind, file string, data []byte, mtime time.Time, join bool) (pushBack bool, err error) {
	switch kind {
	case KindConfig:
		return e.applyConfig(data, join, mtime)
	case KindProjectMemory:
		return false, applyProject(file, data, mtime)
	}
	if err := writeFileAtomic(file, data); err != nil {
		return false, err
	}
	if !mtime.IsZero() {
		_ = os.Chtimes(file, mtime, mtime)
	}
	return false, nil
}

// reread builds the local item of a file just written.
func (e *Engine) reread(kind, name, p, file string) (item, error) {
	switch kind {
	case KindConfig:
		it, _, err := e.configItem()
		return it, err
	case KindProjectMemory:
		it, _, _, err := e.projectItem(name, filepath.Dir(file))
		return it, err
	}
	data, mt, _, err := readFile(file)
	if err != nil {
		return item{}, err
	}
	return item{Kind: kind, Name: name, Path: p, File: file, Data: data, Hash: hashOf(data), ModTime: mt}, nil
}
