package catalog

import (
	"sort"
	"strings"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

// sub builds a subscription whose live list is ids, the way /connect stores it.
func sub(provider string, ids ...string) subscriptions.Subscription {
	s := subscriptions.Subscription{Provider: provider}
	ms := make([]subscriptions.ModelMeta, 0, len(ids))
	for _, id := range ids {
		ms = append(ms, subscriptions.ModelMeta{ID: id})
	}
	s.Models = ms
	s.AvailableModels = ids
	return s
}

func primaryOf(t *testing.T, ms []llm.Model) string {
	t.Helper()
	var primary string
	for _, m := range ms {
		if m.IsPrimary {
			if primary != "" {
				t.Errorf("primary больше одного: %s и %s", primary, m.ID)
			}
			primary = m.ID
		}
	}
	return primary
}

func idsOf(ms []llm.Model) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.ID)
	}
	return out
}

// The point of the change: a source offers exactly what its provider reports.
// The built-in lists that used to be merged in kept showing models the account
// did not have (glm-5.2[1m] on Z.ai, k3 on a plan without it).
func TestFor_OnlyWhatTheProviderReports(t *testing.T) {
	for _, p := range []string{
		subscriptions.SourceZAI, subscriptions.SourceZAIAPI, subscriptions.SourceKimi,
		subscriptions.SourceKimiAPI, subscriptions.SourceAnthropic, subscriptions.SourceOllama,
		subscriptions.SourceOpenAI, subscriptions.SourceOpenRouter,
	} {
		got := idsOf(For(sub(p, "model-a", "model-b")))
		sort.Strings(got) // OpenAI and OpenRouter reorder; what matters is the set
		if strings.Join(got, ",") != "model-a,model-b" {
			t.Errorf("%s: каталог %v, ожидался ровно живой список [model-a model-b]", p, got)
		}
		if len(For(sub(p))) != 0 {
			t.Errorf("%s: без живого списка каталог должен быть пуст — встроенных больше нет", p)
		}
	}
}

// The Coding Plan and the open platform share model IDs (glm-5.2, glm-4.7…),
// so Provider is the only thing that keeps the two catalogs apart. Mixing them
// up sends a request to the wrong endpoint with the wrong key.
func TestZAIAPI_ProviderAndPrimary(t *testing.T) {
	// Server order deliberately puts the weaker model first.
	got := For(sub(subscriptions.SourceZAIAPI, "glm-4.7", "glm-5.2", "glm-4.6"))
	if len(got) != 3 {
		t.Fatalf("ожидалось 3 модели, получено %d", len(got))
	}
	for _, m := range got {
		if m.Provider != "zai-api" {
			t.Errorf("модель %s помечена провайдером %q вместо zai-api", m.ID, m.Provider)
		}
	}
	if p := primaryOf(t, got); p != "glm-5.2" {
		t.Errorf("primary = %q, ожидался glm-5.2 (приоритет, а не порядок сервера)", p)
	}
}

func TestUnknownIDsStillGetPrimary(t *testing.T) {
	got := For(sub(subscriptions.SourceZAIAPI, "glm-experimental-x", "some-other"))
	if len(got) != 2 {
		t.Fatalf("ожидалось 2 модели, получено %d", len(got))
	}
	if !got[0].IsPrimary {
		t.Error("ни одна модель не помечена primary — каталог без primary сломает переключение")
	}
}

// The provider's own names win: Kimi called kimi-for-coding "K2.8 Preview"
// while the built-in catalog still said "K2.7 Code".
func TestFor_ProviderNamesWin(t *testing.T) {
	s := subscriptions.Subscription{Provider: subscriptions.SourceKimi}
	s.SetModels([]subscriptions.ModelMeta{
		{ID: "kimi-for-coding", Name: "K2.8 Preview"},
		{ID: "k3-256k"},
	}, s.ConnectedAt)
	got := For(s)
	if got[0].Name != "K2.8 Preview" {
		t.Errorf("имя = %q, ожидалось имя от провайдера", got[0].Name)
	}
	if got[1].Name == "" {
		t.Error("модель без имени от провайдера должна получить имя из ID")
	}
}

// The provider's own default (Codex isDefault) beats our order of preference.
func TestFor_ProviderDefaultIsPrimary(t *testing.T) {
	s := subscriptions.Subscription{Provider: subscriptions.SourceZAI}
	s.SetModels([]subscriptions.ModelMeta{{ID: "glm-5.3"}, {ID: "glm-4.7", Default: true}}, s.ConnectedAt)
	if p := primaryOf(t, For(s)); p != "glm-4.7" {
		t.Errorf("primary = %q, ожидался дефолт провайдера glm-4.7", p)
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
		{"newest plain alias", []string{"gpt-5-mini", "gpt-5", "gpt-5.5-2026-08-01", "gpt-5.5"}, "gpt-5.5"},
		{"only unknown", []string{"ft:custom-1"}, "ft:custom-1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if p := primaryOf(t, For(sub(subscriptions.SourceOpenAI, c.ids...))); p != c.wantPrimary {
				t.Errorf("primary = %q, ожидался %q", p, c.wantPrimary)
			}
		})
	}
}

// OpenAI lists embeddings, speech and images next to chat models — none of
// them can serve a turn.
func TestOpenAI_DropsNonChatModels(t *testing.T) {
	got := idsOf(For(sub(subscriptions.SourceOpenAI,
		"gpt-5", "text-embedding-3-large", "tts-1", "whisper-1", "dall-e-3",
		"omni-moderation-latest", "gpt-4o-realtime-preview", "gpt-image-1", "o3")))
	if strings.Join(got, ",") != "gpt-5,o3" {
		t.Errorf("каталог %v, ожидались только чатовые gpt-5 и o3", got)
	}
}

func TestEveryModelHasAName(t *testing.T) {
	for _, p := range []string{subscriptions.SourceOpenRouter, subscriptions.SourceOpenAI, subscriptions.SourceZAI} {
		for _, m := range For(sub(p, "anthropic/claude-sonnet-4.5", "kimi-for-coding-highspeed", "gpt-5")) {
			if strings.TrimSpace(m.Name) == "" {
				t.Errorf("%s: модель %s без имени", p, m.ID)
			}
		}
	}
}

func TestOpenRouterDropsBatchVariants(t *testing.T) {
	got := For(sub(subscriptions.SourceOpenRouter, "openai/gpt-5", "openai/gpt-5:batch"))
	if len(got) != 1 || got[0].ID != "openai/gpt-5" {
		t.Errorf("ожидалась одна модель без :batch, получено %v", idsOf(got))
	}
}

func TestOpenRouterOrdersCodingFamiliesFirst(t *testing.T) {
	got := For(sub(subscriptions.SourceOpenRouter,
		"aaa/some-model", "anthropic/claude-sonnet-4.5", "zzz/other", "openai/gpt-5"))
	if got[0].ID != "anthropic/claude-sonnet-4.5" || got[1].ID != "openai/gpt-5" {
		t.Errorf("порядок %v, ожидались семейства для кода сверху", idsOf(got))
	}
	if !got[0].IsPrimary {
		t.Error("первая модель должна быть primary")
	}
}

// The CLIs always offer "Default" (no --model): it works with any CLI version
// and is what a connection starts on even when the CLI could not be asked.
func TestCLI_DefaultAlwaysPresent(t *testing.T) {
	for _, p := range []string{subscriptions.SourceClaudeCLI, subscriptions.SourceCodexCLI} {
		got := For(sub(p))
		if len(got) != 1 || got[0].ID != "" || !got[0].IsPrimary {
			t.Errorf("%s без списка: %+v, ожидался один Default с пустым ID", p, got)
		}
		for _, m := range For(sub(p, "sonnet")) {
			if m.HasTools {
				t.Errorf("%s: у CLI свои инструменты, HasTools должен быть false", p)
			}
		}
	}
}

// Claude Code reports its own "default" (stored with an empty ID) — no second
// Default may be added. Codex marks a real model as default — that one is the
// primary, and the empty entry stays as a fallback below.
func TestCLI_ProviderDefaults(t *testing.T) {
	c := subscriptions.Subscription{Provider: subscriptions.SourceClaudeCLI}
	c.SetModels([]subscriptions.ModelMeta{{ID: "", Name: "Default (recommended)", Default: true}, {ID: "sonnet"}}, c.ConnectedAt)
	got := For(c)
	if len(got) != 2 || got[0].Name != "Default (recommended)" || !got[0].IsPrimary {
		t.Errorf("claude: %+v", got)
	}

	x := subscriptions.Subscription{Provider: subscriptions.SourceCodexCLI}
	x.SetModels([]subscriptions.ModelMeta{{ID: "gpt-5.6-sol", Default: true}, {ID: "gpt-5.6-luna"}}, x.ConnectedAt)
	if p := primaryOf(t, For(x)); p != "gpt-5.6-sol" {
		t.Errorf("codex: primary = %q, ожидался его isDefault gpt-5.6-sol", p)
	}
}

// Models a person typed come first, then the live list, without duplicates;
// a custom endpoint drops embedding models like OpenAI does.
func TestCustom_ManualFirstAndChatOnly(t *testing.T) {
	s := sub("lm-studio", "qwen3-coder", "nomic-embed-text", "llama-4")
	s.API = subscriptions.APIOpenAI
	s.ManualModels = []string{"my-finetune", "llama-4"}
	got := idsOf(For(s))
	if strings.Join(got, ",") != "my-finetune,llama-4,qwen3-coder" {
		t.Errorf("каталог %v", got)
	}
	for _, m := range For(s) {
		if m.Provider != "lm-studio" {
			t.Errorf("модель %s помечена %q, ожидался id кастомного провайдера", m.ID, m.Provider)
		}
	}
}

// Stores written by older builds carry only bare IDs — they must still work.
func TestFor_ReadsLegacyIDOnlyStore(t *testing.T) {
	s := subscriptions.Subscription{Provider: subscriptions.SourceZAI, AvailableModels: []string{"glm-5.3", "glm-4.7"}}
	if got := idsOf(For(s)); strings.Join(got, ",") != "glm-5.3,glm-4.7" {
		t.Errorf("каталог %v", got)
	}
}
