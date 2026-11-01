package sync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	gosync "sync"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// Hooks — how the background runner talks to the process hosting it.
type Hooks struct {
	// Notify shows one message to the person: a hint in the chat, a line of
	// execai serve output. nil — stay silent (the editor's stdout is a
	// protocol).
	Notify func(text string)
	// Pulled — local files changed because of another machine (kinds as in
	// Kind*), so the host can re-read what it keeps in memory.
	Pulled func(kinds []string)
}

// Start runs sync in the background of a long-lived process — the chat,
// execai serve, the editor panel — and returns the function to call on
// exit: it stops the loop and sends what is left. While sync is off on
// this machine nothing happens, not even a plan check; switching it on
// later (/sync on) is picked up by the running loop.
func Start(ctx context.Context, apiBase string, h Hooks) (stop func()) {
	s, err := New(apiBase)
	if err != nil {
		return func() {}
	}
	return s.Start(ctx, h)
}

// Start — see the package-level Start.
func (s *Service) Start(ctx context.Context, h Hooks) (stop func()) {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.loop(ctx, h)
	}()
	var once gosync.Once
	return func() {
		once.Do(func() {
			cancel()
			<-done
			if s.fingerprint() == s.getLastFP() {
				return // nothing changed since the last exchange
			}
			ctx, cancel := context.WithTimeout(context.Background(), ExitPushTimeout)
			defer cancel()
			s.background(ctx, h, true)
		})
	}
}

func orDefault(d, def time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return def
}

// loop: a full exchange at start and every PullEvery; a push PushDelay
// after the local files stop changing.
func (s *Service) loop(ctx context.Context, h Hooks) {
	pullEvery := orDefault(s.PullEvery, PullEvery)
	pushDelay := orDefault(s.PushDelay, PushDelay)
	watch := orDefault(s.WatchEvery, WatchEvery)

	s.background(ctx, h, false)
	fp := s.fingerprint()
	var pushAt time.Time
	nextPull := time.Now().Add(pullEvery)
	t := time.NewTicker(watch)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			if cur := s.fingerprint(); cur != fp {
				fp = cur
				pushAt = now.Add(pushDelay)
			}
			switch {
			case !nextPull.After(now):
				nextPull = now.Add(pullEvery)
				pushAt = time.Time{}
				s.background(ctx, h, false)
				fp = s.fingerprint()
			case !pushAt.IsZero() && !pushAt.After(now):
				pushAt = time.Time{}
				s.background(ctx, h, true)
				fp = s.fingerprint()
			}
		}
	}
}

// background — one exchange of the runner: gate, exchange, and one message
// per kind of trouble.
func (s *Service) background(ctx context.Context, h Hooks, pushOnly bool) {
	defer func() { s.setLastFP(s.fingerprint()) }()
	st, err := LoadState(s.Paths)
	if err != nil || !st.Enabled {
		return
	}
	v := s.Gate.Check(ctx, false)
	if v != Allowed {
		if s.setPaused(v.pauseReason()) {
			s.notify(h, "", i18n.T("sync.bg.paused")+"\n"+v.Message())
		}
		return
	}
	rep, err := s.exchange(ctx, Options{PushOnly: pushOnly})
	// A plan or sign-in pause ends with the first exchange that works, not
	// with the first gate that lets through: a token billing still accepts
	// but sync rejects would otherwise flip «paused / works again» forever.
	if err == nil && (st.Paused == PausedPlan || st.Paused == PausedLogin) {
		s.notify(h, "", i18n.T("sync.bg.resumed")) // runLocked has cleared it
	}
	switch {
	case errors.Is(err, errWiped):
		s.notify(h, "wiped", i18n.Tf("sync.bg.wiped", s.Cmd))
	case errors.Is(err, errKeyChanged), errors.Is(err, errNoKey):
		s.notify(h, "key", i18n.Tf("sync.bg.keyChanged", s.Cmd))
	case errors.Is(err, ErrUnauthorized):
		if s.setPaused(PausedLogin) {
			s.notify(h, "", i18n.T("sync.bg.paused")+"\n"+NotSignedIn.Message())
		}
	}
	if rep.Unreadable > 0 {
		s.notify(h, "unreadable", i18n.Tf("sync.status.unreadable", rep.Unreadable, s.Cmd))
	}
	if rep.QuotaFull {
		s.notify(h, "quota", i18n.T("sync.status.quota"))
	}
	if len(rep.Backups) > 0 {
		s.notify(h, "", i18n.Tf("sync.project.backup", strings.Join(rep.Backups, ", ")))
	}
	if len(rep.PulledKinds) > 0 && h.Pulled != nil {
		h.Pulled(rep.PulledKinds)
	}
}

// setPaused records a pause reason; true when it changed.
func (s *Service) setPaused(reason string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := LoadState(s.Paths)
	if err != nil || !st.Enabled || st.Paused == reason {
		return false
	}
	st.Paused = reason
	return st.Save(s.Paths) == nil
}

// notify shows a message; with a key — once per process.
func (s *Service) notify(h Hooks, key, text string) {
	if h.Notify == nil {
		return
	}
	if key != "" {
		s.mu.Lock()
		if s.warned == nil {
			s.warned = map[string]bool{}
		}
		seen := s.warned[key]
		s.warned[key] = true
		s.mu.Unlock()
		if seen {
			return
		}
	}
	h.Notify(text)
}

func (s *Service) getLastFP() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastFP
}

func (s *Service) setLastFP(fp string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastFP = fp
}

// fingerprint — a cheap summary (names, sizes, times) of the files sync
// looks after; "" while sync is off. Only stat, no reading.
func (s *Service) fingerprint() string {
	st, err := LoadState(s.Paths)
	if err != nil || !st.Enabled {
		return ""
	}
	h := sha256.New()
	stamp := func(name string, fi fs.FileInfo) {
		fmt.Fprintf(h, "%s|%d|%d\n", name, fi.Size(), fi.ModTime().UnixNano())
	}
	memDir := s.Paths.MemoryDir()
	_ = filepath.WalkDir(memDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == memDir {
			return nil
		}
		if skipMemoryFile(d.Name()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if fi, err := d.Info(); err == nil && fi.Mode().IsRegular() {
			stamp(p, fi)
		}
		return nil
	})
	files := []string{s.Paths.ConfigFile(), s.Paths.SubscriptionsFile(), s.Paths.PermissionsFile()}
	for _, ws := range sortedKeys(st.Projects) {
		pd := ProjectDir(st.Projects[ws])
		fmt.Fprintf(h, "project %s\n", ws)
		_ = walkProject(pd, func(rel, _ string, fi fs.FileInfo) error {
			stamp(ws+"/"+rel, fi)
			return nil
		})
	}
	for _, f := range files {
		if fi, err := os.Stat(f); err == nil {
			stamp(f, fi)
		} else {
			fmt.Fprintf(h, "%s|-\n", f)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}
