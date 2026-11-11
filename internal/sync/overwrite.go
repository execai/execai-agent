package sync

import (
	"context"
	"errors"
	"slices"

	"github.com/velesbsdllc/agent-vbai/internal/crypto"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// Keeping this computer's key (/sync on, choice 2): everything on the
// server is encrypted with the other key and cannot be read here.
//
// When the question is shown, what would go is fixed — every blob this
// computer has no counterpart of, at the version seen then (overwriteSeen),
// and the projects among them are named. On the answer only those are
// deleted, each only at that version: what was written or appeared after
// the question stays, the person is told, and /sync on asks about it again
// (State.OverwriteLeft). This computer's own files go up over the old ones.

// overwriteSeen — what keeping this computer's key would delete, as the
// server lists it now: blob key → version; and the names of the projects
// among them, for the question.
func (s *Service) overwriteSeen(ctx context.Context, id *crypto.Identity) (map[string]int64, []string, error) {
	remote, err := s.Client.List(ctx, 0)
	if err != nil {
		return nil, nil, err
	}
	projects, err := s.projects(ctx)
	if err != nil {
		return nil, nil, err
	}
	eng := &Engine{Client: s.Client, ID: id, Paths: s.Paths, Now: s.Now}
	sc, err := eng.collect(projects, Options{})
	if err != nil {
		return nil, nil, err
	}
	seen := map[string]int64{}
	var ws []string
	for _, m := range remote {
		k := blobKey(m.Kind, m.Name)
		if !knownKind(m.Kind) || m.Deleted || sc.skipped[k] {
			continue
		}
		if _, here := sc.items[k]; here {
			continue
		}
		if m.Kind == KindMemory && (sc.memAll || len(sc.memSkip) > 0) {
			continue
		}
		seen[k] = m.Version
		if m.Kind == KindProjectMemory {
			ws = append(ws, m.Name)
		}
	}
	return seen, s.projectLabels(ctx, ws), nil
}

// projectLabels — the name of each workspace id in the quotes of the
// person's language (the id when the web does not say).
func (s *Service) projectLabels(ctx context.Context, ws []string) []string {
	if len(ws) == 0 {
		return nil
	}
	names := map[string]string{}
	if wss, err := s.Client.Workspaces(ctx); err == nil {
		for _, w := range wss {
			names[w.ID] = w.Name
		}
	}
	var out []string
	for _, id := range ws {
		if n := names[id]; n != "" {
			out = append(out, i18n.Tf("sync.projectName", n))
		} else {
			out = append(out, id)
		}
	}
	slices.Sort(out)
	return out
}

// overwrite — this machine's key replaced the server's. A project or the
// settings this computer has go up over the old ones at once (the old one cannot be read
// here, so it is not compared); what was seen at the question is deleted at
// the version seen; anything else stays and is reported.
func (e *Engine) overwrite(ctx context.Context, st *State, remote []Meta, sc *scan, seen map[string]int64, rep *Report) error {
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
		if li, ok := sc.items[k]; ok {
			// This computer has it (a project, the settings): its version
			// goes up over the old one, which cannot be read here — the
			// person chose that the server gets this computer's.
			v, sent, err := e.upload(ctx, m.Kind, m.Name, li.Path, li.File, li.Data, m.Version, rep)
			var ce *ConflictError
			if errors.As(err, &ce) {
				continue // written meanwhile: the exchange compares it
			}
			if err != nil {
				return err
			}
			if sent {
				st.Blobs[k] = BlobState{Path: li.Path, Version: v, Hash: li.Hash}
			}
			continue
		}
		if sc.skipped[k] {
			continue // left out here, not «missing» (local.go)
		}
		if ver, ok := seen[k]; !ok || ver != m.Version {
			rep.OverwriteLeft = append(rep.OverwriteLeft, k)
			continue
		}
		v, err := e.Client.Delete(ctx, m.Kind, m.Name, m.Version)
		var ce *ConflictError
		switch {
		case errors.As(err, &ce):
			rep.OverwriteLeft = append(rep.OverwriteLeft, k) // written meanwhile
			continue
		case err != nil && !errors.Is(err, ErrNotFound) && !errors.Is(err, ErrGone):
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
