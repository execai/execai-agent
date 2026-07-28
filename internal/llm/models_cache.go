package llm

// Offline-friendly bootstrap каталога моделей.
//
// FetchModelsCached — то что вызывает TUI при старте:
//   1. Пытается сходить в /billing-vbai/models_public (5-сек timeout).
//   2. При успехе — сохраняет в ~/.config/execai/models_cache.json и возвращает.
//   3. При ошибке (сеть, DNS, timeout, 5xx) — читает кеш и возвращает его
//      (пусть даже старому). Юзер видит warning "работаешь на кешированных
//      моделях, каталог может быть устаревшим".
//   4. Если и кеша нет — возвращает встроенный fallback-минимум (1 модель),
//      чтобы TUI хотя бы открылся. Реальные запросы могут потом упасть,
//      но чат стартует и юзер увидит понятный экран, а не error.

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

// FetchModelsResult — что реально удалось получить + метка сколько это оффлайн.
type FetchModelsResult struct {
	Models    []Model
	FromCache bool      // true = сеть не пропустила, взяли кеш
	CacheAge  time.Duration // сколько кешу лет
	Fallback  bool      // true = даже кеша не было, отдали встроенный минимум
	Err       error     // непустая если из сети не удалось; агент может отобразить как warning
}

// FetchModelsCached — cache-first bootstrap. Никогда не возвращает пустой
// список моделей (см. embeddedFallback).
func FetchModelsCached(apiBase, token string) FetchModelsResult {
	// Пытаемся сеть с коротким timeout — чтобы не блокировать старт.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	models, err := FetchModels(ctx, apiBase, token)
	if err == nil && len(models) > 0 {
		saveCache(models)
		return FetchModelsResult{Models: models}
	}

	// Сеть подвела — читаем кеш.
	cached, age, cacheErr := loadCache()
	if cacheErr == nil && len(cached) > 0 {
		return FetchModelsResult{
			Models:    cached,
			FromCache: true,
			CacheAge:  age,
			Err:       err,
		}
	}

	// Даже кеша нет — минимум.
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

// embeddedFallback — минимум чтобы TUI открылся когда ни сети ни кеша нет.
// Одна модель под флагманский Anthropic (обычно доступный через ExecAI-gateway).
// Реальные запросы могут упасть — юзер увидит в чате, но интерфейс работает.
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
