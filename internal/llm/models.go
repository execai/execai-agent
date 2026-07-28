package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Model — упрощённая запись из /billing-vbai/models_public.
// Соответствует тому, что мапит фронт в utils/models/ModelsProvider.tsx.
type Model struct {
	ID          string `json:"id"`        // model_name
	Provider    string `json:"provider"`  // openai / anthropic / alibaba / deepinfra / ...
	Name        string `json:"name"`      // display name
	Description string `json:"description"`
	Tier        string `json:"tier"`           // flagship / standard / lite
	IsPrimary   bool   `json:"is_primary"`
	HasTools    bool   `json:"supports_tools"`
}

// rawModel — то, что реально отдаёт billing-vbai. Поля могут идти в snake_case или PascalCase
// (Go-сервис маршалит PascalCase, но в /models_public поля приведены к snake_case).
type rawModel struct {
	ID            string `json:"id"`         // новый catalog R4: id вместо model_name
	ModelName     string `json:"model_name"` // legacy R2/R3 billing
	Provider      string `json:"provider"`
	ProviderName  string `json:"provider_name"`
	DisplayName   string `json:"display_name"` // новый catalog
	NameField     string `json:"name"`         // legacy
	Description   string `json:"description"`
	ModelTier     string `json:"model_tier"`
	IsPrimary     *bool  `json:"is_primary"`
	IsPrimaryAlt  *bool  `json:"isPrimary"`
	ShowInPicker  *bool  `json:"show_in_picker"`
	SupportsTools *bool  `json:"supportsTools"`
	SupportsTools2 *bool `json:"supports_tools"` // snake_case в новом catalog
}

// FetchModels — GET /billing-vbai/models_public, JWT в Authorization Bearer.
func FetchModels(ctx context.Context, apiBase, token string) ([]Model, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+"/billing-vbai/models_public", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	cli := &http.Client{Timeout: 15 * time.Second}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, errors.New("401 от /billing-vbai/models_public — токен истёк, нужно agent-vbai login")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ответ %d: %s", resp.StatusCode, truncate(string(body), 300))
	}

	var raw []rawModel
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("ответ не JSON-массив: %s", truncate(string(body), 300))
	}

	out := make([]Model, 0, len(raw))
	for _, r := range raw {
		// Пропускаем явно скрытые из picker'а (как делает фронт).
		if r.ShowInPicker != nil && !*r.ShowInPicker {
			continue
		}
		id := strings.TrimSpace(r.ID)
		if id == "" {
			id = strings.TrimSpace(r.ModelName)
		}
		if id == "" {
			continue
		}
		prov := strings.TrimSpace(r.Provider)
		if prov == "" {
			prov = strings.TrimSpace(r.ProviderName)
		}
		if prov == "" {
			continue
		}
		name := strings.TrimSpace(r.DisplayName)
		if name == "" {
			name = strings.TrimSpace(r.NameField)
		}
		if name == "" {
			name = id
		}
		isPrimary := false
		if r.IsPrimary != nil {
			isPrimary = *r.IsPrimary
		} else if r.IsPrimaryAlt != nil {
			isPrimary = *r.IsPrimaryAlt
		}
		hasTools := true
		if r.SupportsTools != nil {
			hasTools = *r.SupportsTools
		} else if r.SupportsTools2 != nil {
			hasTools = *r.SupportsTools2
		}
		tier := strings.TrimSpace(r.ModelTier)
		if tier == "" {
			tier = "standard"
		}
		out = append(out, Model{
			ID:          id,
			Provider:    prov,
			Name:        name,
			Description: r.Description,
			Tier:        tier,
			IsPrimary:   isPrimary,
			HasTools:    hasTools,
		})
	}

	// Сортировка: сначала primary (по provider/id), потом остальные (по provider/id) — как у фронта,
	// плюс primary наверху для удобства списка.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].IsPrimary != out[j].IsPrimary {
			return out[i].IsPrimary
		}
		if out[i].Provider != out[j].Provider {
			return out[i].Provider < out[j].Provider
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// PickDefault — выбор модели по умолчанию. Совпадает с логикой web-фронта
// (execaiui-vbai/src/utils/models/ModelsProvider.tsx pickDefaultModel):
//   1) если задан pinnedID и он есть в списке — берём его
//   2) Kimi K2.6 (по id или имени, с обычными опечатками-алиасами)
//   3) is_primary (всё равно K2.6 — IsPrimary=true в models_config)
//   4) первая
//
// 2026-05-11: default переключён с GPT-5.4 на Kimi K2.6. K2.6 — omnimodal
// флагман, дёшево для нас, объективно хорош. Pro+ юзер сам выбирает Opus/Sonnet
// когда нужно. Подробности — в memory/project_billing_economics.md.
func PickDefault(models []Model, pinnedID string) *Model {
	if len(models) == 0 {
		return nil
	}
	if pinnedID != "" {
		for i := range models {
			if models[i].ID == pinnedID {
				return &models[i]
			}
		}
	}

	idAliases := map[string]bool{
		"kimi-k2.6": true, "kimi-k2-6": true, "kimi_k2_6": true,
		"kimik2.6": true,
	}
	nameNeedles := []string{"kimi k2.6", "kimi-k2.6", "kimi k2 6"}

	normalize := func(s string) string {
		return strings.ToLower(strings.Join(strings.Fields(s), " "))
	}
	for i := range models {
		if idAliases[normalize(models[i].ID)] {
			return &models[i]
		}
	}
	for i := range models {
		n := normalize(models[i].Name)
		for _, k := range nameNeedles {
			if strings.Contains(n, k) {
				return &models[i]
			}
		}
	}
	for i := range models {
		if models[i].IsPrimary {
			return &models[i]
		}
	}
	return &models[0]
}
