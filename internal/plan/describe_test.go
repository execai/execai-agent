package plan

import (
	"strings"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

func TestDescribe(t *testing.T) {
	defer i18n.SetLocale("en")
	work, _ := Parse([]byte(`{"tariff_code":"week_work","title":"Работа","state":"active",
		"valid_until":"2026-11-05T22:22:28Z","is_accessible":true}`), t0)
	trial, _ := Parse([]byte(bodyTrial), t0)
	grace, _ := Parse([]byte(bodyGrace), t0)
	none, _ := Parse([]byte(bodyNone), t0)
	flexible, _ := Parse([]byte(bodyFlexible), t0)
	stamp := func(tm time.Time) string { return tm.Local().Format("2006-01-02 15:04") }

	cases := []struct {
		name     string
		locale   string
		p        *Plan
		signedIn bool
		offline  bool
		now      time.Time
		want     []string
		absent   []string
	}{
		{"en work plan", "en", &work, true, false, t0,
			[]string{"ExecAI plan", "active", "Работа (week_work)", stamp(work.ValidUntil), stamp(t0),
				"every source", "Sync and web console: available"},
			[]string{SubscribeURL}},
		{"ru work plan", "ru", &work, true, false, t0,
			[]string{"План ExecAI", "активен", "Работа (week_work)", "все источники",
				"Синк и веб-пульт: доступны"},
			[]string{SubscribeURL}},
		{"en trial", "en", &trial, true, false, t0,
			[]string{"active (trial)", "Sync and web console: requires the Work or Max plan"}, nil},
		{"ru trial", "ru", &trial, true, false, t0,
			[]string{"активен (пробный период)", "нужен план «Работа» или «Макс»"}, nil},
		{"en grace", "en", &grace, true, false, t0,
			[]string{"grace", stamp(grace.GraceUntil), "every source"}, nil},
		{"en no plan", "en", &none, true, false, t0,
			[]string{"no active plan", "your own keys and local models; ExecAI models need a plan", SubscribeURL}, nil},
		{"ru no plan", "ru", &none, true, false, t0,
			[]string{"нет активного плана", "свои ключи и локальные модели; модели ExecAI — по подписке", SubscribeURL}, nil},
		{"en flexible", "en", &flexible, true, false, t0,
			[]string{"no active plan", "your own keys and local models; ExecAI models need a plan"}, nil},
		{"en offline cache", "en", &work, true, true, t0.Add(2 * 24 * time.Hour),
			[]string{"saved answer", "every source"}, nil},
		{"en offline too old", "en", &work, true, true, t0.Add(9 * 24 * time.Hour),
			[]string{"older than 7 days", "your own keys and local models; ExecAI models need a plan"}, nil},
		{"en signed out", "en", nil, false, false, t0,
			[]string{"not signed in", "/login", "your own keys and local models; ExecAI models need a plan"}, nil},
		{"ru signed out", "ru", nil, false, false, t0,
			[]string{"вход не выполнен", "/login"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			i18n.SetLocale(c.locale)
			out := Describe(c.p, c.signedIn, c.offline, c.now)
			for _, w := range c.want {
				if !strings.Contains(out, w) {
					t.Errorf("lacks %q:\n%s", w, out)
				}
			}
			for _, a := range c.absent {
				if strings.Contains(out, a) {
					t.Errorf("must not contain %q:\n%s", a, out)
				}
			}
		})
	}
}
