package chat

import "testing"

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
	keyed := []string{"zai", "zai-api", "kimi", "kimi-api", "anthropic", "openai", "openrouter"}

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
