// Package connect prepares a subscription: default endpoint, key check and
// the live model list.
//
// Why it exists. `/connect` in the terminal did all of that; the same action
// in the editor panel stored the key and nothing else — no endpoint, no model
// list. The result looked connected and half-worked: the catalog fell back to
// the built-in short list, so OpenRouter offered eight models instead of the
// four hundred the account actually has. One place now answers "what does it
// take to connect this provider", and both surfaces ask it.
package connect

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
	"github.com/velesbsdllc/agent-vbai/internal/version"
)

// DefaultBaseURL — the endpoint a provider is reached at when the user did not
// name one. An empty result means the provider has no fixed endpoint (local
// CLIs, Ollama) or is not keyed at all.
func DefaultBaseURL(provider string) string {
	switch provider {
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
	if baseURL == "" || (apiKey == "" && provider != subscriptions.SourceOllama) {
		return sub, 0
	}
	ids, status := FetchModelsFor(provider, baseURL, apiKey)
	if len(ids) > 0 {
		sub.AvailableModels = ids
		sub.ModelsFetchedAt = time.Now()
	}
	return sub, status
}

// KeyRejected reports whether a status from FetchModels means the key does not
// belong to this endpoint. 402/429 count too: a Coding Plan key on a
// pay-per-token endpoint shows up as "no balance", which is the same mistake.
func KeyRejected(status int) bool {
	return status == 401 || status == 403 || status == 402 || status == 429
}

// FetchModelsFor asks a provider for its model list the way that provider
// expects to be asked. Anthropic wants x-api-key and a version header, Ollama
// answers on /api/tags with its own shape, everyone else speaks the
// OpenAI-compatible /v1/models. Without this, "does a new model appear by
// itself" had three different answers depending on the source.
func FetchModelsFor(provider, baseURL, apiKey string) ([]string, int) {
	switch provider {
	case subscriptions.SourceAnthropic:
		return fetchAnthropic(baseURL, apiKey)
	case subscriptions.SourceOllama:
		return fetchOllama(baseURL, apiKey)
	}
	return FetchModels(baseURL, apiKey)
}

// fetchAnthropic — GET /v1/models. Same response shape as OpenAI, different
// authentication: x-api-key plus a dated API version.
func fetchAnthropic(baseURL, apiKey string) ([]string, int) {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	url := strings.TrimSuffix(baseURL, "/")
	if !strings.HasSuffix(url, "/v1") {
		url += "/v1"
	}
	req, err := http.NewRequest("GET", url+"/models?limit=100", nil)
	if err != nil {
		return nil, 0
	}
	req.Header.Set("User-Agent", version.UserAgent())
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	return decodeIDs(req)
}

// fetchOllama — GET /api/tags: what the runner actually has pulled (local) or
// what the account can run (cloud). The key is only needed for the cloud.
func fetchOllama(baseURL, apiKey string) ([]string, int) {
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
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return nil, 0
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, resp.StatusCode
	}
	var out struct {
		Models []struct {
			Model string `json:"model"`
			Name  string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, resp.StatusCode
	}
	ids := make([]string, 0, len(out.Models))
	for _, m := range out.Models {
		id := m.Model
		if id == "" {
			id = m.Name
		}
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids, 200
}

// FetchModels asks an OpenAI-compatible endpoint for its model list. Returns
// the IDs and the HTTP status, so callers can tell "endpoint silent" from
// "key refused".
func FetchModels(baseURL, apiKey string) ([]string, int) {
	if baseURL == "" {
		return nil, 0
	}
	url := strings.TrimSuffix(baseURL, "/")
	if !strings.HasSuffix(url, "/v1") {
		url += "/v1"
	}
	url += "/models"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, 0
	}
	req.Header.Set("User-Agent", version.UserAgent())
	req.Header.Set("Authorization", "Bearer "+apiKey)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, resp.StatusCode
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, resp.StatusCode
	}
	ids := make([]string, 0, len(out.Data))
	for _, m := range out.Data {
		if m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	return ids, 200
}

// ModelsTTL — how long a fetched model list is trusted. A day is long enough
// that nobody notices the request and short enough that a plan upgrade or a
// vendor's new release shows up the same day.
const ModelsTTL = 24 * time.Hour

// WantsLiveModels — sources whose catalog is worth re-fetching: their model
// list changes with the plan or with what the vendor ships.
func WantsLiveModels(provider string) bool {
	switch provider {
	case subscriptions.SourceKimi, subscriptions.SourceKimiAPI, subscriptions.SourceZAIAPI,
		subscriptions.SourceOpenAI, subscriptions.SourceOpenRouter,
		subscriptions.SourceAnthropic, subscriptions.SourceOllama:
		return true
	}
	return false
}

// Stale reports whether the stored model list should be fetched again.
func Stale(sub subscriptions.Subscription) bool {
	if len(sub.AvailableModels) == 0 || sub.ModelsFetchedAt.IsZero() {
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
	if !ok || sub.APIKey == "" {
		return false
	}
	base := sub.BaseURL
	if base == "" {
		base = DefaultBaseURL(provider)
	}
	ids, _ := FetchModelsFor(provider, base, sub.APIKey)
	if len(ids) == 0 {
		return false
	}
	sub.AvailableModels = ids
	sub.ModelsFetchedAt = time.Now()
	if sub.BaseURL == "" {
		sub.BaseURL = base
	}
	st.Subscriptions[provider] = sub
	return st.Save() == nil
}

// decodeIDs performs a prepared request and pulls data[].id out of the answer.
func decodeIDs(req *http.Request) ([]string, int) {
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return nil, 0
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, resp.StatusCode
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, resp.StatusCode
	}
	ids := make([]string, 0, len(out.Data))
	for _, m := range out.Data {
		if m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	return ids, 200
}
