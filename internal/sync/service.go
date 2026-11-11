package sync

import (
	"context"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	gosync "sync"
	"sync/atomic"
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
	// CommandWait — how long a command the person typed waits for another
	// process's exchange (lock.go); zero takes the default. busyWait — the
	// same for the background runner.
	CommandWait time.Duration
	busyWait    time.Duration
	// busy — a command of this Service gave up because another process
	// held the lock (Busy).
	busy atomic.Bool
	// refused — a command of this Service was refused: no plan, not signed
	// in (Refused).
	refused atomic.Bool

	// mu keeps this process's exchanges one at a time; lock adds the lock
	// shared with the other processes of the config directory.
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
		b.WriteString("\n" + i18n.Tf("sync.status.pausedPlan", subscribeURL()))
	case PausedLogin:
		b.WriteString("\n" + i18n.T("sync.status.pausedLogin"))
	case PausedKey:
		b.WriteString("\n" + i18n.Tf("sync.status.pausedKey", s.Cmd))
	case PausedNoKey:
		b.WriteString("\n" + i18n.Tf("sync.status.pausedNoKey", s.Cmd))
	}
	// While paused for the key, what the server holds is not this key's
	// question: no line about files under an earlier key.
	keyPause := st.Paused == PausedKey || st.Paused == PausedNoKey
	if len(st.Hidden) > 0 && !keyPause {
		b.WriteString("\n" + i18n.Tf("sync.status.hidden", strings.Join(s.leftLabels(context.Background(), st.Hidden), ", "), s.Cmd))
	} else if st.Unreadable > 0 && !keyPause {
		b.WriteString("\n" + i18n.Tf("sync.status.unreadable", st.Unreadable, s.Cmd))
	}
	for _, line := range skipLines(st.Skipped) {
		b.WriteString("\n" + line)
	}
	if st.QuotaFull {
		b.WriteString("\n" + i18n.T("sync.status.quota"))
	}
	if len(st.Projects) > 0 {
		b.WriteString("\n" + i18n.Tf("sync.status.projects", len(st.Projects), strings.Join(projectNames(st), ", ")))
	}
	if n := keptProjects(st); n > 0 {
		b.WriteString("\n" + i18n.Tf("sync.status.keptDeleted", n))
	}
	if n := projectCopies(st.Projects); n > 0 {
		b.WriteString("\n" + i18n.Tf("sync.status.projectCopies", n))
	}
	if copies := conflictCopies(s.Paths.UnsentDir()); len(copies) > 0 {
		b.WriteString("\n" + i18n.Tf("sync.status.memoryCopies", len(copies), homePath(s.Paths.UnsentDir())))
	}
	if copies := conflictCopies(s.Paths.ConflictsDir()); len(copies) > 0 {
		// Left by earlier agents (unsent content and replaced versions
		// alike); this one never removes them.
		b.WriteString("\n" + i18n.Tf("sync.status.olderCopies", len(copies), homePath(s.Paths.ConflictsDir())))
	}
	if n := historyCount(s.Paths); n > 0 {
		b.WriteString("\n" + i18n.Tf("sync.status.history", n, homePath(s.Paths.HistoryDir()), historyDays))
	}
	if len(st.OverwriteLeft) > 0 {
		b.WriteString("\n" + i18n.Tf("sync.status.overwriteLeft", len(st.OverwriteLeft), s.Cmd))
	}
	if st.LastError != "" {
		b.WriteString("\n" + i18n.Tf("sync.status.lastError", lastErrorText(st.LastError)))
	}
	b.WriteString("\n" + i18n.Tf("sync.status.commands", s.Cmd))
	return b.String()
}

// The texts of the errors sync itself makes that /sync says in the
// person's language: an HTTP status (StatusError), and a request that got
// no answer (*url.Error: the method, the address in quotes, what failed).
var (
	statusErrRe  = regexp.MustCompile(`^sync: HTTP (\d{3})(:|$)`)
	requestErrRe = regexp.MustCompile(`^(Get|Put|Post|Delete|Head|Patch) "[^"]*": `)
)

// lastErrorText — the last error for /sync. sync.json keeps its text as it
// was; the errors sync itself makes are said in the person's language, any
// other as it is.
func lastErrorText(raw string) string {
	switch raw {
	case errKeyChanged.Error(), ErrKeyMismatch.Error():
		return i18n.T("sync.lastError.keyChanged")
	case errNoServerKey.Error(), ErrNoIdentity.Error():
		return i18n.T("sync.lastError.noServerKey")
	}
	if m := statusErrRe.FindStringSubmatch(raw); m != nil {
		code, _ := strconv.Atoi(m[1])
		return i18n.Tf("sync.lastError.http", code)
	}
	if requestErrRe.MatchString(raw) {
		return i18n.T("sync.lastError.connect")
	}
	return raw
}

// countState — what is in sync, by the saved state.
func (s *Service) countState(st *State) Report { return st.Contents() }

// isLocalErr — the error is about a file on this computer, not the server.
func isLocalErr(err error) bool {
	var pe *fs.PathError
	var le *os.LinkError
	return errors.As(err, &pe) || errors.As(err, &le) ||
		errors.Is(err, errLinkedPlace) || errors.Is(err, errNotAFolder) || errors.Is(err, errNotAFile)
}

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
	switch {
	case len(r.Hidden) > 0:
		lines = append(lines, i18n.Tf("sync.status.hidden", strings.Join(s.leftLabels(context.Background(), r.Hidden), ", "), s.Cmd))
	case r.Unreadable > 0:
		lines = append(lines, i18n.Tf("sync.status.unreadable", r.Unreadable, s.Cmd))
	}
	lines = append(lines, skipLines(r.SkippedNew)...)
	if r.QuotaFull {
		lines = append(lines, i18n.T("sync.status.quota"))
	}
	lines = append(lines, keepNotices(r, s.Paths)...)
	return strings.Join(lines, "\n")
}

// Off switches sync off on this machine. The server keeps the data; the
// machine leaves the list in the web console.
func (s *Service) Off(ctx context.Context) string {
	unlock, err := s.lock(ctx, s.commandWait())
	if err != nil {
		return s.failText(err)
	}
	defer unlock()
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
		return s.refuse(v.Message())
	}
	st, err := LoadState(s.Paths)
	if err != nil {
		return i18n.Tf("sync.error", err)
	}
	if !st.Enabled {
		return i18n.Tf("sync.now.off", s.Cmd)
	}
	rep, err := s.exchange(ctx, Options{}, s.commandWait())
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
	case errors.Is(err, errNoServerKey):
		return i18n.Tf("sync.status.pausedNoKey", s.Cmd)
	case errors.Is(err, ErrUnauthorized):
		return i18n.T("sync.gate.login")
	case errors.Is(err, errBusy):
		s.busy.Store(true)
		return i18n.T("sync.busy")
	case isLocalErr(err):
		return i18n.Tf("sync.localFailed", err)
	}
	return i18n.Tf("sync.failed", lastErrorText(err.Error()))
}

var (
	errWiped      = errors.New("sync: the server has no sync data any more")
	errKeyChanged = errors.New("sync: the key on the server is not this machine's key")
	errNoKey      = errors.New("sync: this machine has no key")
	// errNoServerKey — the server says it has no sync key (identity 404, or
	// a write refused with no_identity): paused, nothing reset.
	errNoServerKey = errors.New("sync: the server has no sync key")
)

// exchange — one exchange with the checks every exchange needs. The
// caller has passed the gate. A full exchange first checks the key on the
// server: gone (wiped from another machine) switches sync off here; another
// key (replaced elsewhere) pauses it until the person enters the password.
// errBusy — another process kept the exchange lock longer than wait.
func (s *Service) exchange(ctx context.Context, opt Options, wait time.Duration) (Report, error) {
	unlock, err := s.lock(ctx, wait)
	if err != nil {
		return Report{}, err
	}
	defer unlock()
	return s.exchangeLocked(ctx, opt)
}

// exchangeLocked — exchange for a caller that holds the lock.
func (s *Service) exchangeLocked(ctx context.Context, opt Options) (Report, error) {
	pushOnly := opt.PushOnly
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
			// «No key» from the server, or from something answering in its
			// name: a pause, never a reset. Only the person switches sync
			// off (/sync off) or makes a new key (/sync on).
			st.Paused = PausedNoKey
			_ = st.Save(s.Paths)
			return Report{}, errNoServerKey
		case err != nil:
			st.LastError = err.Error()
			_ = st.Save(s.Paths)
			return Report{}, err
		case WrappedFingerprint(ri.Wrapped) != KeyFingerprint(id):
			st.Paused = PausedKey
			_ = st.Save(s.Paths)
			return Report{}, errKeyChanged
		}
		if st.Paused == PausedKey || st.Paused == PausedNoKey {
			st.Paused = ""
		}
	}
	return s.runLocked(ctx, st, id, opt)
}

// runLocked runs the engine and saves the state. Caller holds s.mu.
func (s *Service) runLocked(ctx context.Context, st *State, id *crypto.Identity, opt Options) (Report, error) {
	// The folders of the last exchange, before the bindings refresh them:
	// an older agent's bases were agreed there (bases.go).
	opt.LastProjects = maps.Clone(st.Projects)
	if opt.LastProjects == nil {
		opt.LastProjects = map[string]string{}
	}
	if !opt.PushOnly && opt.Only == "" {
		// A one-project exchange leaves the other projects' folders as
		// they were: they are compared in their own exchange.
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
	s.Client.KeyFP = KeyFingerprint(id)
	rep, err := eng.Run(ctx, st, opt)
	switch {
	case errors.Is(err, ErrKeyMismatch):
		// A write refused: the key on the server was replaced on another
		// computer meanwhile.
		st.Paused, err = PausedKey, errKeyChanged
	case errors.Is(err, ErrNoIdentity):
		st.Paused, err = PausedNoKey, errNoServerKey
	}
	// A name that is a file on one side and a folder on the other is left
	// as it is, like anything left out: said once, listed by /sync while it
	// lasts.
	for _, c := range rep.Clashes {
		rep.Skipped = append(rep.Skipped, Skip{Path: c, Reason: SkipNameClash})
	}
	rep.Clashes = nil
	if opt.Only == "" {
		if !opt.PushOnly {
			st.Unreadable = rep.Unreadable
			st.Hidden = rep.Hidden
		}
		rep.SkippedNew = newSkips(st.Skipped, rep.Skipped)
		st.Skipped = rep.Skipped
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

// projects — the projects bound to this machine: workspace id → directory.
// The server lists the bindings of all the person's machines; only this
// machine's count — another machine's path may exist here too, and without
// a machine id (a browser token) nothing is this machine's. A directory
// that is not here stays in the list: the exchange leaves the project out
// and says so (local.go, SkipNoFolder) instead of forgetting it silently.
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
		if self == "" || b.AgentID != self {
			continue // another machine's binding
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
		return s.refuse(v.Message())
	}
	unlock, err := s.lock(ctx, s.commandWait())
	if err != nil {
		return s.failText(err)
	}
	if name != "" {
		if st2, err := LoadState(s.Paths); err == nil {
			if st2.ProjectNames == nil {
				st2.ProjectNames = map[string]string{}
			}
			st2.ProjectNames[ws] = name
			_ = st2.Save(s.Paths)
		}
	}
	rep, err := s.exchangeLocked(ctx, Options{Only: blobKey(KindProjectMemory, ws), Projects: map[string]string{ws: dir}})
	unlock()
	if err != nil {
		return s.failText(err)
	}
	var lines []string
	got, sent := rep.Pulled+rep.Removed > 0, rep.Pushed > 0
	// A line about this project the exchange makes anyway — a copy kept, an
	// older copy put back, files removed, the project kept or sent back up —
	// says what was done: one line for one event, not «received», «sent» or
	// «merged» and then why.
	said := len(rep.Unsent)+len(rep.Rollback)+len(rep.RollbackGone)+len(rep.DeletedThere)+
		len(rep.Resent)+len(rep.ReturnSent)+len(rep.ProjectsKept)+len(rep.ProjectsBack) > 0
	switch {
	case said:
	case got && sent:
		lines = append(lines, i18n.T("sync.project.merged"))
	case got:
		lines = append(lines, i18n.T("sync.project.pulled"))
	case sent:
		lines = append(lines, i18n.T("sync.project.pushed"))
	case rep.Projects > 0:
		lines = append(lines, i18n.T("sync.project.same"))
	case len(rep.Skipped) == 0:
		lines = append(lines, i18n.T("sync.project.empty"))
	}
	rep.Restored = nil // said by the first line: received from the server
	lines = append(lines, keepNotices(rep, s.Paths)...)
	lines = append(lines, skipLines(rep.Skipped)...)
	return strings.Join(lines, "\n")
}

// newSkips — what is left out now and was not at the latest exchange: said
// once, not on every exchange (/sync lists them all).
func newSkips(before, now []Skip) []Skip {
	had := map[Skip]bool{}
	for _, sk := range before {
		had[sk] = true
	}
	var out []Skip
	for _, sk := range now {
		if !had[sk] {
			out = append(out, sk)
		}
	}
	return out
}
