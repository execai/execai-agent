package sync

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/velesbsdllc/agent-vbai/internal/crypto"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// Step — one turn of a short conversation with the person: the /sync on
// setup, a password change, the forget confirmation, a key import. The
// chat and the shell drive the same flows, each with its own input.
type Step struct {
	// Text — what to show now; may be empty.
	Text string
	// Prompt — the next question; "" when the flow is over.
	Prompt string
	// Secret — the answer is a password: hidden while typed, never echoed,
	// never kept in the input history.
	Secret bool
	// Quiet — the answer is visible while typed but not echoed into the
	// transcript (a transfer code, a phrase).
	Quiet bool
}

// Done reports whether the flow is over.
func (s Step) Done() bool { return s.Prompt == "" }

// Flow — a conversation. Start once, then Answer until Done. Calls may
// take seconds (network, password hashing): run them off the UI loop.
type Flow interface {
	Start(ctx context.Context) Step
	Answer(ctx context.Context, answer string) Step
}

const (
	stageEnter = iota + 1
	stageNew
	stageRepeat
	stageChoose
	stageCode
	stagePhrase
	stageConfirm
	stageLeft
)

// ===== /sync on =====

type onFlow struct {
	s      *Service
	stage  int
	remote *RemoteIdentity
	local  *crypto.Identity
	taken  *crypto.Identity // the server's key, when it differs from local
	// password lives only between two questions: «repeat it», or «keep my
	// key» which re-wraps this machine's key with it.
	password string
	// fresh — this machine's key is new or replaced: what sync.json says
	// about the server belongs to the old one.
	fresh bool
	// loses — the projects keeping this computer's key deletes from the
	// server (choosePrompt); nil — not listed yet.
	loses []string
	// seen — what the choice deletes: blob key → the version shown
	// (overwrite.go).
	seen map[string]int64
}

// OnFlow — /sync on: the plan check, then either «enter the password from
// your first computer» or «make up a password», then the first exchange.
func (s *Service) OnFlow() Flow { return &onFlow{s: s} }

func (f *onFlow) Start(ctx context.Context) Step {
	s := f.s
	if v := s.Gate.Check(ctx, true); v != Allowed {
		return Step{Text: s.refuse(v.Message())}
	}
	local, err := loadKey(s.Paths.ConfigDir)
	if err != nil {
		return Step{Text: i18n.Tf("sync.error", err)}
	}
	f.local = local
	ri, err := s.Client.GetIdentity(ctx)
	switch {
	case errors.Is(err, ErrNotFound):
		return f.askNew("")
	case err != nil:
		return Step{Text: s.failText(err)}
	}
	f.remote = ri
	if local != nil && WrappedFingerprint(ri.Wrapped) == KeyFingerprint(local) {
		// This machine already holds the key: no password needed. What
		// keeping it left on the server earlier is asked about again.
		// What keeping a computer's key left on the server, and projects no
		// computer can open any more (encrypted with a key nobody has now):
		// asked about here, on every /sync on until decided.
		if st, err := LoadState(s.Paths); err == nil && st.Enabled && (len(st.OverwriteLeft) > 0 || len(st.Hidden) > 0) {
			keys := append([]string(nil), st.OverwriteLeft...)
			for _, k := range st.Hidden {
				if !slices.Contains(keys, k) {
					keys = append(keys, k)
				}
			}
			if seen, _ := s.stillLeft(ctx, local, keys); len(seen) > 0 {
				f.seen, f.stage = seen, stageLeft
				// What stayed because it changed after the question, and
				// what is under an earlier key for another reason: each
				// said with its own cause.
				var changed, other []string
				for _, k := range keys {
					if _, ok := seen[k]; !ok {
						continue
					}
					if slices.Contains(st.OverwriteLeft, k) {
						changed = append(changed, k)
					} else {
						other = append(other, k)
					}
				}
				var text []string
				if len(changed) > 0 {
					text = append(text, i18n.Tf("sync.on.leftIntro", strings.Join(s.leftLabels(ctx, changed), ", ")))
				}
				if len(other) > 0 {
					text = append(text, i18n.Tf("sync.on.hiddenIntro", strings.Join(s.leftLabels(ctx, other), ", ")))
				}
				return Step{Text: strings.Join(text, "\n"), Prompt: i18n.T("sync.on.leftChoose")}
			}
		}
		return Step{Text: f.finish(ctx, Options{}, i18n.T("sync.on.done"))}
	}
	f.stage = stageEnter
	return Step{Text: i18n.T("sync.on.joinIntro"), Prompt: i18n.T("sync.on.enterPassword"), Secret: true}
}

func (f *onFlow) askNew(text string) Step {
	f.stage = stageNew
	if text == "" {
		text = i18n.T("sync.on.newIntro")
	}
	return Step{Text: text, Prompt: i18n.Tf("sync.on.newPassword", MinPasswordLen), Secret: true}
}

func (f *onFlow) Answer(ctx context.Context, a string) Step {
	s := f.s
	switch f.stage {
	case stageEnter:
		id, err := UnwrapIdentity(f.remote.Wrapped, a)
		if err != nil {
			return Step{Text: i18n.T("sync.password.wrong"), Prompt: i18n.T("sync.on.enterPassword"), Secret: true}
		}
		if f.local == nil {
			if err := s.locked(ctx, func() error { return crypto.SaveIdentity(s.Paths.ConfigDir, id) }); err != nil {
				return Step{Text: s.localErr(err)}
			}
			f.fresh = true
			return Step{Text: f.finish(ctx, Options{Join: true}, i18n.T("sync.on.joined"))}
		}
		if KeyFingerprint(f.local) == KeyFingerprint(id) {
			return Step{Text: f.finish(ctx, Options{Join: true}, i18n.T("sync.on.joined"))}
		}
		f.taken, f.password = id, a
		f.stage = stageChoose
		return Step{Text: i18n.T("sync.on.keyDiffers"), Prompt: f.choosePrompt(ctx)}

	case stageChoose:
		switch strings.TrimSpace(a) {
		case "1":
			f.password = ""
			var backup string
			err := s.locked(ctx, func() (err error) {
				backup, err = replaceKey(s.Paths.ConfigDir, f.taken, s.now())
				return err
			})
			if err != nil {
				return Step{Text: s.localErr(err)}
			}
			f.fresh = true
			head := i18n.T("sync.on.joined")
			if backup != "" {
				head += "\n" + i18n.Tf("sync.on.oldKeySaved", backup)
			}
			return Step{Text: f.finish(ctx, Options{Join: true, NewKey: true}, head)}
		case "2":
			wrapped, err := WrapIdentity(f.local, f.password)
			f.password = ""
			if err != nil {
				return Step{Text: i18n.Tf("sync.error", err)}
			}
			if _, err := s.Client.PutIdentity(ctx, wrapped, f.remote.Version); err != nil {
				var ce *ConflictError
				if errors.As(err, &ce) {
					return Step{Text: i18n.Tf("sync.on.raced", s.Cmd)}
				}
				return Step{Text: s.failText(err)}
			}
			f.fresh = true
			return Step{Text: f.finish(ctx, Options{Overwrite: true, OverwriteSeen: f.seen, NewKey: true}, i18n.T("sync.on.replaced"))}
		}
		return Step{Prompt: f.choosePrompt(ctx)}

	case stageLeft:
		head := i18n.T("sync.on.done")
		switch strings.TrimSpace(a) {
		case "1":
			n, kept, err := s.deleteLeft(ctx, f.seen)
			if err != nil {
				return Step{Text: s.failText(err)}
			}
			head += "\n" + i18n.Tf("sync.on.leftDeleted", n)
			if kept > 0 {
				head += "\n" + i18n.Tf("sync.on.leftChanged", kept, s.Cmd)
			}
			// What this computer has of them goes up now, with its key.
			return Step{Text: f.finish(ctx, Options{NewKey: true}, head)}
		case "2":
			head += "\n" + i18n.T("sync.on.leftKept")
		default:
			return Step{Prompt: i18n.T("sync.on.leftChoose")}
		}
		return Step{Text: f.finish(ctx, Options{}, head)}

	case stageNew:
		if PasswordTooShort(a) {
			return Step{Text: i18n.Tf("sync.password.short", MinPasswordLen),
				Prompt: i18n.Tf("sync.on.newPassword", MinPasswordLen), Secret: true}
		}
		f.password = a
		f.stage = stageRepeat
		return Step{Prompt: i18n.T("sync.on.repeatPassword"), Secret: true}

	case stageRepeat:
		if a != f.password {
			f.password = ""
			return f.askNew(i18n.T("sync.password.mismatch"))
		}
		key := f.local
		if key == nil {
			var err error
			if key, err = crypto.Generate(); err != nil {
				return Step{Text: i18n.Tf("sync.error", err)}
			}
			// Saved before it goes to the server; while another process
			// exchanges, neither happens.
			if err := s.locked(ctx, func() error { return crypto.SaveIdentity(s.Paths.ConfigDir, key) }); err != nil {
				f.password = ""
				return Step{Text: s.localErr(err)}
			}
			f.local = key
			f.fresh = true
		}
		wrapped, err := WrapIdentity(key, f.password)
		f.password = ""
		if err != nil {
			return Step{Text: i18n.Tf("sync.error", err)}
		}
		_, err = s.Client.PutIdentity(ctx, wrapped, 0)
		var ce *ConflictError
		if errors.As(err, &ce) {
			// Another computer set its password a moment ago: join it.
			ri, gerr := s.Client.GetIdentity(ctx)
			if gerr != nil {
				return Step{Text: s.failText(gerr)}
			}
			f.remote = ri
			f.stage = stageEnter
			return Step{Text: i18n.T("sync.on.racedJoin"), Prompt: i18n.T("sync.on.enterPassword"), Secret: true}
		}
		if err != nil {
			return Step{Text: s.failText(err)}
		}
		return Step{Text: f.finish(ctx, Options{}, i18n.T("sync.on.created"))}
	}
	return Step{}
}

// choosePrompt — the choice of the key, with the projects that keeping
// this computer's key deletes from the server named (engine.go,
// overwrite). If they cannot be listed, the question says that projects
// not on this computer would go, without names.
func (f *onFlow) choosePrompt(ctx context.Context) string {
	if f.loses == nil {
		seen, names, err := f.s.overwriteSeen(ctx, f.local)
		f.seen = seen // nil when it could not be listed: then nothing is deleted
		switch {
		case err != nil:
			f.loses = []string{"?"}
		case names == nil:
			f.loses = []string{}
		default:
			f.loses = names
		}
	}
	text := i18n.T("sync.on.chooseKey")
	switch {
	case len(f.loses) == 1 && f.loses[0] == "?":
		text += "\n" + i18n.T("sync.on.chooseKeyProjectsUnknown")
	case len(f.loses) > 0:
		text += "\n" + i18n.Tf("sync.on.chooseKeyProjects", strings.Join(f.loses, ", "))
	}
	return text
}

// finish switches sync on and runs the first exchange.
func (f *onFlow) finish(ctx context.Context, opt Options, head string) string {
	s := f.s
	unlock, err := s.lock(ctx, s.commandWait())
	if err != nil {
		return s.failText(err)
	}
	defer unlock()
	st, err := LoadState(s.Paths)
	if err != nil {
		return i18n.Tf("sync.error", err)
	}
	if f.fresh {
		st.reset()
		if err := resetBases(s.Paths); err != nil {
			return i18n.Tf("sync.error", err)
		}
	}
	st.Enabled = true
	st.Paused = ""
	if err := st.Save(s.Paths); err != nil {
		return i18n.Tf("sync.error", err)
	}
	id, err := loadKey(s.Paths.ConfigDir)
	if err != nil || id == nil {
		return i18n.Tf("sync.error", err)
	}
	rep, err := s.runLocked(ctx, st, id, opt)
	if err != nil {
		return head + "\n" + i18n.Tf("sync.on.firstFailed", err, s.Cmd)
	}
	out := s.summary(head, rep)
	if opt.Overwrite || len(st.OverwriteLeft) > 0 {
		if opt.Overwrite || f.stage == stageLeft {
			st.OverwriteLeft = rep.OverwriteLeft
		}
		if len(rep.OverwriteLeft) > 0 {
			out += "\n" + i18n.Tf("sync.on.overwriteLeft", strings.Join(s.leftLabels(ctx, rep.OverwriteLeft), ", "), s.Cmd)
		}
		if err := st.Save(s.Paths); err != nil {
			return i18n.Tf("sync.error", err)
		}
	}
	return out
}

// stillLeft — of what keeping this computer's key left on the server, the
// blobs still there and still unreadable with this key: key → version, and
// how to name them.
func (s *Service) stillLeft(ctx context.Context, id *crypto.Identity, keys []string) (map[string]int64, string) {
	remote, err := s.Client.List(ctx, 0)
	if err != nil {
		return nil, ""
	}
	want := map[string]bool{}
	for _, k := range keys {
		want[k] = true
	}
	eng := &Engine{Client: s.Client, ID: id, Paths: s.Paths, Now: s.Now}
	seen := map[string]int64{}
	var left []string
	for _, m := range remote {
		k := blobKey(m.Kind, m.Name)
		if !want[k] || m.Deleted {
			continue
		}
		if f, err := eng.fetch(ctx, m.Kind, m.Name, m.Version); err != nil || !f.hidden {
			continue // readable now: not a leftover any more
		}
		seen[k] = m.Version
		left = append(left, k)
	}
	return seen, strings.Join(s.leftLabels(ctx, left), ", ")
}

// leftLabels — how to name blobs left on the server under another key:
// projects by name, memory files by number, the settings as such.
func (s *Service) leftLabels(ctx context.Context, keys []string) []string {
	var ws []string
	memory, settings, others := 0, false, 0
	for _, k := range keys {
		switch kind, name := splitKey(k); kind {
		case KindProjectMemory:
			ws = append(ws, name)
		case KindMemory:
			memory++
		case KindConfig:
			settings = true
		default:
			others++
		}
	}
	out := s.projectLabels(ctx, ws)
	if memory > 0 {
		out = append(out, countPhrase("sync.hidden.memory", memory))
	}
	if settings {
		out = append(out, i18n.T("sync.hidden.settings"))
	}
	if others > 0 {
		out = append(out, i18n.Tf("sync.on.leftOthers", others))
	}
	return out
}

// deleteLeft deletes the leftovers at the versions just shown; one written
// meanwhile stays (kept): it is asked about again on the next /sync on.
func (s *Service) deleteLeft(ctx context.Context, seen map[string]int64) (n, kept int, err error) {
	unlock, err := s.lock(ctx, s.commandWait())
	if err != nil {
		return 0, 0, err
	}
	defer unlock()
	for k, v := range seen {
		kind, name := splitKey(k)
		_, err := s.Client.Delete(ctx, kind, name, v)
		var ce *ConflictError
		switch {
		case errors.As(err, &ce):
			kept++
			continue
		case errors.Is(err, ErrNotFound), errors.Is(err, ErrGone):
			continue
		case err != nil:
			return n, kept, err
		}
		n++
	}
	return n, kept, nil
}

// ===== /sync password =====

type passwordFlow struct {
	s        *Service
	stage    int
	remote   *RemoteIdentity
	local    *crypto.Identity
	password string
}

// PasswordFlow — /sync password: wrap the same key with a new password.
// Other machines keep working: the password is needed only to add one.
func (s *Service) PasswordFlow() Flow { return &passwordFlow{s: s} }

func (f *passwordFlow) Start(ctx context.Context) Step {
	s := f.s
	if v := s.Gate.Check(ctx, true); v != Allowed {
		return Step{Text: s.refuse(v.Message())}
	}
	st, err := LoadState(s.Paths)
	if err != nil {
		return Step{Text: i18n.Tf("sync.error", err)}
	}
	local, err := loadKey(s.Paths.ConfigDir)
	if err != nil {
		return Step{Text: i18n.Tf("sync.error", err)}
	}
	if !st.Enabled || local == nil {
		return Step{Text: i18n.Tf("sync.now.off", s.Cmd)}
	}
	ri, err := s.Client.GetIdentity(ctx)
	switch {
	case errors.Is(err, ErrNotFound):
		return Step{Text: i18n.Tf("sync.bg.wiped", s.Cmd)}
	case err != nil:
		return Step{Text: s.failText(err)}
	case WrappedFingerprint(ri.Wrapped) != KeyFingerprint(local):
		return Step{Text: i18n.Tf("sync.status.pausedKey", s.Cmd)}
	}
	f.remote, f.local, f.stage = ri, local, stageNew
	return Step{Prompt: i18n.Tf("sync.password.new", MinPasswordLen), Secret: true}
}

func (f *passwordFlow) Answer(ctx context.Context, a string) Step {
	s := f.s
	switch f.stage {
	case stageNew:
		if PasswordTooShort(a) {
			return Step{Text: i18n.Tf("sync.password.short", MinPasswordLen),
				Prompt: i18n.Tf("sync.password.new", MinPasswordLen), Secret: true}
		}
		f.password, f.stage = a, stageRepeat
		return Step{Prompt: i18n.T("sync.on.repeatPassword"), Secret: true}
	case stageRepeat:
		if a != f.password {
			f.password, f.stage = "", stageNew
			return Step{Text: i18n.T("sync.password.mismatch"),
				Prompt: i18n.Tf("sync.password.new", MinPasswordLen), Secret: true}
		}
		wrapped, err := WrapIdentity(f.local, f.password)
		f.password = ""
		if err != nil {
			return Step{Text: i18n.Tf("sync.error", err)}
		}
		_, err = s.Client.PutIdentity(ctx, wrapped, f.remote.Version)
		var ce *ConflictError
		if errors.As(err, &ce) {
			// Changed meanwhile; if it is still our key, write over it once.
			ri, gerr := s.Client.GetIdentity(ctx)
			if gerr != nil || WrappedFingerprint(ri.Wrapped) != KeyFingerprint(f.local) {
				return Step{Text: i18n.Tf("sync.status.pausedKey", s.Cmd)}
			}
			_, err = s.Client.PutIdentity(ctx, wrapped, ri.Version)
		}
		if err != nil {
			return Step{Text: s.failText(err)}
		}
		return Step{Text: i18n.T("sync.password.changed")}
	}
	return Step{}
}

// ===== /sync forget =====

type forgetFlow struct{ s *Service }

// ForgetFlow — /sync forget: delete everything from the server after the
// person types the confirmation word. Works without a plan: deleting one's
// data is never behind a paywall.
func (s *Service) ForgetFlow() Flow { return &forgetFlow{s: s} }

func (f *forgetFlow) Start(ctx context.Context) Step {
	if !f.s.signedIn() {
		return Step{Text: f.s.refuse(i18n.T("sync.gate.login"))}
	}
	return Step{Text: i18n.T("sync.forget.warn"), Prompt: i18n.Tf("sync.forget.confirm", i18n.T("sync.forget.word"))}
}

func (f *forgetFlow) Answer(ctx context.Context, a string) Step {
	s := f.s
	a = strings.TrimSpace(a)
	if !strings.EqualFold(a, i18n.T("sync.forget.word")) && !strings.EqualFold(a, "delete") {
		return Step{Text: i18n.T("sync.forget.cancelled")}
	}
	// Locked before the server is wiped: an exchange of another process
	// running meanwhile would put its files back and save sync on again.
	unlock, err := s.lock(ctx, s.commandWait())
	if err != nil {
		return Step{Text: s.failText(err)}
	}
	defer unlock()
	n, err := s.Client.DeleteAll(ctx)
	if err != nil {
		return Step{Text: s.failText(err)}
	}
	st, _ := LoadState(s.Paths)
	st.Enabled = false
	st.Paused = ""
	st.reset()
	_ = resetBases(s.Paths)
	if err := st.Save(s.Paths); err != nil {
		return Step{Text: i18n.Tf("sync.error", err)}
	}
	return Step{Text: i18n.Tf("sync.forget.done", n, s.Cmd)}
}
