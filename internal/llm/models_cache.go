package llm

// Offline-friendly bootstrap of the model catalog.
//
// FetchModelsCached is what the TUI calls at startup:
//   1. Tries to reach /billing-vbai/models_public (5-sec timeout).
//   2. On success — saves to ~/.config/execai/models_cache.json and returns it.
//   3. On error (network, DNS, timeout, 5xx) — reads the cache and returns it
//      (even if stale). The user sees a warning "running on cached
//      models, the catalog may be outdated".
//   4. If there is no cache either — returns the embedded fallback minimum (1 model)
//      so the TUI at least opens. Real requests may fail later,
//      but the chat starts and the user sees a comprehensible screen, not an error.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/config"
)

// FetchModelsResult — what we actually managed to obtain + how offline it is.
type FetchModelsResult struct {
	Models    []Model
	FromCache bool      // true = network failed, took the cache
	CacheAge  time.Duration // how old the cache is
	Fallback  bool      // true = no cache either, returned the embedded minimum
	Err       error     // non-nil if the network fetch failed; the agent may display it as a warning
}

// FetchModelsCached — cache-first bootstrap. Never returns an empty
// model list (see embeddedFallback).
func FetchModelsCached(apiBase, token string) FetchModelsResult {
	// Try the network with a short timeout — so we do not block startup.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	models, err := FetchModels(ctx, apiBase, token)
	if err == nil && len(models) > 0 {
		saveCache(models)
		return FetchModelsResult{Models: models}
	}

	// The network let us down — read the cache.
	cached, age, cacheErr := loadCache()
	if cacheErr == nil && len(cached) > 0 {
		return FetchModelsResult{
			Models:    cached,
			FromCache: true,
			CacheAge:  age,
			Err:       err,
		}
	}

	// No cache either — the minimum.
	return FetchModelsResult{
		Models:   embeddedFallback(),
		Fallback: true,
		Err:      firstNonNil(err, cacheErr, errors.New("нет ни сети ни кеша моделей")),
	}
}

func cachePath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "models_cache.json"), nil
}

type cacheFile struct {
	SavedAt time.Time `json:"saved_at"`
	Models  []Model   `json:"models"`
}

func saveCache(models []Model) {
	path, err := cachePath()
	if err != nil {
		return
	}
	data, err := json.MarshalIndent(cacheFile{SavedAt: time.Now().UTC(), Models: models}, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o600)
}

func loadCache() ([]Model, time.Duration, error) {
	path, err := cachePath()
	if err != nil {
		return nil, 0, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, err
	}
	var f cacheFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, 0, err
	}
	if len(f.Models) == 0 {
		return nil, 0, errors.New("empty cache")
	}
	age := time.Since(f.SavedAt)
	return f.Models, age, nil
}

// embeddedFallback — the minimum for the TUI to open when there is neither network nor cache.
// One model for the flagship Anthropic (usually available via the ExecAI gateway).
// Real requests may fail — the user will see it in the chat, but the interface works.
func embeddedFallback() []Model {
	return []Model{
		{
			ID:          "claude-sonnet-4-6",
			Provider:    "anthropic",
			Name:        "Claude Sonnet 4.6 (offline fallback)",
			Description: "Заглушка когда каталог моделей недоступен — реальные запросы могут не пройти",
			Tier:        "flagship",
			IsPrimary:   true,
			HasTools:    true,
		},
	}
}

func firstNonNil(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

func (r FetchModelsResult) OfflineNotice() string {
	if r.Fallback {
		return fmt.Sprintf("⚠ Работаю оффлайн — каталог моделей недоступен, использую fallback. Причина: %v", r.Err)
	}
	if r.FromCache {
		age := r.CacheAge.Round(time.Minute).String()
		return fmt.Sprintf("ℹ Использую кешированный каталог (возраст: %s). Сеть недоступна: %v", age, r.Err)
	}
	return ""
}
