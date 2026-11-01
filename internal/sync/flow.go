package sync

import (
	"context"
	"errors"
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
}

// OnFlow — /sync on: the plan check, then either «enter the password from
// your first computer» or «make up a password», then the first exchange.
func (s *Service) OnFlow() Flow { return &onFlow{s: s} }

func (f *onFlow) Start(ctx context.Context) Step {
	s := f.s
	if v := s.Gate.Check(ctx, true); v != Allowed {
		return Step{Text: v.Message()}
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
		// This machine already holds the key: no password needed.
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
			if err := crypto.SaveIdentity(s.Paths.ConfigDir, id); err != nil {
				return Step{Text: i18n.Tf("sync.error", err)}
			}
			f.fresh = true
			return Step{Text: f.finish(ctx, Options{Join: true}, i18n.T("sync.on.joined"))}
		}
		if KeyFingerprint(f.local) == KeyFingerprint(id) {
			return Step{Text: f.finish(ctx, Options{Join: true}, i18n.T("sync.on.joined"))}
		}
		f.taken, f.password = id, a
		f.stage = stageChoose
		return Step{Text: i18n.T("sync.on.keyDiffers"), Prompt: i18n.T("sync.on.chooseKey")}

	case stageChoose:
		switch strings.TrimSpace(a) {
		case "1":
			f.password = ""
			backup, err := replaceKey(s.Paths.ConfigDir, f.taken, s.now())
			if err != nil {
				return Step{Text: i18n.Tf("sync.error", err)}
			}
			f.fresh = true
			head := i18n.T("sync.on.joined")
			if backup != "" {
				head += "\n" + i18n.Tf("sync.on.oldKeySaved", backup)
			}
			return Step{Text: f.finish(ctx, Options{Join: true}, head)}
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
			return Step{Text: f.finish(ctx, Options{Overwrite: true}, i18n.T("sync.on.replaced"))}
		}
		return Step{Prompt: i18n.T("sync.on.chooseKey")}

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
			if err := crypto.SaveIdentity(s.Paths.ConfigDir, key); err != nil {
				return Step{Text: i18n.Tf("sync.error", err)}
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

// finish switches sync on and runs the first exchange.
func (f *onFlow) finish(ctx context.Context, opt Options, head string) string {
	s := f.s
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := LoadState(s.Paths)
	if err != nil {
		return i18n.Tf("sync.error", err)
	}
	if f.fresh {
		st.reset()
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
	return s.summary(head, rep)
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
		return Step{Text: v.Message()}
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
		return Step{Text: i18n.T("sync.gate.login")}
	}
	return Step{Text: i18n.T("sync.forget.warn"), Prompt: i18n.Tf("sync.forget.confirm", i18n.T("sync.forget.word"))}
}

func (f *forgetFlow) Answer(ctx context.Context, a string) Step {
	s := f.s
	a = strings.TrimSpace(a)
	if !strings.EqualFold(a, i18n.T("sync.forget.word")) && !strings.EqualFold(a, "delete") {
		return Step{Text: i18n.T("sync.forget.cancelled")}
	}
	n, err := s.Client.DeleteAll(ctx)
	if err != nil {
		return Step{Text: s.failText(err)}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st, _ := LoadState(s.Paths)
	st.Enabled = false
	st.Paused = ""
	st.reset()
	if err := st.Save(s.Paths); err != nil {
		return Step{Text: i18n.Tf("sync.error", err)}
	}
	return Step{Text: i18n.Tf("sync.forget.done", n, s.Cmd)}
}
