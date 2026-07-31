package chat

import "testing"

// The Coding Plan and the open platform share model IDs (glm-5.2, glm-4.7…),
// so Provider is the only thing that keeps the two catalogs apart. Mixing them
// up sends a request to the wrong endpoint with the wrong key.
func TestBuildZAIAPIDynamicCatalog_ProviderAndPrimary(t *testing.T) {
	// Server order deliberately puts the weaker model first.
	got := buildZAIAPIDynamicCatalog([]string{"glm-4.7", "glm-5.2", "glm-4.6"})

	if len(got) != 3 {
		t.Fatalf("ожидалось 3 модели, получено %d", len(got))
	}
	var primary string
	for _, m := range got {
		if m.Provider != "zai-api" {
			t.Errorf("модель %s помечена провайдером %q вместо zai-api", m.ID, m.Provider)
		}
		if m.IsPrimary {
			if primary != "" {
				t.Errorf("primary больше одного: %s и %s", primary, m.ID)
			}
			primary = m.ID
		}
	}
	// Priority list must win over the server's ordering.
	if primary != "glm-5.2" {
		t.Errorf("primary = %q, ожидался glm-5.2 (приоритет, а не порядок сервера)", primary)
	}
}

func TestBuildZAIAPIDynamicCatalog_UnknownIDsStillGetPrimary(t *testing.T) {
	got := buildZAIAPIDynamicCatalog([]string{"glm-experimental-x", "some-other"})
	if len(got) != 2 {
		t.Fatalf("ожидалось 2 модели, получено %d", len(got))
	}
	if !got[0].IsPrimary {
		t.Error("ни одна модель не помечена primary — каталог без primary сломает переключение")
	}
}

// zai-api must be offered in both pickers, otherwise the legitimate path stays
// invisible and users keep connecting the restricted Coding Plan.
func TestPickersOfferZAIAPI(t *testing.T) {
	connect := filterConnectOptions("")
	if !hasLabel(connect, "zai-api") {
		t.Error("/connect не предлагает zai-api")
	}
	if !hasLabel(connect, "zai") {
		t.Error("/connect потерял zai")
	}

	use := filterUseOptionsFor(nil, "", "/source ")
	if !hasLabel(use, "zai-api") {
		t.Error("/source не предлагает zai-api")
	}
}

func hasLabel(items []suggestItem, want string) bool {
	for _, it := range items {
		if it.label == want {
			return true
		}
	}
	return false
}

// Провайдеров перечисляют ЧЕТЫРЕ независимых списка: палитра команд (allCommands),
// подсказки /connect, подсказки /source и валидация в handleConnect. Добавляя
// источник, легко обновить три из четырёх — так и случилось с zai-api: он был
// во всех пикерах, но не в палитре команд, и по /con его просто не было видно.
// Тест держит все списки согласованными.
func TestAllSourcesAppearInEveryList(t *testing.T) {
	// Источники, подключаемые ключом (CLI-делегирование и ollama живут по
	// своим правилам и проверяются отдельно).
	keyed := []string{"zai", "zai-api", "kimi", "kimi-api", "anthropic", "openai"}

	connect := filterConnectOptions("")
	use := filterUseOptionsFor(nil, "", "/source ")

	for _, src := range keyed {
		if !hasLabel(connect, src) {
			t.Errorf("%s отсутствует в подсказках /connect", src)
		}
		if !hasLabel(use, src) {
			t.Errorf("%s отсутствует в подсказках /source", src)
		}
		found := false
		for _, it := range allCommands {
			if it.label == "/connect "+src {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s отсутствует в палитре команд (allCommands) — не найдётся по /con", src)
		}
	}
}
