// Package catalog builds the model list for a source.
//
// Why it exists. The same question — "which models does this source have?" —
// used to be answered twice: in the TUI (internal/chat) with human-readable
// names, and in the editor protocol (internal/ide) by dumping the raw IDs the
// provider returned. The panel showed things like `kimi-for-coding-highspeed`
// with no name, in server order, mixed with whatever else the endpoint listed.
// One source of truth removes that class of bug for good: every surface —
// TUI, `execai ide`, `execai serve` — calls For().
//
// The rule for every source: the live list from the provider (fetched during
// /connect and stored in the subscription) wins, the built-in catalog is the
// fallback for when the endpoint is unreachable or the connection predates the
// fetch. Names and descriptions always come from the built-in catalog when the
// ID is known — a live list gives IDs, not prose.
package catalog

import (
	"sort"
	"strings"

	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

// For returns the model catalog of a source. available is the live list of IDs
// from the subscription (may be empty).
func For(provider string, available []string) []llm.Model {
	switch provider {
	case subscriptions.SourceZAI:
		return llm.GLMModels()
	case subscriptions.SourceZAIAPI:
		return dynamicOr(available, llm.ZAIAPIModels(), "zai-api", zaiPrimary)
	case subscriptions.SourceKimi:
		// Coding Plan: the built-in catalog is authoritative for names and
		// order (the docs list exactly three IDs the endpoint accepts), the
		// live list only adds what the plan gained since this build.
		return merge(llm.KimiModels(), available, "kimi")
	case subscriptions.SourceKimiAPI:
		return dynamicOr(available, llm.MoonshotModels(), "kimi-api", kimiAPIPrimary)
	case subscriptions.SourceOpenAI:
		return dynamicOr(available, llm.OpenAIModels(), "openai", openAIPrimary)
	case subscriptions.SourceOpenRouter:
		return openRouter(available)
	case subscriptions.SourceAnthropic:
		// The built-in list gives names and descriptions; anything the API
		// reports beyond it — a model released after this build — is appended.
		return merge(llm.AnthropicModels(), available, "anthropic")
	case subscriptions.SourceOllama:
		// What the runner actually has (or the cloud account can run). The
		// built-in cloud list is only the fallback for an unreachable runner.
		return merge(llm.OllamaCloudModels(), available, "ollama")
	case subscriptions.SourceCodexCLI:
		return llm.CodexCLIModels()
	case subscriptions.SourceClaudeCLI:
		return llm.ClaudeCLIModels()
	}
	return nil
}

// merge keeps the built-in catalog as the backbone and appends live IDs that it
// does not know yet. Nothing the provider reports is hidden, and nothing the
// provider forgot to report disappears: a Coding Plan whose /models endpoint is
// terse must still offer the models the plan actually has.
func merge(builtin []llm.Model, available []string, provider string) []llm.Model {
	if len(available) == 0 {
		return builtin
	}
	known := map[string]bool{}
	for _, m := range builtin {
		known[m.ID] = true
	}
	out := append([]llm.Model(nil), builtin...)
	for _, id := range available {
		if known[id] {
			continue
		}
		known[id] = true
		out = append(out, llm.Model{
			ID: id, Provider: provider, Name: prettyName(id),
			Tier: "standard", HasTools: true,
		})
	}
	return out
}

// dynamicOr builds a catalog from the live list, falling back to the built-in
// one. Names of known IDs are taken from the built-in catalog.
func dynamicOr(available []string, builtin []llm.Model, provider string, primaryOrder []string) []llm.Model {
	if len(available) == 0 {
		return builtin
	}
	names := map[string]llm.Model{}
	for _, m := range builtin {
		names[m.ID] = m
	}
	have := map[string]bool{}
	for _, id := range available {
		have[id] = true
	}
	primaryID := ""
	for _, want := range primaryOrder {
		if have[want] {
			primaryID = want
			break
		}
	}
	out := make([]llm.Model, 0, len(available))
	for _, id := range available {
		mod, ok := names[id]
		if !ok {
			mod = llm.Model{ID: id, Name: prettyName(id), Tier: "standard", HasTools: true}
		}
		mod.Provider = provider
		mod.IsPrimary = id == primaryID
		out = append(out, mod)
	}
	if primaryID == "" && len(out) > 0 {
		out[0].IsPrimary = true
	}
	return out
}

var (
	zaiPrimary     = []string{"glm-5.2", "glm-4.7", "glm-4.6", "glm-4.5"}
	kimiAPIPrimary = []string{"kimi-k3", "kimi-k3-preview", "kimi-k3-turbo", "k3", "kimi-latest", "kimi-k2-turbo-preview", "moonshot-v1-auto", "moonshot-v1-128k"}
	openAIPrimary  = []string{"gpt-5", "gpt-5-mini", "o3", "gpt-4.1", "gpt-4o", "o4-mini", "gpt-4o-mini"}
	// openRouterTop — vendors and families worth putting on top of a catalog
	// that is several hundred models long. Prefix match on the OpenRouter ID
	// ("anthropic/claude-sonnet-4.5"), best first.
	openRouterTop = []string{
		"anthropic/claude-opus", "anthropic/claude-sonnet", "anthropic/claude-haiku",
		"openai/gpt-5", "openai/o3", "openai/gpt-4.1",
		"google/gemini-2.5-pro", "google/gemini",
		"moonshotai/kimi", "z-ai/glm", "deepseek/deepseek", "qwen/qwen",
		"x-ai/grok", "meta-llama/llama", "mistralai/",
	}
)

// openRouter builds the catalog for OpenRouter. Their /models endpoint lists
// several hundred entries from every vendor, so the order matters more than
// anywhere else: the families people actually code with go on top, the rest
// stays reachable below (both pickers filter by substring).
func openRouter(available []string) []llm.Model {
	if len(available) == 0 {
		return llm.OpenRouterModels()
	}
	rank := func(id string) int {
		for i, p := range openRouterTop {
			if strings.HasPrefix(id, p) {
				return i
			}
		}
		return len(openRouterTop)
	}
	ids := make([]string, 0, len(available))
	for _, id := range available {
		// ":batch" variants exist for OpenRouter's asynchronous batch API and
		// cannot serve a chat turn — offering them would only produce errors.
		if strings.HasSuffix(id, ":batch") {
			continue
		}
		ids = append(ids, id)
	}
	sort.SliceStable(ids, func(i, j int) bool {
		ri, rj := rank(ids[i]), rank(ids[j])
		if ri != rj {
			return ri < rj
		}
		return ids[i] < ids[j]
	})
	out := make([]llm.Model, 0, len(ids))
	for i, id := range ids {
		tier := "standard"
		if rank(id) < len(openRouterTop) {
			tier = "flagship"
		}
		out = append(out, llm.Model{
			ID: id, Provider: "openrouter", Name: prettyName(id),
			Tier: tier, HasTools: true, IsPrimary: i == 0,
		})
	}
	return out
}

// prettyName turns a model ID into something readable when no catalog entry
// carries a name: "anthropic/claude-sonnet-4.5" → "Claude Sonnet 4.5 · anthropic",
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
