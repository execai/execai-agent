// Package machine describes this computer the way the person needs to see
// it: what is connected (account, model sources), what syncs, and whether
// the machine can be called from the web chat and the phone — plus the
// five-step tutorial built from the same facts.
//
// One snapshot feeds every interface: /machine and /start in the terminal,
// the machine_state event of `execai ide` (the editor plugin and Studio),
// and the link behind /phone. Build is pure; Local reads this machine's own
// files, the network is only asked for the folder bindings (CountBindings).
package machine

import (
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/inbox"
	"github.com/velesbsdllc/agent-vbai/internal/plan"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
	agentsync "github.com/velesbsdllc/agent-vbai/internal/sync"
)

// Tutorial steps, in the order they are shown.
const (
	StepLogin   = "login"
	StepModels  = "models"
	StepSync    = "sync"
	StepProject = "project"
	StepPhone   = "phone"
)

// StepIDs — the tutorial in order.
var StepIDs = []string{StepLogin, StepModels, StepSync, StepProject, StepPhone}

// Which projects the machine syncs and takes tasks for.
const (
	ModeAll      = "all"
	ModeSelected = "selected"
	ModeNone     = "none"
)

// Why the machine cannot be called from the web and the phone yet.
const (
	BlockLogin    = "login"    // not signed in as an agent of this machine
	BlockPlan     = "plan"     // the plan does not include the web console
	BlockListener = "listener" // nothing here accepts tasks (execai serve, an editor window)
)

// ExecAIID — the id of the ExecAI models among the sources.
const ExecAIID = subscriptions.SourceExecAI

// Source — one model source and whether it answered its latest check.
type Source struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Alive bool   `json:"alive"`
	// CheckedAt — RFC 3339 time of the latest answer or check; "" = never.
	CheckedAt string `json:"checked_at"`

	// Active — the source the next turn goes to (terminal only).
	Active bool `json:"-"`
	// Checked — CheckedAt as a time; zero = never.
	Checked time.Time `json:"-"`
}

// Sync — what travels between the person's computers from this one.
type Sync struct {
	Enabled bool `json:"enabled"`
	// LastPushAt — RFC 3339 time of the latest upload; "" = none yet.
	LastPushAt string `json:"last_push_at"`
	// Config — the config with the provider keys is in sync.
	Config              bool   `json:"config"`
	PersonalMemoryFiles int    `json:"personal_memory_files"`
	Projects            int    `json:"projects"`
	Mode                string `json:"mode"`

	// Paused — why sync waits (agentsync.Paused*), "" = it does not.
	Paused   string    `json:"-"`
	LastPush time.Time `json:"-"`
}

// Presence — whether the machine can be called from the web and the phone.
type Presence struct {
	// ListenerRunning — something on this machine accepts tasks from the
	// site: execai serve or an editor window (the inbox lock is held).
	ListenerRunning bool `json:"listener_running"`
	// ReachableFromWeb — a task from the web chat or the phone gets here.
	ReachableFromWeb bool `json:"reachable_from_web"`

	// ListenerKind — inbox.KindServe or inbox.KindIDE; ListenerPID — its pid.
	ListenerKind string `json:"-"`
	ListenerPID  int    `json:"-"`
	// Blockers — Block* in the order to fix them; empty when reachable.
	Blockers []string `json:"-"`
}

// State — the machine card.
type State struct {
	// ID — the agent id of this machine (empty when not signed in as an agent).
	ID      string `json:"id"`
	Name    string `json:"name"`
	OS      string `json:"os"`
	Version string `json:"version"`

	LoggedIn    bool   `json:"logged_in"`
	EmailMasked string `json:"email_masked"`
	// Plan — the tariff code of the active ExecAI plan as billing names it
	// (week_work, week_max, month_start…), "" when there is none. The
	// interfaces translate it themselves.
	Plan string `json:"plan"`
	// PlanTitle — billing's display name of that plan (terminal only).
	PlanTitle string `json:"-"`
	// PlanPending — signed in, but billing has not answered yet (no saved
	// answer): the plan is being checked, which is not "no plan".
	PlanPending bool `json:"plan_pending,omitempty"`

	Sources  []Source `json:"sources"`
	Sync     Sync     `json:"sync"`
	Presence Presence `json:"presence"`

	// Arch — the CPU architecture (terminal only).
	Arch string `json:"-"`
}

// Step — one tutorial step and whether it is done.
type Step struct {
	ID   string `json:"id"`
	Done bool   `json:"done"`
}

// Tutorial — the five steps in order.
type Tutorial struct {
	Steps []Step `json:"steps"`
}

// Done counts the finished steps.
func (t Tutorial) Done() int {
	n := 0
	for _, s := range t.Steps {
		if s.Done {
			n++
		}
	}
	return n
}

// IsDone reports whether the step id is finished.
func (t Tutorial) IsDone(id string) bool {
	for _, s := range t.Steps {
		if s.ID == id {
			return s.Done
		}
	}
	return false
}

// Snapshot — what every interface shows: the card, the tutorial, the link
// for the phone ("" without an agent id).
type Snapshot struct {
	Machine  State    `json:"machine"`
	Tutorial Tutorial `json:"tutorial"`
	PhoneURL string   `json:"phone_url"`
	// PayURL — where to pick a plan on this installation's web (terminal
	// only).
	PayURL string `json:"-"`
}

// Inputs — the facts a snapshot is built from. Local fills them from this
// machine; tests write them by hand.
type Inputs struct {
	Creds    *config.Credentials // nil = never signed in
	APIBase  string
	Hostname string
	OS, Arch string
	Version  string
	Plan     *plan.Plan           // the latest answer from billing, nil = none
	Subs     *subscriptions.Store // nil = no own sources
	Checks   Checks               // results of /machine check
	Sync     *agentsync.State     // nil = sync never set up here
	Listener *inbox.Owner         // the live owner of the inbox, nil = none
	// Bindings — folders of this machine bound to web projects, by the
	// server; -1 = unknown (offline, not asked).
	Bindings int
	Marks    Marks
	Now      time.Time
}

// Build makes the snapshot. Pure: no files, no network.
func Build(in Inputs) Snapshot {
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}
	cr := in.Creds
	if cr == nil {
		cr = &config.Credentials{}
	}
	signedIn := cr.Token != ""

	st := State{
		ID:       cr.AgentID,
		Name:     firstNonEmpty(cr.Alias, in.Hostname),
		OS:       in.OS,
		Arch:     in.Arch,
		Version:  in.Version,
		LoggedIn: signedIn,
	}
	if signedIn {
		st.EmailMasked = MaskEmail(cr.Email)
		if p := in.Plan; p != nil && !p.LoggedOut && p.Granted() {
			st.Plan = p.TariffCode
			st.PlanTitle = firstNonEmpty(p.Title, p.TariffCode)
		}
		st.PlanPending = in.Plan == nil
	}
	st.Sources = sources(in, signedIn, now)
	st.Sync = syncPart(in.Sync)
	st.Presence = presence(in, cr, now)

	alive := false
	for _, s := range st.Sources {
		alive = alive || s.Alive
	}
	syncDone := in.Sync != nil && in.Sync.Enabled && in.Sync.Paused == "" &&
		(!in.Sync.LastPull.IsZero() || !in.Sync.LastPush.IsZero())
	projectDone := in.Bindings > 0 ||
		(in.Bindings < 0 && in.Sync != nil && len(in.Sync.Projects) > 0)
	done := map[string]bool{
		StepLogin:   signedIn,
		StepModels:  alive,
		StepSync:    syncDone,
		StepProject: projectDone,
		StepPhone:   !in.Marks.WebCallAt.IsZero() || !in.Marks.PhoneShownAt.IsZero(),
	}
	tut := Tutorial{Steps: make([]Step, 0, len(StepIDs))}
	for _, id := range StepIDs {
		tut.Steps = append(tut.Steps, Step{ID: id, Done: done[id]})
	}

	return Snapshot{
		Machine:  st,
		Tutorial: tut,
		PhoneURL: PhoneURL(WebBase(in.APIBase, cr.WebBase), cr.AgentID),
		PayURL:   PaymentURL(in.APIBase, cr.WebBase),
	}
}

// sources — the ExecAI models first, then the person's own sources.
//
// ExecAI answers when billing says the plan is active (the same check every
// turn makes). An own source answers when its model list came back — at
// /connect, on a refresh, or at /machine check; a failed /machine check
// newer than the last answer means it does not.
func sources(in Inputs, signedIn bool, now time.Time) []Source {
	ex := Source{ID: ExecAIID, Title: subscriptions.ProviderName(ExecAIID)}
	if in.Subs == nil || in.Subs.ActiveSubscription() == nil {
		ex.Active = true
	}
	if signedIn && in.Plan != nil && !in.Plan.LoggedOut {
		ex.Alive = in.Plan.Active(now)
		ex.Checked = in.Plan.CheckedAt
	}
	out := []Source{stamp(ex)}
	if in.Subs == nil {
		return out
	}
	active := in.Subs.ActiveSubscription()
	for _, sub := range in.Subs.List() {
		s := Source{ID: sub.Provider, Title: subscriptions.DisplayName(sub),
			Active: active != nil && active.Provider == sub.Provider}
		if !sub.ModelsFetchedAt.IsZero() {
			s.Alive, s.Checked = true, sub.ModelsFetchedAt
		}
		if c, ok := in.Checks[sub.Provider]; ok && !c.At.Before(s.Checked) {
			s.Alive, s.Checked = c.OK, c.At
		}
		out = append(out, stamp(s))
	}
	return out
}

func stamp(s Source) Source {
	s.CheckedAt = rfc3339(s.Checked)
	return s
}

func syncPart(st *agentsync.State) Sync {
	// Projects sync only where a folder is bound to them (/project); the
	// choice "all / selected / none" for the machine comes with the web side
	// of the bindings (E105 part B). Until then every machine is "selected".
	out := Sync{Mode: ModeSelected}
	if st == nil {
		return out
	}
	c := st.Contents()
	out.Enabled = st.Enabled
	out.Config = c.Config
	out.PersonalMemoryFiles = c.Memory
	out.Projects = c.Projects
	if out.Projects == 0 {
		out.Projects = len(st.Projects)
	}
	out.Paused = st.Paused
	out.LastPush = st.LastPush
	out.LastPushAt = rfc3339(st.LastPush)
	return out
}

// presence — tasks reach this machine when it is signed in as an agent
// (tasks are addressed to an agent id), the plan includes the web console,
// and the inbox has an owner here.
func presence(in Inputs, cr *config.Credentials, now time.Time) Presence {
	var p Presence
	if o := in.Listener; o != nil {
		p.ListenerRunning = true
		p.ListenerKind = o.Kind
		if p.ListenerKind == "" {
			p.ListenerKind = inbox.KindServe
		}
		p.ListenerPID = o.PID
	}
	switch {
	case cr.Token == "" || cr.AgentID == "":
		p.Blockers = append(p.Blockers, BlockLogin)
	case in.Plan == nil || !in.Plan.Allows(plan.FeatureBoard, now):
		p.Blockers = append(p.Blockers, BlockPlan)
	}
	if !p.ListenerRunning {
		p.Blockers = append(p.Blockers, BlockListener)
	}
	p.ReachableFromWeb = len(p.Blockers) == 0
	return p
}

func rfc3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// MaskEmail hides an address down to its first letters:
// "alice@example.com" → "a***@e***.com". Shown wherever the screen may be
// seen by others or shared (a screenshot, a demo).
func MaskEmail(email string) string {
	email = strings.TrimSpace(email)
	if email == "" {
		return ""
	}
	at := strings.LastIndexByte(email, '@')
	if at < 0 {
		return firstRune(email) + "***"
	}
	local, domain := email[:at], email[at+1:]
	out := firstRune(local) + "***@"
	dot := strings.IndexByte(domain, '.')
	if dot < 0 {
		return out + firstRune(domain) + "***"
	}
	return out + firstRune(domain[:dot]) + "***" + domain[dot:]
}

func firstRune(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	if n == 0 || r == utf8.RuneError {
		return ""
	}
	return string(r)
}

// WebBase — the address of the web chat (config.WebBase): the one the
// sign-in was confirmed on, or derived from api_base.
func WebBase(apiBase, saved string) string { return config.WebBase(apiBase, saved) }

// PaymentURL — the page to pick a plan on, on the web of this installation
// (config.PaymentURL, campaign "gate").
func PaymentURL(apiBase, savedWebBase string) string {
	return config.PaymentURL(apiBase, savedWebBase, "gate")
}

// PhoneURL — the page that opens the web chat with this machine chosen for
// tasks; "" without an agent id.
func PhoneURL(webBase, agentID string) string {
	if agentID == "" || webBase == "" {
		return ""
	}
	return strings.TrimRight(webBase, "/") + "/m/" + url.PathEscape(agentID)
}
