package llm

import (
	"encoding/json"
	"testing"
)

// Картинка во внутреннем формате — строка; OpenAI-совместимые провайдеры ждут
// объект {"url": …}. Без перевода мультимодальная модель через OpenRouter или
// Moonshot картинки не видит, хотя умеет их.
func TestWithOpenAIImages_ShapeForVision(t *testing.T) {
	msgs := []AIMessage{{
		Role: "user",
		Content: []ContentBlock{
			{Type: "text", Text: "что на картинке"},
			{Type: "image_url", ImageURL: "data:image/png;base64,AAA"},
		},
	}}
	got := withOpenAIImages(msgs)
	raw, err := json.Marshal(got[0].Content)
	if err != nil {
		t.Fatal(err)
	}
	var blocks []map[string]any
	if err := json.Unmarshal(raw, &blocks); err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 2 {
		t.Fatalf("ожидалось 2 блока, получено %d: %s", len(blocks), raw)
	}
	img, ok := blocks[1]["image_url"].(map[string]any)
	if !ok {
		t.Fatalf("image_url не объект: %s", raw)
	}
	if img["url"] != "data:image/png;base64,AAA" {
		t.Fatalf("url потерялся: %s", raw)
	}
	// Исходное сообщение не должно меняться — история принадлежит вызывающему.
	if _, still := msgs[0].Content.([]ContentBlock); !still {
		t.Error("исходная история изменена")
	}
}
