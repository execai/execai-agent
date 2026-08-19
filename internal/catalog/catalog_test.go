package catalog

import (
	"strings"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

func primaryOf(t *testing.T, provider string, ids []string) string {
	t.Helper()
	var primary string
	for _, m := range For(provider, ids) {
		if m.IsPrimary {
			if primary != "" {
				t.Errorf("primary больше одного: %s и %s", primary, m.ID)
			}
			primary = m.ID
		}
	}
	return primary
}

// The Coding Plan and the open platform share model IDs (glm-5.2, glm-4.7…),
// so Provider is the only thing that keeps the two catalogs apart. Mixing them
// up sends a request to the wrong endpoint with the wrong key.
func TestZAIAPI_ProviderAndPrimary(t *testing.T) {
	// Server order deliberately puts the weaker model first.
	got := For(subscriptions.SourceZAIAPI, []string{"glm-4.7", "glm-5.2", "glm-4.6"})
	if len(got) != 3 {
		t.Fatalf("ожидалось 3 модели, получено %d", len(got))
	}
	for _, m := range got {
		if m.Provider != "zai-api" {
			t.Errorf("модель %s помечена провайдером %q вместо zai-api", m.ID, m.Provider)
		}
	}
	if p := primaryOf(t, subscriptions.SourceZAIAPI, []string{"glm-4.7", "glm-5.2", "glm-4.6"}); p != "glm-5.2" {
		t.Errorf("primary = %q, ожидался glm-5.2 (приоритет, а не порядок сервера)", p)
	}
}

func TestUnknownIDsStillGetPrimary(t *testing.T) {
	got := For(subscriptions.SourceZAIAPI, []string{"glm-experimental-x", "some-other"})
	if len(got) != 2 {
		t.Fatalf("ожидалось 2 модели, получено %d", len(got))
	}
	if !got[0].IsPrimary {
		t.Error("ни одна модель не помечена primary — каталог без primary сломает переключение")
	}
}

func TestOpenAI_PrimaryIsBestAvailable(t *testing.T) {
	cases := []struct {
		name        string
		ids         []string
		wantPrimary string
	}{
		{"has gpt-5", []string{"gpt-4o", "gpt-5", "o4-mini"}, "gpt-5"},
		{"no gpt-5, has o3", []string{"gpt-4o", "o3", "o4-mini"}, "o3"},
		{"only gpt-4o-mini", []string{"gpt-4o-mini", "text-embedding-3-small"}, "gpt-4o-mini"},
		{"unknown only", []string{"whisper-1", "text-embedding-3"}, "whisper-1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if p := primaryOf(t, subscriptions.SourceOpenAI, tc.ids); p != tc.wantPrimary {
				t.Errorf("primary = %q, want %q", p, tc.wantPrimary)
			}
		})
	}
}

// Every model in every catalog must carry a name: the editor panel shows the
// name, and a catalog of bare IDs is exactly the "strange list" the owner saw.
func TestEveryModelHasAName(t *testing.T) {
	cases := []struct {
		provider string
		ids      []string
	}{
		{subscriptions.SourceKimi, []string{"k3", "kimi-for-coding-highspeed", "kimi-brand-new"}},
		{subscriptions.SourceKimiAPI, []string{"kimi-latest", "moonshot-v1-128k"}},
		{subscriptions.SourceZAIAPI, []string{"glm-5.2"}},
		{subscriptions.SourceOpenAI, []string{"gpt-5"}},
		{subscriptions.SourceOpenRouter, []string{"anthropic/claude-sonnet-4.5", "someone/weird-model"}},
		{subscriptions.SourceKimi, nil},       // built-in fallback
		{subscriptions.SourceOpenRouter, nil}, // built-in fallback
	}
	for _, tc := range cases {
		for _, m := range For(tc.provider, tc.ids) {
			if strings.TrimSpace(m.Name) == "" {
				t.Errorf("%s: модель %q без имени", tc.provider, m.ID)
			}
			if m.Provider == "" {
				t.Errorf("%s: модель %q без провайдера", tc.provider, m.ID)
			}
		}
	}
}

// The Kimi Coding Plan endpoint accepts three documented IDs; the built-in
// catalog is what gives them readable names and order. A live list must not
// drop them, and must not hide anything the plan gained since this build.
func TestKimiCodingKeepsBuiltinAndAddsNew(t *testing.T) {
	got := For(subscriptions.SourceKimi, []string{"kimi-for-coding", "kimi-k4-preview"})
	ids := map[string]string{}
	for _, m := range got {
		ids[m.ID] = m.Name
	}
	for _, want := range []string{"k3", "kimi-for-coding", "kimi-for-coding-highspeed"} {
		if _, ok := ids[want]; !ok {
			t.Errorf("встроенная модель %s пропала из каталога", want)
		}
	}
	if ids["kimi-for-coding"] != "Kimi K2.7 Code" {
		t.Errorf("имя из встроенного каталога потеряно: %q", ids["kimi-for-coding"])
	}
	if _, ok := ids["kimi-k4-preview"]; !ok {
		t.Error("новая модель с сервера не попала в каталог")
	}
}

// OpenRouter lists hundreds of models: the families people code with must come
// first, or the picker is unusable.
func TestOpenRouterDropsBatchVariants(t *testing.T) {
	got := For(subscriptions.SourceOpenRouter, []string{
		"anthropic/claude-opus-4.5", "anthropic/claude-opus-4.5:batch", "openai/gpt-5",
	})
	for _, m := range got {
		if strings.HasSuffix(m.ID, ":batch") {
			t.Errorf("batch-вариант %s остался в каталоге — в чате он не работает", m.ID)
		}
	}
	if len(got) != 2 {
		t.Errorf("ожидалось 2 модели после фильтра, получено %d", len(got))
	}
}

func TestOpenRouterOrdersCodingFamiliesFirst(t *testing.T) {
	got := For(subscriptions.SourceOpenRouter, []string{
		"aaa/first-alphabetically", "deepseek/deepseek-chat", "anthropic/claude-sonnet-4.5", "zzz/last",
	})
	if got[0].ID != "anthropic/claude-sonnet-4.5" {
		t.Errorf("первой должна быть claude-sonnet, а не %q", got[0].ID)
	}
	if !got[0].IsPrimary {
		t.Error("первая модель не помечена primary")
	}
	if got[len(got)-1].ID == "deepseek/deepseek-chat" {
		t.Error("deepseek оказался в самом хвосте — приоритет не сработал")
	}
}
