package sync

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

// Decision of the owner (01.11.2026): the whole config travels as one
// opaque encrypted blob — config.json, subscriptions.json with the provider
// keys, permissions.json. It is encrypted here like everything else; the
// server keeps one ciphertext and does not know or care what is inside.
// The config is global: the same on every computer of the person.
//
// The one machine-specific value is api_base in config.json (which server
// this machine talks to): a local api_base is kept when a blob is applied.
// credentials.json, host_id, plan.json, sessions, models_cache.json,
// projects.json and serve.pid never travel.
var ConfigFiles = []string{"config.json", "subscriptions.json", "permissions.json"}

// configItem packs the config files present here; ok=false when none is.
func (e *Engine) configItem() (item, bool, error) {
	files := map[string][]byte{}
	var mt time.Time
	for _, name := range ConfigFiles {
		raw, m, ok, err := readFile(filepath.Join(e.Paths.ConfigDir, name))
		if err != nil {
			return item{}, false, err
		}
		if !ok {
			continue
		}
		files[name] = raw
		if m.After(mt) {
			mt = m
		}
	}
	if len(files) == 0 {
		return item{}, false, nil
	}
	data, err := packFiles(files)
	if err != nil {
		return item{}, false, err
	}
	return item{Kind: KindConfig, Name: NameConfig, Path: NameConfig, File: e.Paths.ConfigFile(),
		Data: data, Hash: hashOf(data), ModTime: mt}, true, nil
}

// applyConfig writes the files of a config blob. A file the blob lacks is
// left alone, one that is not valid JSON is skipped (a broken config.json
// would stop the agent from starting). join — the first exchange of this
// machine: its own sources are kept next to the ones that arrive, and
// pushBack says the result must go back to the server for the others.
func (e *Engine) applyConfig(data []byte, join bool, mtime time.Time) (pushBack bool, err error) {
	files, err := unpackFiles(data)
	if err != nil {
		return false, err
	}
	for _, name := range ConfigFiles {
		content, ok := files[name]
		if !ok || !json.Valid(content) {
			continue
		}
		target := filepath.Join(e.Paths.ConfigDir, name)
		local, err := os.ReadFile(target)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
		switch name {
		case "config.json":
			content = keepAPIBase(local, content)
		case "subscriptions.json":
			if join && len(local) > 0 {
				var added bool
				content, added = joinSubscriptions(local, content)
				pushBack = pushBack || added
			}
		}
		if err := writeFileAtomic(target, content); err != nil {
			return false, err
		}
		if !mtime.IsZero() {
			_ = os.Chtimes(target, mtime, mtime)
		}
	}
	return pushBack, nil
}

// keepAPIBase — the incoming config.json with this machine's api_base, when
// it has one.
func keepAPIBase(local, remote []byte) []byte {
	var l struct {
		APIBase string `json:"api_base"`
	}
	if json.Unmarshal(local, &l) != nil || l.APIBase == "" {
		return remote
	}
	all := map[string]json.RawMessage{}
	if json.Unmarshal(remote, &all) != nil {
		return remote
	}
	v, _ := json.Marshal(l.APIBase)
	all["api_base"] = v
	out, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return remote
	}
	return out
}

// joinSubscriptions — on joining, the arriving sources win, and the ones
// only this machine had stay; the active source is the arriving one when it
// exists. added — this machine brought sources the others do not have yet.
// After that the store is global and replaced as a whole.
func joinSubscriptions(local, remote []byte) (out []byte, added bool) {
	var l, r subscriptions.Store
	if json.Unmarshal(local, &l) != nil || json.Unmarshal(remote, &r) != nil {
		return remote, false
	}
	if r.Subscriptions == nil {
		r.Subscriptions = map[string]subscriptions.Subscription{}
	}
	for id, sub := range l.Subscriptions {
		if _, ok := r.Subscriptions[id]; !ok {
			r.Subscriptions[id] = sub
			added = true
		}
	}
	if !added {
		return remote, false
	}
	if _, ok := r.Subscriptions[r.Active]; !ok && r.Active != "" && r.Active != subscriptions.SourceExecAI {
		r.Active = l.Active
	}
	out, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return remote, false
	}
	return out, true
}
