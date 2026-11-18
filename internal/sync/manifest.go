package sync

import (
	"encoding/json"
	"strings"
	"unicode"

	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

// The server takes up to 50 names of up to 64 printable characters and
// refuses the whole manifest otherwise; a name a person typed for a custom
// endpoint can be longer.
const (
	maxManifestSources = 50
	maxManifestField   = 64
)

// manifestField — a name as the server accepts it.
func manifestField(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return -1
	}, strings.TrimSpace(s))
	if r := []rune(s); len(r) > maxManifestField {
		s = strings.TrimSpace(string(r[:maxManifestField]))
	}
	return s
}

// Manifest — what the web console learns about this machine. Sent as is,
// NOT encrypted, so it carries names only: which sources are set up here,
// which one is active, the agent version. Never a key, an address or a
// header of a source — the type has no field that could hold one.
type Manifest struct {
	Sources      []string `json:"sources"`
	Active       string   `json:"active"`
	AgentVersion string   `json:"agent_version"`
}

// BuildManifest lists the sources of this machine by their display names:
// the ExecAI models first (always there), then each connected provider.
func BuildManifest(subs *subscriptions.Store, agentVersion string) Manifest {
	execai := subscriptions.ProviderName(subscriptions.SourceExecAI)
	m := Manifest{Sources: []string{execai}, Active: execai, AgentVersion: agentVersion}
	if subs == nil {
		return m
	}
	seen := map[string]bool{execai: true}
	for _, s := range subs.List() {
		name := manifestField(subscriptions.DisplayName(s))
		if name == "" || seen[name] || len(m.Sources) >= maxManifestSources {
			continue
		}
		seen[name] = true
		m.Sources = append(m.Sources, name)
	}
	if a := subs.ActiveSubscription(); a != nil {
		m.Active = manifestField(subscriptions.DisplayName(*a))
	}
	m.AgentVersion = manifestField(m.AgentVersion)
	return m
}

// hash — to send the manifest only when it changed.
func (m Manifest) hash() string {
	b, _ := json.Marshal(m)
	return hashOf(b)
}
