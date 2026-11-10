package sync

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	gosync "sync"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/crypto"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/plan"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
	"github.com/velesbsdllc/agent-vbai/internal/version"
)

// Service is sync on this machine: the commands, the setup questions and
// the background exchange. One per process; safe for concurrent use —
// exchanges run one at a time.
type Service struct {
	Paths  Paths
	Client *Client
	Gate   *Gate
	// Creds returns the current credentials (nil when signed out).
	Creds func() *config.Credentials
	// Subs returns the source store — for the manifest and key transfer.
	Subs func() *subscriptions.Store
	// Version — the agent version for the manifest.
	Version func() string
	Now     func() time.Time
	// Cmd — how the command is typed here: "/sync" in the chat, "execai
	// sync" in a shell. Used in the hints.
	Cmd string

	// Timing of the background runner; zero takes the package defaults.
	PullEvery, PushDelay, WatchEvery time.Duration

	mu gosync.Mutex
	// warned — background messages already shown in this process.
	warned map[string]bool
	// lastFP — the fingerprint of the local files after the latest exchange.
	lastFP string
}

// New wires a Service to this machine: config directory, credentials.json,
// subscriptions.json, plan.json.
func New(apiBase string) (*Service, error) {
	paths, err := DefaultPaths()
	if err != nil {
		return nil, err
	}
	creds := func() *config.Credentials {
		cr, _ := config.LoadCredentials()
		return cr
	}
	token := func() string {
		if cr := creds(); cr != nil {
			return cr.Token
		}
		return ""
	}
	return &Service{
		Paths:  paths,
		Client: NewClient(apiBase, token),
		Gate:   &Gate{Checker: plan.NewChecker(apiBase, filepath.Join(paths.ConfigDir, "plan.json"), token)},
		Creds:  creds,
		Subs: func() *subscriptions.Store {
			s, _ := subscriptions.Load()
			return s
		},
		Version: version.Get,
		Cmd:     "/sync",
	}, nil
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) agentID() string {
	if s.Creds == nil {
		return ""
	}
	if cr := s.Creds(); cr != nil {
		return cr.AgentID
	}
	return ""
}

func (s *Service) signedIn() bool {
	if s.Creds == nil {
		return false
	}
	cr := s.Creds()
	return cr != nil && cr.Token != ""
}

// Status — the /sync report. Local only: it never waits on the network.
func (s *Service) Status() string {
	st, err := LoadState(s.Paths)
	if err != nil {
		return i18n.Tf("sync.error", err)
	}
	var b strings.Builder
	if !st.Enabled {
		b.WriteString(i18n.Tf("sync.status.off", s.Cmd))
		b.WriteString("\n" + i18n.Tf("sync.status.keysFallback", s.Cmd))
		return b.String()
	}
	last := st.LastPull
	if st.LastPush.After(last) {
		last = st.LastPush
	}
	if last.IsZero() {
		b.WriteString(i18n.T("sync.status.onNever"))
	} else {
		b.WriteString(i18n.Tf("sync.status.on", last.Local().Format("2006-01-02 15:04")))
	}
	b.WriteString("\n" + s.whatLine(s.countState(st)))
	switch st.Paused {
	case PausedPlan:
		b.WriteString("\n" + i18n.Tf("sync.status.pausedPlan", SubscribeURL))
	case PausedLogin:
		b.WriteString("\n" + i18n.T("sync.status.pausedLogin"))
	case PausedKey:
		b.WriteString("\n" + i18n.Tf("sync.status.pausedKey", s.Cmd))
	}
	if st.Unreadable > 0 && st.Paused != PausedKey {
		b.WriteString("\n" + i18n.Tf("sync.status.unreadable", st.Unreadable, s.Cmd))
	}
	if len(st.TooBig) > 0 {
		b.WriteString("\n" + i18n.Tf("sync.status.tooBig", strings.Join(st.TooBig, ", ")))
	}
	if st.QuotaFull {
		b.WriteString("\n" + i18n.T("sync.status.quota"))
	}
	if len(st.Projects) > 0 {
		b.WriteString("\n" + i18n.Tf("sync.status.projects", len(st.Projects), strings.Join(projectNames(st), ", ")))
	}
	if st.LastError != "" {
		b.WriteString("\n" + i18n.Tf("sync.status.lastError", st.LastError))
	}
	b.WriteString("\n" + i18n.Tf("sync.status.commands", s.Cmd))
	return b.String()
}

// countState — what is in sync, by the saved state.
func (s *Service) countState(st *State) Report { return st.Contents() }

// whatLine — «Syncing: memory (N files), settings and provider keys — all
// encrypted on this computer».
func (s *Service) whatLine(r Report) string {
	parts := []string{i18n.Tf("sync.what.memory", r.Memory)}
	if r.Config {
		parts = append(parts, i18n.T("sync.what.config"))
	}
	if r.Projects > 0 {
		parts = append(parts, i18n.Tf("sync.what.projects", r.Projects))
	}
	return i18n.Tf("sync.status.what", strings.Join(parts, ", "))
}

// summary — the result of an exchange the person asked for.
func (s *Service) summary(head string, r Report) string {
	lines := []string{head, s.whatLine(r)}
	if r.Unreadable > 0 {
		lines = append(lines, i18n.Tf("sync.status.unreadable", r.Unreadable, s.Cmd))
	}
	if len(r.TooBig) > 0 {
		lines = append(lines, i18n.Tf("sync.status.tooBig", strings.Join(r.TooBig, ", ")))
	}
	if r.QuotaFull {
		lines = append(lines, i18n.T("sync.status.quota"))
	}
	return strings.Join(lines, "\n")
}

// Off switches sync off on this machine. The server keeps the data; the
// machine leaves the list in the web console.
func (s *Service) Off(ctx context.Context) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := LoadState(s.Paths)
	if err != nil {
		return i18n.Tf("sync.error", err)
	}
	if !st.Enabled {
		return i18n.Tf("sync.off.already", s.Cmd)
	}
	st.Enabled = false
	st.Paused = ""
	st.MachineManifestHash = ""
	if err := st.Save(s.Paths); err != nil {
		return i18n.Tf("sync.error", err)
	}
	if id := s.agentID(); id != "" {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		_ = s.Client.DeleteMachine(ctx, id) // best effort: the web shows it as stale otherwise
	}
	return i18n.Tf("sync.off.done", s.Cmd)
}

// SyncNow — /sync now: one full exchange right away.
func (s *Service) SyncNow(ctx context.Context) string {
	if v := s.Gate.Check(ctx, true); v != Allowed {
		return v.Message()
	}
	st, err := LoadState(s.Paths)
	if err != nil {
		return i18n.Tf("sync.error", err)
	}
	if !st.Enabled {
		return i18n.Tf("sync.now.off", s.Cmd)
	}
	rep, err := s.exchange(ctx, Options{})
	if err != nil {
		return s.failText(err)
	}
	return s.summary(i18n.Tf("sync.now.done", rep.Pulled+rep.Removed, rep.Pushed+rep.Deleted), rep)
}

// failText explains a failed exchange.
func (s *Service) failText(err error) string {
	switch {
	case errors.Is(err, errWiped):
		return i18n.Tf("sync.bg.wiped", s.Cmd)
	case errors.Is(err, errKeyChanged):
		return i18n.Tf("sync.status.pausedKey", s.Cmd)
	case errors.Is(err, ErrUnauthorized):
		return i18n.T("sync.gate.login")
	}
	return i18n.Tf("sync.failed", err)
}

var (
	errWiped      = errors.New("sync: the server has no sync data any more")
	errKeyChanged = errors.New("sync: the key on the server is not this machine's key")
	errNoKey      = errors.New("sync: this machine has no key")
)

// exchange — one exchange with the checks every exchange needs. The
// caller has passed the gate. A full exchange first checks the key on the
// server: gone (wiped from another machine) switches sync off here; another
// key (replaced elsewhere) pauses it until the person enters the password.
func (s *Service) exchange(ctx context.Context, opt Options) (Report, error) {
	pushOnly := opt.PushOnly
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := LoadState(s.Paths)
	if err != nil {
		return Report{}, err
	}
	if !st.Enabled {
		return Report{}, nil
	}
	id, err := loadKey(s.Paths.ConfigDir)
	if err != nil {
		return Report{}, err
	}
	if id == nil {
		st.Paused = PausedKey
		_ = st.Save(s.Paths)
		return Report{}, errNoKey
	}
	if pushOnly && st.Paused == PausedKey {
		return Report{}, errKeyChanged
	}
	if !pushOnly {
		ri, err := s.Client.GetIdentity(ctx)
		switch {
		case errors.Is(err, ErrNotFound):
			st.Enabled = false
			st.Paused = ""
			st.reset()
			_ = st.Save(s.Paths)
			return Report{}, errWiped
		case err != nil:
			st.LastError = err.Error()
			_ = st.Save(s.Paths)
			return Report{}, err
		case WrappedFingerprint(ri.Wrapped) != KeyFingerprint(id):
			st.Paused = PausedKey
			_ = st.Save(s.Paths)
			return Report{}, errKeyChanged
		}
		if st.Paused == PausedKey {
			st.Paused = ""
		}
	}
	return s.runLocked(ctx, st, id, opt)
}

// runLocked runs the engine and saves the state. Caller holds s.mu.
func (s *Service) runLocked(ctx context.Context, st *State, id *crypto.Identity, opt Options) (Report, error) {
	if !opt.PushOnly {
		if projects, err := s.projects(ctx); err == nil {
			st.Projects = projects
			s.projectNames(ctx, st)
		}
	}
	for ws, dir := range opt.Projects {
		// A binding made a moment ago, which the bindings list may not
		// show yet.
		if st.Projects == nil {
			st.Projects = map[string]string{}
		}
		st.Projects[ws] = dir
	}
	opt.Projects = st.Projects
	eng := &Engine{Client: s.Client, ID: id, Paths: s.Paths, Now: s.Now}
	rep, err := eng.Run(ctx, st, opt)
	if opt.Only == "" {
		if !opt.PushOnly {
			st.Unreadable = rep.Unreadable
		}
		st.TooBig = rep.TooBig
		st.QuotaFull = rep.QuotaFull
	}
	st.LastError = ""
	if err != nil {
		st.LastError = err.Error()
	} else {
		// An exchange that went through ends a plan or sign-in pause, also
		// when the person asked for it (/sync now after renewing the plan).
		if st.Paused == PausedPlan || st.Paused == PausedLogin {
			st.Paused = ""
		}
		if !opt.PushOnly && opt.Only == "" {
			s.sendManifest(ctx, st)
		}
	}
	if serr := st.Save(s.Paths); serr != nil && err == nil {
		err = serr
	}
	return rep, err
}

// projects — the projects bound to this machine whose directory exists
// here: workspace id → directory.
func (s *Service) projects(ctx context.Context) (map[string]string, error) {
	binds, err := s.Client.Bindings(ctx)
	if err != nil {
		return nil, err
	}
	self := s.agentID()
	out := map[string]string{}
	for _, b := range binds {
		if b.WorkspaceID == "" || b.LocalPath == "" || !validName(b.WorkspaceID) {
			continue
		}
		if self != "" && b.AgentID != "" && b.AgentID != self {
			continue // another machine's binding
		}
		if fi, err := os.Stat(b.LocalPath); err != nil || !fi.IsDir() {
			continue
		}
		out[b.WorkspaceID] = b.LocalPath
	}
	return out, nil
}

// sendManifest tells the server which sources this machine has — names
// only — when that changed or was last said long ago.
func (s *Service) sendManifest(ctx context.Context, st *State) {
	agentID := s.agentID()
	if agentID == "" || s.Subs == nil {
		return
	}
	ver := "dev"
	if s.Version != nil {
		ver = s.Version()
	}
	m := BuildManifest(s.Subs(), ver)
	h := m.hash()
	if h == st.MachineManifestHash && s.now().Sub(st.ManifestAt) < ManifestEvery {
		return
	}
	if err := s.Client.PutMachine(ctx, agentID, m); err != nil {
		return
	}
	st.MachineManifestHash = h
	st.ManifestAt = s.now()
}

// projectNames — the names of the projects bound here: the web name, or
// the directory's when the web has not told it.
func projectNames(st *State) []string {
	var out []string
	for _, ws := range sortedKeys(st.Projects) {
		name := st.ProjectNames[ws]
		if name == "" {
			name = filepath.Base(st.Projects[ws])
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// projectNames fills st.ProjectNames from the web projects when a bound one
// has no name yet; a failure leaves the directory names.
func (s *Service) projectNames(ctx context.Context, st *State) {
	missing := false
	for ws := range st.Projects {
		if st.ProjectNames[ws] == "" {
			missing = true
		}
	}
	if !missing {
		return
	}
	wss, err := s.Client.Workspaces(ctx)
	if err != nil {
		return
	}
	if st.ProjectNames == nil {
		st.ProjectNames = map[string]string{}
	}
	for _, w := range wss {
		if _, ok := st.Projects[w.ID]; ok && w.Name != "" {
			st.ProjectNames[w.ID] = w.Name
		}
	}
}

// SyncProject exchanges one project right after it was bound or published
// here: a project from the web lands in the directory, a local one goes up.
// Text for the person; "" when there is nothing to say.
func (s *Service) SyncProject(ctx context.Context, ws, dir, name string) string {
	st, err := LoadState(s.Paths)
	if err != nil {
		return i18n.Tf("sync.error", err)
	}
	if !st.Enabled {
		return i18n.Tf("sync.project.off", s.Cmd)
	}
	if v := s.Gate.Check(ctx, true); v != Allowed {
		return v.Message()
	}
	if name != "" {
		s.mu.Lock()
		if st2, err := LoadState(s.Paths); err == nil {
			if st2.ProjectNames == nil {
				st2.ProjectNames = map[string]string{}
			}
			st2.ProjectNames[ws] = name
			_ = st2.Save(s.Paths)
		}
		s.mu.Unlock()
	}
	rep, err := s.exchange(ctx, Options{Only: blobKey(KindProjectMemory, ws), Projects: map[string]string{ws: dir}})
	if err != nil {
		return s.failText(err)
	}
	var lines []string
	switch {
	case rep.Pulled+rep.Removed > 0:
		lines = append(lines, i18n.T("sync.project.pulled"))
	case rep.Pushed > 0:
		lines = append(lines, i18n.T("sync.project.pushed"))
	case rep.Projects > 0:
		lines = append(lines, i18n.T("sync.project.same"))
	default:
		lines = append(lines, i18n.T("sync.project.empty"))
	}
	if len(rep.Backups) > 0 {
		lines = append(lines, i18n.Tf("sync.project.backup", strings.Join(rep.Backups, ", ")))
	}
	if len(rep.TooBig) > 0 {
		lines = append(lines, i18n.Tf("sync.status.tooBig", strings.Join(rep.TooBig, ", ")))
	}
	return strings.Join(lines, "\n")
}
