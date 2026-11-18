package machine

import (
	"encoding/json"
	"os"
	"path/filepath"
	gosync "sync"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/config"
)

// MarksFile — the onboarding marks of this machine, in the config
// directory. Not in config.json on purpose: that file travels to the
// person's other computers with sync, and these marks are about this one.
const MarksFile = "onboarding.json"

// Marks — what has happened on this machine that the tutorial counts.
type Marks struct {
	// PhoneShownAt — the first time /phone showed this machine's link.
	PhoneShownAt time.Time `json:"phone_shown_at,omitempty"`
	// WebCallAt — the first time a task from the web chat or the phone
	// reached this machine.
	WebCallAt time.Time `json:"web_call_at,omitempty"`
}

var marksMu gosync.Mutex

func marksPath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, MarksFile), nil
}

// LoadMarks reads the marks; a missing or broken file means none.
func LoadMarks() Marks {
	var m Marks
	p, err := marksPath()
	if err != nil {
		return m
	}
	if data, err := os.ReadFile(p); err == nil {
		_ = json.Unmarshal(data, &m)
	}
	return m
}

// MarkPhoneShown records the first /phone; true when this was the first.
func MarkPhoneShown() bool {
	return mark(func(m *Marks) *time.Time { return &m.PhoneShownAt })
}

// MarkWebCall records the first task from the site; true when this was the
// first.
func MarkWebCall() bool {
	return mark(func(m *Marks) *time.Time { return &m.WebCallAt })
}

func mark(field func(*Marks) *time.Time) bool {
	marksMu.Lock()
	defer marksMu.Unlock()
	m := LoadMarks()
	t := field(&m)
	if !t.IsZero() {
		return false
	}
	*t = time.Now().UTC()
	p, err := marksPath()
	if err != nil {
		return false
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return false
	}
	return os.WriteFile(p, data, 0o600) == nil
}
