// Package connect prepares a subscription: default endpoint, key check and
// the live model list.
//
// Why it exists. `/connect` in the terminal did all of that; the same action
// in the editor panel stored the key and nothing else — no endpoint, no model
// list. The result looked connected and half-worked: the catalog fell back to
// the built-in short list, so OpenRouter offered eight models instead of the
// four hundred the account actually has. One place now answers "what does it
// take to connect this provider", and both surfaces ask it.
//
// The model list of every source comes from the source itself: an HTTP
// /models endpoint, Ollama's /api/tags, or the delegated CLI's own protocol.
// There is no built-in catalog to fall back to — those went stale within
// weeks every time (Codex offered gpt-5 when the account had gpt-5.6, Kimi
// called K2.8 "K2.7").
package connect

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
	"github.com/velesbsdllc/agent-vbai/internal/version"
)

// ModelMeta is one model of a live list.
type ModelMeta = subscriptions.ModelMeta

// DefaultBaseURL — the endpoint a provider is reached at when the user did not
// name one. An empty result means the provider has no fixed endpoint (local
// CLIs, custom endpoints) or is not keyed at all.
func DefaultBaseURL(provider string) string {
	switch provider {
	case subscriptions.SourceZAI:
		return "https://api.z.ai/api/anthropic"
	case subscriptions.SourceZAIAPI:
		return "https://api.z.ai/api/paas/v4"
	case subscriptions.SourceKimi:
		return "https://api.kimi.com/coding"
	case subscriptions.SourceKimiAPI:
		return "https://api.moonshot.ai/v1"
	case subscriptions.SourceOpenAI:
		return "https://api.openai.com/v1"
	case subscriptions.SourceOpenRouter:
		return "https://openrouter.ai/api/v1"
	case subscriptions.SourceAnthropic:
		return "https://api.anthropic.com"
	case subscriptions.SourceOllama:
		return "http://localhost:11434"
	}
	return ""
}

// DefaultPlan — "coding" for subscription plans, "api" for pay-per-token.
// Only a label: it shows in the status bar and in /subscriptions.
func DefaultPlan(provider string) string {
	switch provider {
	case subscriptions.SourceAnthropic, subscriptions.SourceKimiAPI,
		subscriptions.SourceZAIAPI, subscriptions.SourceOpenAI, subscriptions.SourceOpenRouter:
		return "api"
	}
	return "coding"
}

// RejectsBadKey — providers whose /models endpoint answers honestly enough to
// refuse a wrong key right away. For the others a bad key surfaces on the
// first request, and claiming "connected" would be a lie either way.
func RejectsBadKey(provider string) bool {
	switch provider {
	case subscriptions.SourceKimiAPI, subscriptions.SourceZAIAPI,
		subscriptions.SourceOpenAI, subscriptions.SourceOpenRouter:
		return true
	}
	return false
}

// Prepare fills in what a subscription needs: endpoint, plan and the live
// model list. status is the HTTP status of the model request (0 = no answer);
// the caller decides what a 401/402/403/429 means for its own wording.
func Prepare(provider, apiKey, baseURL string) (subscriptions.Subscription, int) {
	if baseURL == "" {
		baseURL = DefaultBaseURL(provider)
	}
	sub := subscriptions.Subscription{
		Provider: provider,
		APIKey:   apiKey,
		BaseURL:  baseURL,
		Plan:     DefaultPlan(provider),
	}
	if baseURL == "" || (apiKey == "" && NeedsKey(sub)) {
		return sub, 0
	}
	return sub, Fill(&sub)
}

// Custom describes a user-defined endpoint.
type Custom struct {
	Name    string            // what the person calls it: "LM Studio"
	API     string            // subscriptions.APIOpenAI | subscriptions.APIAnthropic
	BaseURL string            // as typed; normalized by PrepareCustom
	APIKey  string            // optional: local servers often run without one
	Headers map[string]string // optional extra headers
	Models  []string          // optional model IDs for endpoints without /models
}

// PrepareCustom validates a custom endpoint and fetches its model list.
// status is the HTTP status of the model request (0 = no answer). An error
// means the description itself is wrong (name, protocol, address).
func PrepareCustom(c Custom) (subscriptions.Subscription, int, error) {
	id := subscriptions.CustomID(c.Name)
	if err := subscriptions.ValidateCustomID(id); err != nil {
		return subscriptions.Subscription{}, 0, err
	}
	api, err := ParseAPI(c.API)
	if err != nil {
		return subscriptions.Subscription{}, 0, err
	}
	base, err := NormalizeCustomBase(api, c.BaseURL)
	if err != nil {
		return subscriptions.Subscription{}, 0, err
	}
	sub := subscriptions.Subscription{
		Provider:     id,
		Name:         strings.TrimSpace(c.Name),
		API:          api,
		BaseURL:      base,
		APIKey:       strings.TrimSpace(c.APIKey),
		Headers:      c.Headers,
		ManualModels: cleanIDs(c.Models),
		Plan:         api,
	}
	// An OpenAI-style address without a version segment is ambiguous:
	// "https://openrouter.ai/api" serves under /api/v1, a Perplexity-like
	// server right under the root. The model list and the chat must use the
	// same base, so the one that answers /models wins and is stored.
	status := 0
	for _, cand := range openAICandidates(api, base) {
		try := sub
		try.BaseURL = cand
		if st := Fill(&try); st == 200 {
			return try, st, nil
		} else if status == 0 || KeyRejected(st) {
			status = st
		}
	}
	// Nobody answered /models. A bare host almost always means "/v1" (LM
	// Studio, vLLM, llama.cpp, Ollama); a longer path is kept as typed.
	if u, err := url.Parse(base); err == nil && u.Path == "" && api == subscriptions.APIOpenAI {
		sub.BaseURL = base + "/v1"
	}
	return sub, status, nil
}

// openAICandidates lists the bases worth asking, most likely first. A
// versioned base ("/v1", Z.ai's "/v4") is unambiguous; otherwise "/v1" is
// tried before the address as typed.
func openAICandidates(api, base string) []string {
	if api != subscriptions.APIOpenAI || versionSuffix.MatchString(base) {
		return []string{base}
	}
	return []string{base + "/v1", base}
}

// ParseAPI accepts the protocol name in the forms people type.
func ParseAPI(s string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "openai", "openai-compatible", "openai-compat", "oai", "chat", "chat/completions":
		return subscriptions.APIOpenAI, nil
	case "2", "anthropic", "anthropic-compatible", "anthropic-compat", "claude", "messages":
		return subscriptions.APIAnthropic, nil
	}
	return "", fmt.Errorf("тип интерфейса %q не поддерживается: openai (chat/completions) или anthropic (messages)", s)
}

var versionSuffix = regexp.MustCompile(`/v\d+(beta|alpha)?\d*$`)

// NormalizeCustomBase turns whatever address was typed into the base the
// client appends its path to. OpenAI-style clients post to {base}/chat/completions,
// so the base keeps its version segment; whether a bare address needs "/v1"
// is settled by asking the server (PrepareCustom). Anthropic-style clients
// post to {base}/v1/messages, so the version segment is cut off. Full
// endpoint URLs pasted from documentation are trimmed back.
func NormalizeCustomBase(api, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("нужен адрес сервера, например http://localhost:1234/v1")
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("адрес %q не похож на http(s)-URL", raw)
	}
	u.RawQuery, u.Fragment = "", ""
	p := strings.TrimRight(u.Path, "/")
	for _, tail := range []string{"/chat/completions", "/completions", "/messages", "/models"} {
		p = strings.TrimSuffix(p, tail)
	}
	if api == subscriptions.APIAnthropic {
		p = versionSuffix.ReplaceAllString(p, "")
	}
	u.Path = p
	return strings.TrimRight(u.String(), "/"), nil
}

// NeedsKey reports whether a subscription cannot work without an API key.
// The CLIs bring their own login, a local Ollama and custom endpoints may
// run open.
func NeedsKey(sub subscriptions.Subscription) bool {
	if sub.IsCustom() {
		return false
	}
	switch sub.Provider {
	case subscriptions.SourceClaudeCLI, subscriptions.SourceCodexCLI, subscriptions.SourceOllama:
		return false
	}
	return true
}

// KeyRejected reports whether a status from FetchModels means the key does not
// belong to this endpoint. 402/429 count too: a Coding Plan key on a
// pay-per-token endpoint shows up as "no balance", which is the same mistake.
func KeyRejected(status int) bool {
	return status == 401 || status == 403 || status == 402 || status == 429
}

// Fetch asks a source for its model list the way that source expects to be
// asked. It is a variable so tests can answer without the network.
var Fetch = fetch

// Fill fetches the live list into sub. Returns the status of the request
// (200 = list stored; 0 = no answer).
func Fill(sub *subscriptions.Subscription) int {
	ms, status := Fetch(*sub)
	if len(ms) > 0 {
		sub.SetModels(ms, time.Now())
	}
	return status
}

func fetch(sub subscriptions.Subscription) ([]ModelMeta, int) {
	base := sub.BaseURL
	if base == "" {
		base = DefaultBaseURL(sub.Provider)
	}
	if sub.IsCustom() {
		if sub.API == subscriptions.APIAnthropic {
			return fetchAnthropic(base, sub.APIKey, sub.Headers)
		}
		// Exactly {base}/models — the chat goes to {base}/chat/completions,
		// and the two must never disagree about the base.
		return fetchOpenAIAt(strings.TrimSuffix(base, "/")+"/models", sub.APIKey, sub.Headers)
	}
	switch sub.Provider {
	case subscriptions.SourceAnthropic, subscriptions.SourceZAI:
		// The Z.ai Coding Plan lives on an Anthropic-compatible base: the
		// same GET /v1/models with the same x-api-key (checked on api.z.ai).
		return fetchAnthropic(base, sub.APIKey, nil)
	case subscriptions.SourceOllama:
		return fetchOllama(base, sub.APIKey)
	case subscriptions.SourceClaudeCLI:
		return fromCLI(llm.ListClaudeCLIModels(context.Background()))
	case subscriptions.SourceCodexCLI:
		return fromCLI(llm.ListCodexCLIModels(context.Background()))
	}
	return fetchOpenAI(base, sub.APIKey, nil)
}

// FetchModelsFor is the ID-only form of Fetch for a built-in provider.
func FetchModelsFor(provider, baseURL, apiKey string) ([]string, int) {
	ms, status := Fetch(subscriptions.Subscription{Provider: provider, BaseURL: baseURL, APIKey: apiKey})
	return ids(ms), status
}

// FetchModels asks an OpenAI-compatible endpoint for its model IDs.
func FetchModels(baseURL, apiKey string) ([]string, int) {
	ms, status := fetchOpenAI(baseURL, apiKey, nil)
	return ids(ms), status
}

func fromCLI(ms []llm.CLIModel, err error) ([]ModelMeta, int) {
	if err != nil || len(ms) == 0 {
		return nil, 0
	}
	out := make([]ModelMeta, 0, len(ms))
	for _, m := range ms {
		out = append(out, ModelMeta{ID: m.ID, Name: m.Name, Description: m.Description, Default: m.Default})
	}
	return out, 200
}

// fetchAnthropic — GET /v1/models: x-api-key plus a dated API version, paged
// with has_more/last_id. Bearer goes along for gateways that expect it
// (LiteLLM, Z.ai), exactly as the chat client sends it.
func fetchAnthropic(baseURL string, apiKey string, headers map[string]string) ([]ModelMeta, int) {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	root := versionSuffix.ReplaceAllString(strings.TrimSuffix(baseURL, "/"), "")
	var out []ModelMeta
	after := ""
	for page := 0; page < 20; page++ {
		u := root + "/v1/models?limit=1000"
		if after != "" {
			u += "&after_id=" + url.QueryEscape(after)
		}
		req, err := http.NewRequest("GET", u, nil)
		if err != nil {
			return nil, 0
		}
		req.Header.Set("User-Agent", version.UserAgent())
		if apiKey != "" {
			req.Header.Set("x-api-key", apiKey)
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
		req.Header.Set("anthropic-version", "2023-06-01")
		setHeaders(req, headers)
		var body struct {
			Data    []rawModel `json:"data"`
			HasMore bool       `json:"has_more"`
			LastID  string     `json:"last_id"`
		}
		if status := doJSON(req, &body); status != 200 {
			if len(out) > 0 {
				return out, 200
			}
			return nil, status
		}
		out = append(out, metas(body.Data)...)
		if !body.HasMore || body.LastID == "" || body.LastID == after {
			break
		}
		after = body.LastID
	}
	return out, 200
}

// fetchOpenAI — GET {base}/models with a Bearer key. A base that already ends
// in a version segment (/v1, Z.ai's /v4) is used as is; a bare one gets /v1.
func fetchOpenAI(baseURL, apiKey string, headers map[string]string) ([]ModelMeta, int) {
	if baseURL == "" {
		return nil, 0
	}
	u := strings.TrimSuffix(baseURL, "/")
	if !versionSuffix.MatchString(u) {
		u += "/v1"
	}
	return fetchOpenAIAt(u+"/models", apiKey, headers)
}

// fetchOpenAIAt asks one exact /models URL.
func fetchOpenAIAt(modelsURL, apiKey string, headers map[string]string) ([]ModelMeta, int) {
	req, err := http.NewRequest("GET", modelsURL, nil)
	if err != nil {
		return nil, 0
	}
	req.Header.Set("User-Agent", version.UserAgent())
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	setHeaders(req, headers)
	var body struct {
		Data []rawModel `json:"data"`
	}
	if status := doJSON(req, &body); status != 200 {
		return nil, status
	}
	return metas(body.Data), 200
}

// fetchOllama — GET /api/tags: what the runner actually has pulled (local) or
// what the account can run (cloud). The key is only needed for the cloud.
func fetchOllama(baseURL, apiKey string) ([]ModelMeta, int) {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	req, err := http.NewRequest("GET", strings.TrimSuffix(baseURL, "/")+"/api/tags", nil)
	if err != nil {
		return nil, 0
	}
	req.Header.Set("User-Agent", version.UserAgent())
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	var body struct {
		Models []struct {
			Model string `json:"model"`
			Name  string `json:"name"`
		} `json:"models"`
	}
	if status := doJSON(req, &body); status != 200 {
		return nil, status
	}
	out := make([]ModelMeta, 0, len(body.Models))
	for _, m := range body.Models {
		id := m.Model
		if id == "" {
			id = m.Name
		}
		if id != "" {
			out = append(out, ModelMeta{ID: id})
		}
	}
	return out, 200
}

// rawModel covers the shapes seen in the wild: OpenAI {id}, Anthropic and
// Kimi {id, display_name}, OpenRouter and Z.ai {id, name}; a creation time as
// Unix seconds (created) or RFC 3339 (created_at).
type rawModel struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Name        string `json:"name"`
	Created     int64  `json:"created"`
	CreatedAt   string `json:"created_at"`
}

func (m rawModel) createdUnix() int64 {
	if m.Created > 0 {
		return m.Created
	}
	if t, err := time.Parse(time.RFC3339, m.CreatedAt); err == nil {
		return t.Unix()
	}
	return 0
}

// metas turns a /models answer into the stored list, newest first when the
// provider dates its models (Z.ai lists oldest first: glm-4.5 on top, the
// current glm-5.3 at the bottom of the picker).
func metas(raw []rawModel) []ModelMeta {
	dated := len(raw) > 1
	for _, m := range raw {
		if m.createdUnix() == 0 {
			dated = false
			break
		}
	}
	if dated {
		raw = append([]rawModel(nil), raw...)
		sort.SliceStable(raw, func(i, j int) bool { return raw[i].createdUnix() > raw[j].createdUnix() })
	}
	out := make([]ModelMeta, 0, len(raw))
	seen := map[string]bool{}
	for _, m := range raw {
		id := strings.TrimSpace(m.ID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		name := strings.TrimSpace(m.DisplayName)
		if name == "" {
			name = strings.TrimSpace(m.Name)
		}
		if name == id {
			name = ""
		}
		out = append(out, ModelMeta{ID: id, Name: name})
	}
	return out
}

func doJSON(req *http.Request, into any) int {
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return resp.StatusCode
	}
	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		// 200 with something that is not a model list (an HTML login page of
		// a proxy) — the endpoint answered, but there is nothing to show.
		return 0
	}
	return 200
}

func setHeaders(req *http.Request, headers map[string]string) {
	for k, v := range headers {
		req.Header.Set(k, v)
	}
}

func ids(ms []ModelMeta) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.ID)
	}
	return out
}

func cleanIDs(in []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range in {
		for _, id := range strings.Split(s, ",") {
			id = strings.TrimSpace(id)
			if id != "" && !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	return out
}

// ModelsTTL — how long a fetched model list is trusted. A day is long enough
// that nobody notices the request and short enough that a plan upgrade or a
// vendor's new release shows up the same day.
const ModelsTTL = 24 * time.Hour

// WantsLiveModels — every source except our own gateway (its catalog comes
// from billing) keeps a live list that is worth re-fetching.
func WantsLiveModels(provider string) bool {
	return provider != "" && provider != subscriptions.SourceExecAI
}

// Stale reports whether the stored model list should be fetched again.
func Stale(sub subscriptions.Subscription) bool {
	if len(sub.AvailableModels) == 0 || sub.ModelsFetchedAt.IsZero() {
		return true
	}
	// A list stored by a build that kept only IDs: fetch once more to get
	// the names the provider sends along.
	if len(sub.Models) == 0 {
		return true
	}
	return time.Since(sub.ModelsFetchedAt) > ModelsTTL
}

// RefreshModels re-reads the store from disk, fetches the live list for one
// provider and saves it back. It deliberately works on its own copy of the
// store: callers run it in the background, and the store they hold must not be
// mutated under them. Reports whether anything was written.
func RefreshModels(provider string) bool {
	st, err := subscriptions.Load()
	if err != nil || st == nil {
		return false
	}
	sub, ok := st.Subscriptions[provider]
	if !ok || (sub.APIKey == "" && NeedsKey(sub)) {
		return false
	}
	if sub.BaseURL == "" {
		sub.BaseURL = DefaultBaseURL(provider)
	}
	if Fill(&sub) != 200 || len(sub.Models) == 0 {
		return false
	}
	// The fetch may have taken seconds: re-read so a change made meanwhile
	// (a /disconnect, another key) is not overwritten with the old copy.
	st, err = subscriptions.Load()
	if err != nil || st == nil {
		return false
	}
	cur, ok := st.Subscriptions[provider]
	if !ok || cur.APIKey != sub.APIKey || cur.BaseURL != sub.BaseURL && cur.BaseURL != "" {
		return false
	}
	cur.SetModels(sub.Models, sub.ModelsFetchedAt)
	if cur.BaseURL == "" {
		cur.BaseURL = sub.BaseURL
	}
	st.Subscriptions[provider] = cur
	return st.Save() == nil
}
