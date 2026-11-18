package sync

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

// Decision of the owner (01.11.2026): the whole config travels as one
// opaque encrypted blob — config.json, subscriptions.json with the provider
// keys, permissions.json. It is encrypted here like everything else; the
// server keeps one ciphertext and does not know or care what is inside.
// The config is global: the same on every computer of the person.
//
// The machine-specific values: api_base in config.json (which server this
// machine talks to) is kept locally when a blob is applied, and the
// MachineOnlyKeys never leave the machine at all. credentials.json, host_id,
// plan.json, sessions, models_cache.json, projects.json and serve.pid never
// travel.
var ConfigFiles = []string{"config.json", "subscriptions.json", "permissions.json"}

// MachineOnlyKeys — config.json fields about this computer alone: they are
// cut out before the config is packed, and an arriving config does not
// change them here. notifications depends on each computer's screen and
// terminal (E105); model_auto marks a model the agent picked itself here;
// ui_browser names the program execai ui opens its window with (E106).
var MachineOnlyKeys = []string{"notifications", "model_auto", "ui_browser"}

// configScan packs the config files present here; ok=false when none is.
// sk — the settings are left out of this exchange in both directions: a
// settings file is a link (sync does not write through links), not a
// regular file, cannot be read, or the pack is over the limit.
func (e *Engine) configScan() (it item, ok bool, sk *Skip) {
	files := map[string][]byte{}
	rawHash := map[string]string{}
	for _, name := range ConfigFiles {
		file := filepath.Join(e.Paths.ConfigDir, name)
		fi, err := os.Lstat(file)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			continue
		case err != nil:
			sk := unreadable(file, file, err)
			return item{}, false, &sk
		case fi.Mode()&fs.ModeSymlink != 0:
			return item{}, false, &Skip{Path: file, Reason: SkipLink}
		case !fi.Mode().IsRegular():
			return item{}, false, &Skip{Path: file, Reason: SkipSpecial}
		}
		raw, err := os.ReadFile(file)
		if err != nil {
			sk := unreadable(file, file, err)
			return item{}, false, &sk
		}
		rawHash[name] = hashOf(raw)
		if name == "config.json" {
			raw = stripMachineOnly(agreedModel(raw, e.agreedModel))
		}
		files[name] = raw
	}
	if len(files) == 0 {
		return item{}, false, nil
	}
	data, err := packFiles(files)
	if err != nil {
		sk := unreadable(e.Paths.ConfigFile(), e.Paths.ConfigFile(), err)
		return item{}, false, &sk
	}
	if sealedSize(KindConfig, NameConfig, int64(len(data))) > MaxBlobBytes {
		return item{}, false, &Skip{Path: e.Paths.ConfigFile(), Reason: SkipTooBig, Limit: MaxBlobBytes >> 10}
	}
	return item{Kind: KindConfig, Name: NameConfig, Path: NameConfig, File: e.Paths.ConfigFile(),
		Data: data, Hash: hashOf(data), Raw: rawHash}, true, nil
}

// configItem — configScan for settings just written: left out is an error
// here.
func (e *Engine) configItem() (item, bool, error) {
	it, ok, sk := e.configScan()
	if sk != nil {
		return item{}, false, fmt.Errorf("sync: %s: %s %s", sk.Path, sk.Reason, sk.Detail)
	}
	return it, ok, nil
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

// agreedModel — config.json as it travels when the agent picked its model
// here by itself (selected_model_id equal to model_auto: the chosen one is
// not in this computer's list, say on a start without the network): with
// the model of the agreed settings, so a pick of this computer alone is
// neither sent nor taken for the person's choice. A model the person chose
// (model_auto cleared, config.ChooseModel) travels as it is. Unchanged when
// the agreed model is not known.
func agreedModel(raw []byte, agreed *string) []byte {
	if agreed == nil {
		return raw
	}
	all := map[string]json.RawMessage{}
	if json.Unmarshal(raw, &all) != nil {
		return raw
	}
	var sel, auto string
	_ = json.Unmarshal(all["selected_model_id"], &sel)
	_ = json.Unmarshal(all["model_auto"], &auto)
	if auto == "" || sel != auto || sel == *agreed {
		return raw
	}
	if *agreed == "" {
		delete(all, "selected_model_id")
	} else {
		v, _ := json.Marshal(*agreed)
		all["selected_model_id"] = v
	}
	out, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return raw
	}
	return out
}

// machineOnly — key names one of the MachineOnlyKeys, in any spelling. The
// agent reads config.json with encoding/json, which matches a field's name
// without regard to case: "UI_BROWSER" is ui_browser to it, and so it must
// be here — matched byte for byte, another spelling travelled.
func machineOnly(key string) bool {
	for _, k := range MachineOnlyKeys {
		if strings.EqualFold(k, key) {
			return true
		}
	}
	return false
}

// stripMachineOnly — config.json without the MachineOnlyKeys. A file that
// has none of them is returned byte for byte, so its hash does not change.
func stripMachineOnly(raw []byte) []byte {
	all := map[string]json.RawMessage{}
	if json.Unmarshal(raw, &all) != nil {
		return raw
	}
	cut := false
	for k := range all {
		if machineOnly(k) {
			delete(all, k)
			cut = true
		}
	}
	if !cut {
		return raw
	}
	out, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return raw
	}
	return out
}

// keepMachineOnly — the incoming config.json with this machine's own
// MachineOnlyKeys: its values where it has them, as it spells them, and
// none where it has none — whatever arrived under any spelling of those
// names. An incoming file that needs no change is returned byte for byte.
func keepMachineOnly(local, remote []byte) []byte {
	all := map[string]json.RawMessage{}
	if json.Unmarshal(remote, &all) != nil {
		return remote
	}
	var mine map[string]json.RawMessage
	_ = json.Unmarshal(local, &mine) // missing or broken — nothing of its own
	own := map[string]json.RawMessage{}
	for k, v := range mine {
		if machineOnly(k) {
			own[k] = v
		}
	}
	arrived := map[string]json.RawMessage{}
	for k, v := range all {
		if machineOnly(k) {
			arrived[k] = v
			delete(all, k)
		}
	}
	same := len(arrived) == len(own)
	for k, v := range own {
		if av, ok := arrived[k]; !ok || !bytes.Equal(av, v) {
			same = false
		}
		all[k] = v
	}
	if same {
		return remote
	}
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
