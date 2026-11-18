package config

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/notify"
)

// The notifications setting: system by default, an unknown word reads as
// the default, and it is saved under "notifications" only when set.
func TestNotifications(t *testing.T) {
	for in, want := range map[string]notify.Mode{"": notify.ModeSystem, "bell": notify.ModeBell, "off": notify.ModeOff, "loud": notify.ModeSystem} {
		c := &Config{Notifications: in}
		if got := c.GetNotifications(); got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
	var nilCfg *Config
	if nilCfg.GetNotifications() != notify.ModeSystem {
		t.Error("nil config")
	}
	raw, _ := json.Marshal(Defaults())
	if strings.Contains(string(raw), "notifications") {
		t.Errorf("an unset setting is written: %s", raw)
	}
	raw, _ = json.Marshal(&Config{APIBase: "x", Notifications: "off"})
	if !strings.Contains(string(raw), `"notifications":"off"`) {
		t.Errorf("not saved: %s", raw)
	}
}
