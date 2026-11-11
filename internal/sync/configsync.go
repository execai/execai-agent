package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"maps"
	"time"
)

// The settings — config.json, subscriptions.json, permissions.json — travel
// as one blob, but each file is decided on its own by the table
// (threeway.go) against its base: the settings' manifest in
// sync-bases.json, file by file (bases.go). A provider key added on one
// computer and a model changed on another both stay; a stricter permission
// set here does not give way to the server's because another settings file
// changed there. The settings folder is always the same one (trusted), and
// a missing settings file is never a deletion: it comes back.
//
// Joining sync (the first exchange after this computer took the key from
// the server), where a file differs the server's wins — that is what the
// person came for — and what this computer loses is kept and told; its own
// sources stay next to the ones that arrive and go back up.
//
// Merging inside a file, key by key, is not done here: a file changed on
// both sides keeps the server's and this computer's goes aside, told.
//
// As for a project, the merged settings go up before anything is written
// here, and the base is recorded once the files are in place.
func (e *Engine) syncConfig(ctx context.Context, st *State, bs *Bases, remote map[string]Meta, sc *scan, opt Options, rep *Report) error {
	k := blobKey(KindConfig, NameConfig)
	if !opt.wants(k) || sc.skipped[k] {
		return nil
	}
	li, here := sc.items[k]
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 && remote == nil {
			return nil // send-only and refused: the next full exchange decides
		}
		if attempt > 0 {
			it, ok, err := e.configItem()
			if err != nil {
				return nil // left out now: the next exchange
			}
			li, here = it, ok
		}
		b, had := st.Blobs[k]
		var L map[string][]byte
		if here {
			var err error
			if L, err = unpackFiles(li.Data); err != nil {
				return err
			}
		}
		var B map[string]string
		if had && !b.Deleted {
			switch {
			case bs.Settings != nil && bs.SettingsVersion == b.Version && bs.SettingsHash == b.Hash:
				B = bs.Settings
			case here && li.Hash == b.Hash:
				B = settingsHashes(L)
			}
		}

		// The server's side.
		var s fetched
		switch {
		case remote == nil:
			// Send-only: the server is taken for the base until it says
			// otherwise; what needs the server's content waits.
			if !here || (had && !b.Deleted && li.Hash == b.Hash) {
				return nil
			}
			switch {
			case !had:
				s.none = true
			case b.Deleted:
				s = fetched{gone: true, ver: b.Version}
			default:
				if B == nil || missingHere(B, L) {
					return nil // the next full exchange
				}
				s = fetched{ver: b.Version, data: nil}
			}
		case attempt == 0:
			m, listed := remote[k]
			switch {
			case !listed:
				s.none = true
			case m.Deleted:
				s = fetched{gone: true, ver: m.Version}
			case had && !b.Deleted && m.Version == b.Version && B != nil && !missingHere(B, L):
				s = fetched{ver: b.Version} // the server holds the base: no need to read it
			default:
				var err error
				if s, err = e.fetch(ctx, KindConfig, NameConfig, m.Version); err != nil {
					return err
				}
				if s.none {
					return nil // listed but not given: next time
				}
			}
		default:
			var err error
			if s, err = e.fetch(ctx, KindConfig, NameConfig, 0); err != nil {
				return err
			}
		}
		if s.gone {
			// Deleted on the server (what an earlier key left, deleted on
			// /sync on): no base any more, and every settings file here
			// goes up again — a settings file is never deleted.
			B = nil
		}
		if s.hidden {
			rep.hide(k)
			return nil
		}
		var S map[string][]byte
		sameAsBase := !s.none && !s.gone && s.data == nil
		if !s.none && !s.gone && !sameAsBase {
			var err error
			if S, err = unpackFiles(s.data); err != nil {
				return err
			}
			if B == nil && had && !b.Deleted && hashOf(s.data) == b.Hash {
				B = settingsHashes(S)
			}
		}
		if B != nil {
			B = upgradeBase(maps.Clone(B), L, S)
		}
		if !here && (s.none || s.gone || sameAsBase) {
			if s.gone {
				st.Blobs[k] = BlobState{Version: s.ver, Deleted: true}
			}
			return nil
		}

		// File by file.
		type job struct {
			name    string
			content []byte
			d       decision
		}
		var jobs []job
		M := map[string][]byte{}
		for _, name := range ConfigFiles {
			lv, lok := L[name]
			var sv []byte
			sok := false
			if sameAsBase {
				// The server holds the base: its content for a file is the
				// base's, which here equals l wherever l = b.
				if h, ok := B[name]; ok {
					if lok && settingHash(name, lv) == h {
						sv, sok = lv, true
					} else {
						sok = true // b's content: not needed unless taken (see missingHere)
					}
				}
			} else {
				sv, sok = S[name]
			}
			l, s := absent(), absent()
			if lok {
				l = present(settingHash(name, lv))
			}
			if sok {
				if sv != nil {
					s = present(settingHash(name, sv))
				} else {
					s = present(B[name])
				}
			}
			if !sok && !lok {
				continue
			}
			bb := noBase()
			if B != nil {
				if h, ok := B[name]; ok {
					bb = present(h)
				} else if had && !b.Deleted {
					bb = absent()
				}
			}
			d := decide(l, bb, s, dctx{trust: trustFull, absenceDeletes: false,
				sReturn: s.exists && bb.exists && s.hash != bb.hash && inList(bs.SettingsPrev[name], s.hash)})
			if d.reason == reasonReverted {
				// A settings file in an earlier version (a provider key
				// removed, a permission an older agent put back): the
				// version here was the agreed one, so it goes to the history
				// for 30 days — never a permanent copy, which would keep a
				// removed key for good. The person is told only where it
				// matters: permissions and prohibitions, provider keys; any
				// other settings file is replaced as usual (a model switched
				// there and back elsewhere is no news).
				d.keep, d.reason = keepHistory, reasonNone
				if name == "permissions.json" || name == "subscriptions.json" {
					d.keep, d.reason = keepHistoryAlways, reasonSettingsEarlier
				}
			}
			if opt.Join && lok && sok && !l.same(s) {
				d = decision{v: vTake, keep: keepUnsent, reason: reasonJoin}
			}
			if name == "permissions.json" && d.v == vTake && d.keep == keepUnsent && !opt.Join {
				// A permission taken away here is a change too, though the
				// server's list holds all of this one: the person must know
				// what of this computer's permissions was not applied.
				d.keep = keepAlways
			}
			switch d.v {
			case vKeep:
				// The same setting on both sides: the server's bytes stay,
				// so this computer's own address does not go up.
				if sok && sv != nil {
					M[name] = sv
				} else if lok {
					M[name] = lv
				}
			case vSend, vResend:
				if lok {
					M[name] = lv
				}
			case vTake:
				if !sok {
					continue // a settings file is never deleted here
				}
				content := sv
				if name == "subscriptions.json" && opt.Join && lok {
					// This computer's own sources stay and go up with the rest.
					content, _ = joinSubscriptions(lv, sv)
				}
				M[name] = content
				jobs = append(jobs, job{name: name, content: content, d: d})
			}
		}
		if len(M) == 0 {
			return nil
		}
		mData, err := packFiles(M)
		if err != nil {
			return err
		}
		mHash := hashOf(mData)
		var sHashes map[string]string
		switch {
		case sameAsBase:
			sHashes, mHash = B, b.Hash
		case s.data != nil:
			sHashes = settingsHashes(S)
		}

		// Up first — when the settings differ from the server's, not merely
		// this computer's own address.
		newVer := s.ver
		if sHashes == nil || !sameSettings(settingsHashes(M), sHashes) {
			mHash = hashOf(mData)
			v, ok, err := e.upload(ctx, KindConfig, NameConfig, NameConfig, li.File, mData, s.ver, rep)
			var ce *ConflictError
			if errors.As(err, &ce) {
				continue // written meanwhile: decide again on what is there now
			}
			if err != nil || !ok {
				return err
			}
			newVer = v
		} else if s.data != nil {
			mHash = hashOf(s.data)
		}

		kp := e.keeper(rep)
		wrote := false
		for _, j := range jobs {
			if !json.Valid(j.content) {
				continue // a broken file would stop the agent from starting
			}
			pl := place{kind: KindConfig, root: e.Paths.ConfigDir, rel: j.name}
			local, _, err := kp.read(pl)
			if err != nil {
				return err
			}
			content := j.content
			if j.name == "config.json" {
				content = keepMachineOnly(local, keepAPIBase(local, content))
			}
			changed, err := kp.put(pl, content, time.Time{}, li.Raw[j.name], j.d.keep, j.d.reason, opt.Join)
			if isChanged(err) {
				rep.Changed = addArea(rep.Changed, KindConfig, e.Paths.ConfigDir, pl.file())
				return nil
			}
			if err != nil && isLocalErr(err) {
				rep.Skipped = append(rep.Skipped, Skip{Path: pl.file(), Reason: SkipUnreadable, Cause: causeWrite})
				return nil
			}
			if err != nil {
				return err
			}
			if changed {
				wrote = true
				if j.d.reason == reasonSettingsEarlier {
					rep.SettingsEarlier = addArea(rep.SettingsEarlier, KindConfig, e.Paths.ConfigDir, pl.file())
				}
			}
			if j.d.reason == reasonBoth {
				rep.Conflicts++
			}
		}
		if wrote {
			rep.pulled(KindConfig)
		}

		// What is on disk now goes for the base — sent first when writing
		// it (this computer's own keys kept) made it differ from what went
		// up, so the base is what the server holds.
		fresh, ok, err := e.configItem()
		if err != nil || !ok {
			return err
		}
		freshFiles, err := unpackFiles(fresh.Data)
		if err != nil {
			return err
		}
		if !sameSettings(settingsHashes(freshFiles), settingsHashes(M)) {
			v, ok, err := e.upload(ctx, KindConfig, NameConfig, NameConfig, fresh.File, fresh.Data, newVer, rep)
			var ce *ConflictError
			if errors.As(err, &ce) {
				return nil // the next exchange compares again
			}
			if err != nil || !ok {
				return err
			}
			newVer, mHash, M = v, fresh.Hash, freshFiles
		}
		e.noteSettings(st, bs, BlobState{Path: NameConfig, Version: newVer, Hash: mHash}, settingsHashes(M), M["config.json"])
		return nil
	}
	return nil
}

// settingHash — the hash a settings file is compared by: config.json
// without what is each computer's own (the server address, notifications),
// so two computers with their own addresses hold the same settings;
// subscriptions.json without what the agent writes there by itself (the
// providers' model lists, dropSelfUpdated), so a model list one computer
// fetched is no change of the settings.
func settingHash(name string, data []byte) string {
	if name == "subscriptions.json" {
		return subscriptionsHash(data)
	}
	if name != "config.json" {
		return hashOf(data)
	}
	all := map[string]json.RawMessage{}
	if json.Unmarshal(data, &all) != nil {
		return hashOf(data)
	}
	delete(all, "api_base")
	for k := range all {
		if machineOnly(k) {
			delete(all, k)
		}
	}
	out, err := json.Marshal(all) // keys sorted: the same settings, the same bytes
	if err != nil {
		return hashOf(data)
	}
	return hashOf(out)
}

// subscriptionsHash — settingHash of subscriptions.json: the file as JSON
// with keys in order, without what the agent writes there by itself. (Older
// agents hashed the file as it was: upgradeBase.)
func subscriptionsHash(data []byte) string {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) != nil {
		return hashOf(data)
	}
	dropSelfUpdated(v)
	out, err := json.Marshal(v)
	if err != nil {
		return hashOf(data)
	}
	return hashOf(out)
}

// upgradeBase — the settings base in the hashes of this agent: a base an
// older agent recorded hashed subscriptions.json as it was; where that is
// the file here (or the server's), its hash now is the base.
func upgradeBase(B map[string]string, L, S map[string][]byte) map[string]string {
	for name, h := range B {
		for _, side := range []map[string][]byte{L, S} {
			if v, ok := side[name]; ok && v != nil && hashOf(v) == h {
				B[name] = settingHash(name, v)
				break
			}
		}
	}
	return B
}

func settingsHashes(files map[string][]byte) map[string]string {
	out := make(map[string]string, len(files))
	for name, data := range files {
		out[name] = settingHash(name, data)
	}
	return out
}

func sameSettings(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// missingHere — a settings file of the base is missing here: bringing it
// back needs the server's content.
func missingHere(B map[string]string, L map[string][]byte) bool {
	for name := range B {
		if _, ok := L[name]; !ok {
			return true
		}
	}
	return false
}

// noteSettings records the settings' base, file by file; what a file had
// before joins its earlier hashes.
func (e *Engine) noteSettings(st *State, bs *Bases, nb BlobState, files map[string]string, config []byte) {
	if bs.SettingsPrev == nil {
		bs.SettingsPrev = map[string][]string{}
	}
	for name, h := range bs.Settings {
		if files[name] != h {
			bs.SettingsPrev[name] = pushPrev(bs.SettingsPrev[name], h)
		}
	}
	st.Blobs[blobKey(KindConfig, NameConfig)] = nb
	bs.Settings, bs.SettingsVersion, bs.SettingsHash = files, nb.Version, nb.Hash
	var c struct {
		Model string `json:"selected_model_id"`
	}
	_ = json.Unmarshal(config, &c)
	bs.SettingsModel = &c.Model
	e.agreedModel = bs.SettingsModel
}
