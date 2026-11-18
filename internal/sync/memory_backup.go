package sync

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/connect"
)

// Permanent copies of personal memory and settings files (keeper.go,
// keepUnsent): kept on this computer in
//
//	<config>/sync-unsent/<YYYY-MM-DD>/<path in memory, or the settings file name, with .local-<HHMMSS> before the extension>
//
// The folder is outside <config>/memory and is not one of the settings
// files, so the copies are neither memory the agent reads nor files that
// sync sends. Every copy is written 0600 in a 0700 folder: a settings copy
// may hold provider keys, exactly as subscriptions.json itself does, and is
// never shown by content — messages name the file only. Sync never removes
// them; the same content of the same file is not copied twice.
//
// Not sync-conflicts/, where they went before: agent 7.17 and the earlier
// candidates of this change remove the day folders there older than 30
// days, and may still run in the same config folder (ExecAI Studio). This
// agent never touches sync-conflicts/ and counts what is there in /sync.

// ConflictCopyDays — how long a history version is kept (history.go); the
// permanent copies are kept for good.
const ConflictCopyDays = historyDays

// conflictDayLayout — the name of a day folder.
const conflictDayLayout = "2006-01-02"

// ConflictsDir — <config>/sync-conflicts: copies of earlier agents.
func (p Paths) ConflictsDir() string { return filepath.Join(p.ConfigDir, "sync-conflicts") }

// UnsentDir — <config>/sync-unsent: the permanent copies of memory and
// settings files.
func (p Paths) UnsentDir() string { return filepath.Join(p.ConfigDir, "sync-unsent") }

// configLoses reports whether putting winner in place of loser (one
// settings file) drops anything of loser that is not this computer's own by
// design. Not counted: api_base and the MachineOnlyKeys of config.json
// (kept here when a config is applied), a selected_model_id the agent picked
// itself (equal to autoModel, this computer's model_auto), the model lists
// and connection time of a source (fetched again), and — when joining — the
// sources the join keeps and the active source (the arriving one is the
// point of joining). Lists count as sets. A file that is not JSON is always
// counted.
func configLoses(name string, loser, winner []byte, join bool, autoModel string) bool {
	if bytes.Equal(loser, winner) {
		return false
	}
	if name == "subscriptions.json" && join {
		winner, _ = joinSubscriptions(loser, winner)
	}
	var l, w any
	if json.Unmarshal(loser, &l) != nil || json.Unmarshal(winner, &w) != nil {
		return true
	}
	switch name {
	case "config.json":
		dropKeys(l, append([]string{"api_base"}, MachineOnlyKeys...)...)
		dropKeys(w, append([]string{"api_base"}, MachineOnlyKeys...)...)
		if m, ok := l.(map[string]any); ok && autoModel != "" && m["selected_model_id"] == autoModel {
			delete(m, "selected_model_id") // picked by the agent, not a choice
		}
	case "subscriptions.json":
		for _, v := range []any{l, w} {
			if join {
				dropKeys(v, "active")
			}
			dropSelfUpdated(v)
		}
	}
	return !jsonContains(w, l)
}

// SelfUpdatedKeys — fields of a source in subscriptions.json the agent
// writes by itself, not the person: the provider's model list
// (connect.RefreshModels, on start and on a source switch), when it was
// fetched, and when the source was connected. They are no setting: a change
// of them alone is not sent, does not count as an edit and makes no
// conflict, and they are no loss when a file is replaced.
var SelfUpdatedKeys = []string{"models", "available_models", "models_fetched_at", "connected_at"}

// dropSelfUpdated removes from a decoded subscriptions.json what the agent
// writes by itself: SelfUpdatedKeys of every source, and a base_url that is
// the provider's own default (the model list refresh fills it in when it is
// empty).
func dropSelfUpdated(v any) {
	m, ok := v.(map[string]any)
	if !ok {
		return
	}
	subs, ok := m["subscriptions"].(map[string]any)
	if !ok {
		return
	}
	for id, sub := range subs {
		dropKeys(sub, SelfUpdatedKeys...)
		sm, ok := sub.(map[string]any)
		if !ok {
			continue
		}
		bu, _ := sm["base_url"].(string)
		prov, _ := sm["provider"].(string)
		if bu != "" && (bu == connect.DefaultBaseURL(id) || bu == connect.DefaultBaseURL(prov)) {
			delete(sm, "base_url")
		}
	}
}

// dropKeys removes keys from v when it is a JSON object.
func dropKeys(v any, keys ...string) {
	if m, ok := v.(map[string]any); ok {
		for _, k := range keys {
			delete(m, k)
		}
	}
}

// jsonContains — everything in part is also in whole: objects key by key,
// lists as sets, other values equal.
func jsonContains(whole, part any) bool {
	switch p := part.(type) {
	case map[string]any:
		w, ok := whole.(map[string]any)
		if !ok {
			return false
		}
		for k, pv := range p {
			wv, ok := w[k]
			if !ok || !jsonContains(wv, pv) {
				return false
			}
		}
		return true
	case []any:
		w, ok := whole.([]any)
		if !ok {
			return false
		}
		for _, pv := range p {
			found := false
			for _, wv := range w {
				if reflect.DeepEqual(wv, pv) {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
		return true
	}
	return reflect.DeepEqual(whole, part)
}

// keepUnsentCopy writes one copy of a memory or settings file (rel — its
// path in the memory folder, or the settings file name) under today's
// folder of root (<config>/sync-unsent), flushed to disk, and returns its
// path; created=false when a copy of the same file with exactly this
// content is there already, any day — that one is returned.
func keepUnsentCopy(root string, now time.Time, rel string, data []byte) (p string, created bool, err error) {
	if !safeRel(rel) {
		return "", false, nil
	}
	base := path.Base(rel)
	ext := path.Ext(base)
	if ext == base {
		ext = "" // ".md" alone is a name, not an extension
	}
	stem := strings.TrimSuffix(base, ext)
	if days, err := os.ReadDir(root); err == nil {
		for _, d := range days {
			if !d.IsDir() {
				continue
			}
			if same := identicalCopy(filepath.Join(root, d.Name(), filepath.FromSlash(path.Dir(rel))), stem, ext, data); same != "" {
				privateDirs(root, filepath.Dir(same))
				privateFile(same)
				return same, false, nil
			}
		}
	}
	dir := filepath.Join(root, now.Format(conflictDayLayout), filepath.FromSlash(path.Dir(rel)))
	name := stem + ".local-" + now.Format("150405")
	target := filepath.Join(dir, name+ext)
	for i := 2; fileExists(target); i++ {
		target = filepath.Join(dir, fmt.Sprintf("%s-%d%s", name, i, ext))
	}
	if err := writeFileAtomic(target, data); err != nil {
		return "", false, err
	}
	privateDirs(root, dir)
	return target, true, nil
}

// identicalCopy — a copy in dir of the file stem…ext, either side
// (<stem>.local-… or <stem>.server-…), holding exactly data.
func identicalCopy(dir, stem, ext string, data []byte) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, de := range entries {
		n := de.Name()
		if !de.Type().IsRegular() || !strings.HasSuffix(n, ext) ||
			!(strings.HasPrefix(n, stem+".local-") || strings.HasPrefix(n, stem+".server-")) {
			continue
		}
		p := filepath.Join(dir, n)
		if b, err := os.ReadFile(p); err == nil && bytes.Equal(b, data) {
			return p
		}
	}
	return ""
}

func fileExists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// conflictCopies lists every copy kept in root (<config>/sync-unsent, or the
// sync-conflicts of earlier agents), newest day first — for /sync. None of
// them is ever removed by sync.
func conflictCopies(root string) []string {
	var out []string
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			out = append(out, p)
		}
		return nil
	})
	sort.Sort(sort.Reverse(sort.StringSlice(out)))
	return out
}
