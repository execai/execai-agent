package messages

import (
	"strings"
	"testing"
)

// Every text that names the plans with sync and tasks from the web and the
// phone names all three of them: «Работа», «Макс» and «Месяц» (Work, Max,
// Month). A plan left out of one line sends a paying person to buy what they
// already have.
func TestPlanLists_NameTheMonthPlan(t *testing.T) {
	keys := map[string][]string{
		"sync":    {"sync.gate.noPlan", "sync.status.off", "sync.status.pausedPlan", "sync.cmd.long"},
		"machine": {"start.sync.cmd", "machine.block.plan", "phone.plan"},
		"inbox":   {"inbox.plan.refused"},
		"plan":    {"plan.features.no"},
	}
	locales := []struct {
		loc   string
		names []string
		cats  map[string]map[string]string
	}{
		{"ru", []string{"«Работа»", "«Макс»", "«Месяц»"}, map[string]map[string]string{
			"sync": ruSyncMessages, "machine": ruMachineMessages, "inbox": ruInboxMessages, "plan": ruPlanMessages}},
		{"en", []string{"Work", "Max", "Month"}, map[string]map[string]string{
			"sync": enSyncMessages, "machine": enMachineMessages, "inbox": enInboxMessages, "plan": enPlanMessages}},
		{"de", []string{"Work", "Max", "Month"}, map[string]map[string]string{
			"sync": deSyncMessages, "machine": deMachineMessages, "inbox": deInboxMessages}},
		{"es", []string{"Work", "Max", "Month"}, map[string]map[string]string{
			"sync": esSyncMessages, "machine": esMachineMessages, "inbox": esInboxMessages}},
		{"zh", []string{"Work", "Max", "Month"}, map[string]map[string]string{
			"sync": zhSyncMessages, "machine": zhMachineMessages, "inbox": zhInboxMessages}},
	}
	for _, l := range locales {
		for group, cat := range l.cats {
			for _, k := range keys[group] {
				v, ok := cat[k]
				if !ok {
					t.Errorf("%s: %s is missing", l.loc, k)
					continue
				}
				for _, name := range l.names {
					if !strings.Contains(v, name) {
						t.Errorf("%s: %s does not name %s: %q", l.loc, k, name, v)
					}
				}
			}
		}
	}
}
