package sync

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

// The manifest carries names only: no key, no address, no header — and
// names the server will accept.
func TestManifest_NamesOnly(t *testing.T) {
	subs := &subscriptions.Store{Subscriptions: map[string]subscriptions.Subscription{}}
	subs.Add(subscriptions.Subscription{Provider: subscriptions.SourceKimiAPI, APIKey: "sk-moonshot-secret"})
	subs.Add(subscriptions.Subscription{Provider: "corp", API: subscriptions.APIOpenAI, Name: strings.Repeat("Corp gateway ", 10),
		APIKey: "sk-corp-secret", BaseURL: "https://llm.corp.internal/v1", Headers: map[string]string{"X-Tenant": "tenant-secret"}})
	_ = subs.Activate(subscriptions.SourceKimiAPI)

	m := BuildManifest(subs, "6.98")
	raw, _ := json.Marshal(m)
	for _, bad := range []string{"sk-", "secret", "llm.corp", "X-Tenant", "api_key", "base_url"} {
		if strings.Contains(string(raw), bad) {
			t.Fatalf("manifest leaks %q: %s", bad, raw)
		}
	}
	if m.Sources[0] != "ExecAI" || m.Active != "Moonshot" || len(m.Sources) != 3 {
		t.Fatalf("manifest: %+v", m)
	}
	for _, s := range m.Sources {
		if len([]rune(s)) > maxManifestField {
			t.Fatalf("name too long for the server: %q", s)
		}
	}
	if BuildManifest(nil, "dev").Active != "ExecAI" {
		t.Fatal("no sources: ExecAI")
	}
}
