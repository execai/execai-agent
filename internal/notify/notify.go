// Package notify tells the person that the agent needs them while they look
// at another window: a turn has finished, or the agent waits for a
// permission or an answer.
//
// Two halves. Watcher decides WHEN: it keeps the focus reports of the
// terminal and the state of the turn and of the open question, and says
// whether an event is worth a notification. Notifier decides HOW: a desktop
// notification, the terminal's own notification sequence, the bell — chosen
// once from the OS and the environment (deliver.go).
//
// Neither half ever sees a prompt or an answer: the caller builds the text,
// and it names a folder or a tool at most.
package notify

import (
	"strings"
	"time"
)

// Mode is the "notifications" setting. It belongs to the machine: it lives
// in config.json, but sync leaves it out (internal/sync, machineOnlyKeys).
type Mode string

const (
	// ModeSystem — a desktop notification (the terminal's own one where it
	// has it, otherwise the OS tool) and the bell. The default.
	ModeSystem Mode = "system"
	// ModeBell — only what travels through the terminal: its notification
	// sequence and the bell. Starts no program; works over SSH.
	ModeBell Mode = "bell"
	// ModeOff — nothing.
	ModeOff Mode = "off"
)

// Modes lists the settings in the order they are shown.
func Modes() []Mode { return []Mode{ModeSystem, ModeBell, ModeOff} }

// ParseMode reads a setting. An empty one is the default; ok=false for an
// unknown word, which also reads as the default.
func ParseMode(s string) (Mode, bool) {
	switch Mode(strings.ToLower(strings.TrimSpace(s))) {
	case "", ModeSystem:
		return ModeSystem, true
	case ModeBell:
		return ModeBell, true
	case ModeOff:
		return ModeOff, true
	}
	return ModeSystem, false
}

// Focus is what the terminal has said about its window.
type Focus int

const (
	// FocusUnknown — no report yet. Many terminals never send one (and on
	// Windows the console input does not carry them to the program).
	FocusUnknown Focus = iota
	// FocusIn — the window is in front of the person.
	FocusIn
	// FocusOut — the person is in another window.
	FocusOut
)

const (
	// TurnAfter — with the focus unknown, a finished turn is announced when
	// the person has not typed anything in the terminal for this long.
	TurnAfter = 30 * time.Second
	// WaitAfter — a question still open after this long is announced, unless
	// the window is known to be in focus (or, with the focus unknown, the
	// person has typed since it came).
	WaitAfter = 15 * time.Second
)

// Watcher decides when the person should hear from the agent. It only keeps
// state; the caller feeds it events and delivers. Not safe for concurrent
// use: the TUI calls it from its update loop.
type Watcher struct {
	focus Focus
	// seen — the last time the person did something in this terminal.
	seen time.Time

	inTurn bool
	// background — the turn is work the person put in the background (a
	// /loop iteration, a wake-up): its end is not announced as such.
	background bool
	turnAt     time.Time

	// The open question: its id (0 — none), the notification text, when it
	// came and whether the person has been told about it already.
	waitSeq  int
	waitID   int
	waitNote string
	waitAt   time.Time
	waitTold bool
}

// Focus returns the last focus report.
func (w *Watcher) Focus() Focus { return w.focus }

// SetFocus records a focus report of the terminal.
func (w *Watcher) SetFocus(in bool, now time.Time) {
	if in {
		w.focus = FocusIn
		w.seen = now
		return
	}
	w.focus = FocusOut
}

// ResetFocus forgets the reports: reporting was switched off and on again.
func (w *Watcher) ResetFocus() { w.focus = FocusUnknown }

// Touch — the person pressed a key here. Keys reach only the window in
// focus, so a report of the window being away is stale.
func (w *Watcher) Touch(now time.Time) {
	w.seen = now
	if w.focus == FocusOut {
		w.focus = FocusIn
	}
}

// TurnStarted — the agent began a turn. A turn the person started counts as
// them being here; a background one (a /loop iteration, a wake-up) does not.
func (w *Watcher) TurnStarted(now time.Time, background bool) {
	w.inTurn = true
	w.background = background
	w.turnAt = now
	if background {
		w.turnAt = w.seen
	}
	w.endWait()
}

// InTurn — a turn is running.
func (w *Watcher) InTurn() bool { return w.inTurn }

// TurnEnded — the turn is over; true when the person should hear about it.
// stopped — the person stopped it themselves, there is nothing to tell. A
// background turn is never announced as finished: the caller decides what
// of background work is news and asks Away.
func (w *Watcher) TurnEnded(now time.Time, stopped bool) bool {
	if !w.inTurn {
		return false
	}
	w.inTurn = false
	w.endWait()
	if stopped || w.background {
		return false
	}
	switch w.focus {
	case FocusOut:
		return true
	case FocusIn:
		return false
	}
	last := w.turnAt
	if w.seen.After(last) {
		last = w.seen
	}
	return now.Sub(last) >= TurnAfter
}

// Away — the person is not known to be looking: the window is away, or the
// focus is unknown and nothing was typed for TurnAfter.
func (w *Watcher) Away(now time.Time) bool {
	switch w.focus {
	case FocusOut:
		return true
	case FocusIn:
		return false
	}
	return w.seen.IsZero() || now.Sub(w.seen) >= TurnAfter
}

// WaitStarted — the agent asks the person something; note is the text of
// the notification. tellNow=true — the window is away, tell them at once.
// Otherwise the caller asks WaitDue(id) again after WaitAfter.
func (w *Watcher) WaitStarted(now time.Time, note string) (id int, tellNow bool) {
	w.waitSeq++
	w.waitID = w.waitSeq
	w.waitNote = note
	w.waitAt = now
	w.waitTold = w.focus == FocusOut
	return w.waitID, w.waitTold
}

// WaitDue — the timer of question id fired. It returns the note when the
// same question is still open, nobody has been told about it yet, and the
// person is not known to be looking: the window is away, or the focus is
// unknown and nothing was typed since the question came.
func (w *Watcher) WaitDue(id int) (string, bool) {
	if id == 0 || id != w.waitID || w.waitTold {
		return "", false
	}
	switch w.focus {
	case FocusIn:
		return "", false
	case FocusUnknown:
		if w.seen.After(w.waitAt) {
			return "", false
		}
	}
	w.waitTold = true
	return w.waitNote, true
}

// WaitEnded — the question was answered or withdrawn.
func (w *Watcher) WaitEnded() { w.endWait() }

func (w *Watcher) endWait() {
	w.waitID = 0
	w.waitNote = ""
	w.waitTold = false
}
