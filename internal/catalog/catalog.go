// Package catalog builds the model list for a source.
//
// Why it exists. The same question — "which models does this source have?" —
// used to be answered twice: in the TUI (internal/chat) with human-readable
// names, and in the editor protocol (internal/ide) by dumping the raw IDs the
// provider returned. One source of truth removes that class of bug for good:
// every surface — TUI, `execai ide`, `execai serve` — calls For().
//
// The rule for every source: the list is what the provider itself reports
// (fetched at /connect, refreshed daily, stored in the subscription). Nothing
// is built in — the built-in catalogs this package used to merge in went
// stale within weeks and kept offering models the account did not have.
// Names come from the provider when it sends them, otherwise they are derived
// from the ID.
package catalog

import (
	"regexp"
	"sort"
	"strings"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

// For returns the model catalog of a subscription. It may be empty: the
// provider has not been asked yet or did not answer. Callers fetch then
// (connect.Fill) and, failing that, let the person type a model ID.
func For(sub subscriptions.Subscription) []llm.Model {
	// Models typed by the person come first: an endpoint without /models,
	// or one that does not list everything it serves.
	live := withManual(sub.LiveModels(), sub.ManualModels)
	switch sub.Provider {
	case subscriptions.SourceClaudeCLI, subscriptions.SourceCodexCLI:
		return cliCatalog(sub.Provider, live)
	case subscriptions.SourceOpenRouter:
		return openRouter(live)
	}
	if sub.Provider == subscriptions.SourceOpenAI || sub.IsCustom() {
		live = chatOnly(live)
	}
	prefer := preferred[sub.Provider]
	if sub.Provider == subscriptions.SourceOpenAI {
		live = sortOpenAI(live)
		prefer = plainAliases(live)
	}
	return build(sub.Provider, live, prefer, true)
}

// build turns a live list into catalog entries, keeping the provider's order.
// The primary model is the provider's own default when it names one, else the
// first ID found in prefer, else the first entry.
func build(provider string, live []subscriptions.ModelMeta, prefer []string, hasTools bool) []llm.Model {
	out := make([]llm.Model, 0, len(live))
	primary := -1
	for i, m := range live {
		if m.Default && primary < 0 {
			primary = i
		}
	}
	if primary < 0 {
		have := map[string]int{}
		for i, m := range live {
			have[m.ID] = i
		}
		for _, want := range prefer {
			if i, ok := have[want]; ok {
				primary = i
				break
			}
		}
	}
	if primary < 0 && len(live) > 0 {
		primary = 0
	}
	for i, m := range live {
		name := m.Name
		if name == "" {
			name = prettyName(m.ID)
		}
		out = append(out, llm.Model{
			ID: m.ID, Provider: provider, Name: name, Description: m.Description,
			Tier: "standard", HasTools: hasTools, IsPrimary: i == primary,
		})
	}
	return out
}

// preferred — which model to start on when the provider names no default and
// the person has not picked one yet. Only an order of preference: a model
// missing from the live list is simply skipped, nothing is added.
var preferred = map[string][]string{
	subscriptions.SourceZAI:     {"glm-5.3", "glm-5.2", "glm-5.1", "glm-5", "glm-4.7"},
	subscriptions.SourceZAIAPI:  {"glm-5.3", "glm-5.2", "glm-5.1", "glm-5", "glm-4.7"},
	subscriptions.SourceKimi:    {"k3", "kimi-for-coding"},
	subscriptions.SourceKimiAPI: {"kimi-k3", "kimi-k2.7-code", "kimi-k2.6", "kimi-latest"},
	subscriptions.SourceOllama:  {"glm-5.3", "glm-5.2", "kimi-k3", "qwen3-coder:480b-cloud", "deepseek-v4-flash"},
}

// cliCatalog — Claude Code and Codex. Their tools run inside the CLI, not
// through ours (HasTools=false). An entry with an empty ID ("do not pass
// --model") is always there: it works with any CLI version and is what a
// fresh connection starts on even when the CLI could not be asked.
func cliCatalog(provider string, live []subscriptions.ModelMeta) []llm.Model {
	hasDefault := false
	for _, m := range live {
		if m.ID == "" {
			hasDefault = true
			break
		}
	}
	if !hasDefault {
		def := subscriptions.ModelMeta{ID: "", Name: i18n.T("catalog.cliDefault"), Default: true,
			Description: i18n.T("catalog.cliDefaultDesc")}
		for _, m := range live {
			if m.Default {
				// Codex marks a real model as its default; the empty entry
				// is then only a fallback and must not steal the primary.
				def.Default = false
				break
			}
		}
		live = append([]subscriptions.ModelMeta{def}, live...)
	}
	return build(provider, live, nil, false)
}

// withManual puts the IDs a person typed first and appends whatever the
// provider itself reports.
func withManual(live []subscriptions.ModelMeta, manual []string) []subscriptions.ModelMeta {
	if len(manual) == 0 {
		return live
	}
	out := make([]subscriptions.ModelMeta, 0, len(manual)+len(live))
	seen := map[string]bool{}
	for _, id := range manual {
		if !seen[id] {
			seen[id] = true
			out = append(out, subscriptions.ModelMeta{ID: id})
		}
	}
	for _, m := range live {
		if !seen[m.ID] {
			seen[m.ID] = true
			out = append(out, m)
		}
	}
	return out
}

// nonChat — fragments of IDs that cannot serve a chat turn. OpenAI's /models
// lists embeddings, speech, images and moderation next to chat models;
// local servers (vLLM, LM Studio) list their embedding models too. Offering
// those in a picker only produces errors.
var nonChat = []string{
	"embedding", "embed-", "-embed", "tts", "whisper", "transcribe", "dall-e",
	"gpt-image", "moderation", "davinci", "babbage", "-audio", "realtime",
	"sora", "rerank",
}

func chatOnly(live []subscriptions.ModelMeta) []subscriptions.ModelMeta {
	out := make([]subscriptions.ModelMeta, 0, len(live))
	for _, m := range live {
		low := strings.ToLower(m.ID)
		skip := false
		for _, frag := range nonChat {
			if strings.Contains(low, frag) {
				skip = true
				break
			}
		}
		if !skip {
			out = append(out, m)
		}
	}
	return out
}

// openAIFamilies — OpenAI returns its list in no useful order; the families
// people code with go first, newest version first within a family.
var openAIFamilies = []string{"gpt-5", "o3", "gpt-4.1", "gpt-4o", "o4", "codex", "o1", "gpt-4", "chatgpt"}

// plainAlias — a flagship alias without a size or date suffix: "gpt-5.5",
// "o3". A fresh connection starts on the first of them rather than on a
// "-mini" or a dated snapshot that happens to sort first.
var plainAlias = regexp.MustCompile(`^(gpt-\d+(\.\d+)?|o\d+)$`)

func plainAliases(live []subscriptions.ModelMeta) []string {
	var out []string
	for _, m := range live {
		if plainAlias.MatchString(m.ID) {
			out = append(out, m.ID)
		}
	}
	return out
}

// snapshotSuffix — a dated pin of an alias: "gpt-4o-2024-08-06", "gpt-4-0613".
var snapshotSuffix = regexp.MustCompile(`-(\d{4}-\d{2}-\d{2}|\d{4})$`)

func sortOpenAI(live []subscriptions.ModelMeta) []subscriptions.ModelMeta {
	rank := func(id string) int {
		for i, p := range openAIFamilies {
			if strings.HasPrefix(id, p) {
				return i
			}
		}
		return len(openAIFamilies)
	}
	out := append([]subscriptions.ModelMeta(nil), live...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i].ID, out[j].ID
		if ra, rb := rank(a), rank(b); ra != rb {
			return ra < rb
		}
		// Newest version first, the alias before its dated snapshots.
		if ka, kb := snapshotSuffix.ReplaceAllString(a, ""), snapshotSuffix.ReplaceAllString(b, ""); ka != kb {
			return ka > kb
		}
		if len(a) != len(b) {
			return len(a) < len(b)
		}
		return a > b
	})
	return out
}

// openRouterTop — vendors and families worth putting on top of a catalog
// that is several hundred models long. Prefix match on the OpenRouter ID
// ("anthropic/claude-sonnet-4.5"), best first.
var openRouterTop = []string{
	"anthropic/claude-opus", "anthropic/claude-sonnet", "anthropic/claude-haiku",
	"openai/gpt-5", "openai/o3", "openai/gpt-4.1",
	"google/gemini-2.5-pro", "google/gemini",
	"moonshotai/kimi", "z-ai/glm", "deepseek/deepseek", "qwen/qwen",
	"x-ai/grok", "meta-llama/llama", "mistralai/",
}

// openRouter builds the catalog for OpenRouter. Their /models endpoint lists
// several hundred entries from every vendor, so the order matters more than
// anywhere else: the families people actually code with go on top, the rest
// stays reachable below (both pickers filter by substring).
func openRouter(live []subscriptions.ModelMeta) []llm.Model {
	rank := func(id string) int {
		for i, p := range openRouterTop {
			if strings.HasPrefix(id, p) {
				return i
			}
		}
		return len(openRouterTop)
	}
	ms := make([]subscriptions.ModelMeta, 0, len(live))
	for _, m := range live {
		// ":batch" variants exist for OpenRouter's asynchronous batch API and
		// cannot serve a chat turn — offering them would only produce errors.
		if strings.HasSuffix(m.ID, ":batch") {
			continue
		}
		ms = append(ms, m)
	}
	sort.SliceStable(ms, func(i, j int) bool {
		ri, rj := rank(ms[i].ID), rank(ms[j].ID)
		if ri != rj {
			return ri < rj
		}
		return ms[i].ID < ms[j].ID
	})
	out := make([]llm.Model, 0, len(ms))
	for i, m := range ms {
		tier := "standard"
		if rank(m.ID) < len(openRouterTop) {
			tier = "flagship"
		}
		name := m.Name
		if name == "" {
			name = prettyName(m.ID)
		}
		out = append(out, llm.Model{
			ID: m.ID, Provider: subscriptions.SourceOpenRouter, Name: name,
			Tier: tier, HasTools: true, IsPrimary: i == 0,
		})
	}
	return out
}

// prettyName turns a model ID into something readable when the provider sent
// no name: "anthropic/claude-sonnet-4.5" → "Claude Sonnet 4.5 · anthropic",
// "kimi-for-coding-highspeed" → "Kimi For Coding Highspeed".
func prettyName(id string) string {
	vendor, name := "", id
	if i := strings.Index(id, "/"); i > 0 {
		vendor, name = id[:i], id[i+1:]
	}
	words := strings.FieldsFunc(name, func(r rune) bool { return r == '-' || r == '_' || r == '.' })
	for i, w := range words {
		if w == "" {
			continue
		}
		// Keep version-ish and short tokens as they are: "4.5", "k3", "gpt".
		if w[0] >= '0' && w[0] <= '9' {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	// Re-join with the separators the ID used for versions ("4.5" must not
	// become "4 5"), so rebuild from the original with capitalisation applied.
	out := name
	for _, w := range words {
		lower := strings.ToLower(w)
		if lower == w {
			continue
		}
		out = strings.Replace(out, lower, w, 1)
	}
	out = strings.ReplaceAll(out, "-", " ")
	out = strings.ReplaceAll(out, "_", " ")
	if vendor != "" {
		return out + " · " + vendor
	}
	return out
}
