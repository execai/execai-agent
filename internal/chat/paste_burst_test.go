package chat

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

func newBurstTestModel() *tuiModel {
	ta := textarea.New()
	ta.Focus()
	ta.SetWidth(80)
	ta.SetHeight(1)
	ta.CharLimit = 32768
	m := &tuiModel{
		pasteStore:   map[int]pasteEntry{},
		cfg:          &config.Config{APIBase: "https://api.execai.ru"},
		creds:        &config.Credentials{Token: "stub"},
		subs:         &subscriptions.Store{Subscriptions: map[string]subscriptions.Subscription{}},
		models:       []llm.Model{{ID: "m1", Provider: "p", IsPrimary: true}},
		execAIModels: []llm.Model{{ID: "m1", Provider: "p", IsPrimary: true}},
		textarea:     ta,
	}
	return m
}

// sendKey delivers a KeyMsg through Update and returns the updated model.
func sendKey(t *testing.T, m *tuiModel, msg tea.KeyMsg) *tuiModel {
	t.Helper()
	res, _ := m.Update(msg)
	mm, ok := res.(*tuiModel)
	if !ok {
		t.Fatalf("Update returned %T", res)
	}
	return mm
}

// TestPasteBurst_RapidEnterDoesNotSubmit — an Enter arriving <8ms after a rune
// key (SSH paste without bracketed paste) must NOT submit; it becomes part of
// the burst buffer.
func TestPasteBurst_RapidEnterDoesNotSubmit(t *testing.T) {
	m := newBurstTestModel()

	// First key: types normally (gap is huge — lastKeyAt is zero).
	m = sendKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	// Simulate paste stream: next keys arrive "instantly".
	m.lastKeyAt = time.Now() // as if 'h' just happened
	m = sendKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	if !m.pasteBurstActive {
		t.Fatal("burst should be active after two rapid keys")
	}
	// Rapid Enter — must go into the buffer, not submit.
	m = sendKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.pasteBurstActive {
		t.Fatal("burst should stay active across rapid Enter")
	}
	if got := m.pasteBurst.String(); !strings.Contains(got, "\n") {
		t.Errorf("burst buffer should contain newline, got %q", got)
	}
	// History must be empty — nothing was submitted to the agent.
	if len(m.history) != 0 {
		t.Errorf("nothing should be submitted during burst, history=%d", len(m.history))
	}
}

// TestPasteBurst_FlushCollapsesMultiline — flushing a multiline burst inserts
// a [Pasted #N] marker and stores the full text.
func TestPasteBurst_FlushCollapsesMultiline(t *testing.T) {
	m := newBurstTestModel()
	m.pasteBurstActive = true
	m.pasteBurst.WriteString("line one\nline two\nline three")
	m.pasteFlushGen = 7

	res, _ := m.Update(pasteFlushMsg{gen: 7})
	m = res.(*tuiModel)

	if m.pasteBurstActive {
		t.Fatal("burst should be inactive after flush")
	}
	val := m.textarea.Value()
	if !strings.Contains(val, "[Pasted #1") {
		t.Errorf("textarea should contain paste marker, got %q", val)
	}
	stored, ok := m.pasteStore[1]
	if !ok {
		t.Fatal("pasteStore should contain entry #1")
	}
	if stored.text != "line one\nline two\nline three" {
		t.Errorf("stored text mismatch: %q", stored.text)
	}
	if stored.lines != 3 {
		t.Errorf("want 3 lines, got %d", stored.lines)
	}
}

// TestPasteBurst_StaleTimerIgnored — a flush with an outdated gen is a no-op.
func TestPasteBurst_StaleTimerIgnored(t *testing.T) {
	m := newBurstTestModel()
	m.pasteBurstActive = true
	m.pasteBurst.WriteString("partial")
	m.pasteFlushGen = 10

	res, _ := m.Update(pasteFlushMsg{gen: 9}) // stale
	m = res.(*tuiModel)

	if !m.pasteBurstActive {
		t.Fatal("stale flush must not deactivate the burst")
	}
	if m.pasteBurst.String() != "partial" {
		t.Fatal("stale flush must not consume the buffer")
	}
}

// TestPasteBurst_SmallFlushInsertsDirectly — a short single-line burst is
// inserted as plain text (no marker).
func TestPasteBurst_SmallFlushInsertsDirectly(t *testing.T) {
	m := newBurstTestModel()
	m.pasteBurstActive = true
	m.pasteBurst.WriteString("hello")
	m.pasteFlushGen = 1

	res, _ := m.Update(pasteFlushMsg{gen: 1})
	m = res.(*tuiModel)

	if got := m.textarea.Value(); got != "hello" {
		t.Errorf("small burst should insert directly, got %q", got)
	}
	if len(m.pasteStore) != 0 {
		t.Errorf("no marker expected for small paste")
	}
}

// TestSlowTyping_NoBurst — keys at human speed never enter burst mode and
// Enter submits normally (here: empty input → no submit, but no burst either).
func TestSlowTyping_NoBurst(t *testing.T) {
	m := newBurstTestModel()
	m = sendKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m.lastKeyAt = time.Now().Add(-100 * time.Millisecond) // human-speed gap
	m = sendKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	if m.pasteBurstActive {
		t.Fatal("human-speed typing must not trigger burst mode")
	}
	if got := m.textarea.Value(); got != "hi" {
		t.Errorf("textarea should contain typed text, got %q", got)
	}
}
