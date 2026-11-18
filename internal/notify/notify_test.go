package notify

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)

func TestParseMode(t *testing.T) {
	for in, want := range map[string]Mode{"": ModeSystem, "system": ModeSystem, " Bell ": ModeBell, "OFF": ModeOff} {
		if got, ok := ParseMode(in); !ok || got != want {
			t.Errorf("ParseMode(%q) = %q, %v", in, got, ok)
		}
	}
	if got, ok := ParseMode("loud"); ok || got != ModeSystem {
		t.Errorf("unknown: %q, %v", got, ok)
	}
}

// The window is away: every finished turn is announced, however short.
func TestTurn_FocusOut(t *testing.T) {
	var w Watcher
	w.SetFocus(false, t0)
	w.TurnStarted(t0, false)
	if !w.TurnEnded(t0.Add(time.Second), false) {
		t.Fatal("a turn finished while away was not announced")
	}
}

// The window is in front of the person: nothing, however long.
func TestTurn_FocusIn(t *testing.T) {
	var w Watcher
	w.SetFocus(true, t0)
	w.TurnStarted(t0, false)
	if w.TurnEnded(t0.Add(10*time.Minute), false) {
		t.Fatal("announced a turn the person watched")
	}
}

// Focus unknown: the threshold since the person last typed.
func TestTurn_FocusUnknown(t *testing.T) {
	var w Watcher
	w.TurnStarted(t0, false)
	if w.TurnEnded(t0.Add(TurnAfter-time.Second), false) {
		t.Fatal("a short turn was announced")
	}
	w.TurnStarted(t0, false)
	if !w.TurnEnded(t0.Add(TurnAfter), false) {
		t.Fatal("a long turn was not announced")
	}
	// Typing during the turn: the person is here, the clock starts again.
	w.TurnStarted(t0, false)
	w.Touch(t0.Add(20 * time.Second))
	if w.TurnEnded(t0.Add(40*time.Second), false) {
		t.Fatal("announced although the person typed 20 s ago")
	}
}

// A turn the person stopped, or no turn at all, is never announced; one
// turn — one announcement.
func TestTurn_StoppedAndOnce(t *testing.T) {
	var w Watcher
	w.SetFocus(false, t0)
	if w.TurnEnded(t0, false) {
		t.Fatal("announced without a turn")
	}
	w.TurnStarted(t0, false)
	if w.TurnEnded(t0.Add(time.Minute), true) {
		t.Fatal("announced a stopped turn")
	}
	w.TurnStarted(t0, false)
	if !w.TurnEnded(t0.Add(time.Minute), false) || w.TurnEnded(t0.Add(time.Minute), false) {
		t.Fatal("one turn must be announced exactly once")
	}
}

// A key pressed while the window was reported away: the report is stale.
func TestTouch_FixesStaleBlur(t *testing.T) {
	var w Watcher
	w.SetFocus(false, t0)
	w.Touch(t0)
	if w.Focus() != FocusIn {
		t.Fatalf("focus %v after a key", w.Focus())
	}
	w.ResetFocus()
	if w.Focus() != FocusUnknown {
		t.Fatal("reset")
	}
}

// The window is away when the question comes: tell at once, and only once.
func TestWait_FocusOut(t *testing.T) {
	var w Watcher
	w.SetFocus(false, t0)
	id, now := w.WaitStarted(t0, "ExecAI is waiting for approval: Bash")
	if !now {
		t.Fatal("not told at once")
	}
	if _, ok := w.WaitDue(id); ok {
		t.Fatal("told twice about one question")
	}
}

// In focus when it comes: nothing at once; nothing later while in focus;
// if the person left meanwhile, the timer tells them.
func TestWait_FocusIn(t *testing.T) {
	var w Watcher
	w.SetFocus(true, t0)
	id, now := w.WaitStarted(t0, "q")
	if now {
		t.Fatal("told at once while in focus")
	}
	if _, ok := w.WaitDue(id); ok {
		t.Fatal("told while in focus")
	}
	id, _ = w.WaitStarted(t0, "q2")
	w.SetFocus(false, t0.Add(2*time.Second))
	if note, ok := w.WaitDue(id); !ok || note != "q2" {
		t.Fatalf("left during the question: %q %v", note, ok)
	}
	if _, ok := w.WaitDue(id); ok {
		t.Fatal("told twice")
	}
}

// Focus unknown: the timer tells, unless the person typed since.
func TestWait_FocusUnknown(t *testing.T) {
	var w Watcher
	id, now := w.WaitStarted(t0, "q")
	if now {
		t.Fatal("told at once with the focus unknown")
	}
	if note, ok := w.WaitDue(id); !ok || note != "q" {
		t.Fatal("not told after the threshold")
	}
	id, _ = w.WaitStarted(t0, "q2")
	w.Touch(t0.Add(3 * time.Second))
	if _, ok := w.WaitDue(id); ok {
		t.Fatal("told although the person is typing")
	}
}

// An answered question, a stale timer, a new question: no mix-ups.
func TestWait_EndedAndStale(t *testing.T) {
	var w Watcher
	id1, _ := w.WaitStarted(t0, "q1")
	w.WaitEnded()
	if _, ok := w.WaitDue(id1); ok {
		t.Fatal("told about an answered question")
	}
	id2, _ := w.WaitStarted(t0, "q2")
	if _, ok := w.WaitDue(id1); ok {
		t.Fatal("a stale timer told")
	}
	w.TurnStarted(t0, false) // a new turn withdraws whatever was open
	if _, ok := w.WaitDue(id2); ok {
		t.Fatal("told about a question of the previous turn")
	}
	if _, ok := w.WaitDue(0); ok {
		t.Fatal("id 0")
	}
}

// Background work (/loop, a wake-up): its turns are never announced as
// finished; the caller asks Away for the events that are news.
func TestTurn_Background(t *testing.T) {
	var w Watcher
	w.SetFocus(false, t0)
	w.TurnStarted(t0, true)
	if !w.InTurn() {
		t.Fatal("not in a turn")
	}
	if w.TurnEnded(t0.Add(time.Hour), false) {
		t.Fatal("a background turn was announced")
	}
	if w.InTurn() {
		t.Fatal("the turn did not end")
	}
	// A question inside it is still a question.
	if _, now := w.WaitStarted(t0, "q"); !now {
		t.Fatal("a question in background work was not told")
	}
}

// Away: the window is away, or, with the focus unknown, nothing was typed
// for TurnAfter.
func TestAway(t *testing.T) {
	var w Watcher
	if !w.Away(t0) {
		t.Fatal("unknown focus, never typed: away")
	}
	w.Touch(t0)
	if w.Away(t0.Add(TurnAfter - time.Second)) {
		t.Fatal("typed a moment ago")
	}
	if !w.Away(t0.Add(TurnAfter)) {
		t.Fatal("typed long ago")
	}
	w.SetFocus(true, t0)
	if w.Away(t0.Add(time.Hour)) {
		t.Fatal("in focus")
	}
	w.SetFocus(false, t0)
	if !w.Away(t0) {
		t.Fatal("blurred")
	}
}
