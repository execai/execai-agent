package tools

import (
	"encoding/base64"
	"os"
	"strings"
	"testing"
)

// testdata/search_stream.sse is a real response captured from browser-vbai
// (query "Kimi K3 context window size", mode fast). It carries both server-side
// quirks we compensate for: the "[Поиск] …" banner and the doubled answer.
func TestParseSearchStream_RealCapture(t *testing.T) {
	f, err := os.Open("testdata/search_stream.sse")
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	defer f.Close()

	text, sources, err := parseSearchStream(f)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if text == "" {
		t.Fatal("пустой текст ответа")
	}
	if strings.Contains(text, "[Поиск]") {
		t.Error("служебный баннер [Поиск] не отрезан")
	}
	// The answer opens with this sentence; a second occurrence means the
	// duplicate slipped through.
	const marker = "context window is 1,048,576 tokens"
	if n := strings.Count(text, marker); n != 1 {
		t.Errorf("ответ продублирован: вхождений %q = %d, ожидалось 1", marker, n)
	}
	if len(sources) == 0 {
		t.Fatal("источники не распарсились")
	}
	for i, s := range sources {
		if !strings.HasPrefix(s.URL, "http") {
			t.Errorf("источник %d: некорректный URL %q", i, s.URL)
		}
	}
	seen := map[string]bool{}
	for _, s := range sources {
		if seen[s.URL] {
			t.Errorf("дубликат источника: %s", s.URL)
		}
		seen[s.URL] = true
	}
}

func TestDedupeDoubledText(t *testing.T) {
	// Deliberately non-periodic: a repeated phrase would be genuinely ambiguous
	// (see the length guard in dedupeDoubledText).
	long := "Контекстное окно модели составляет 1 048 576 токенов, что обычно " +
		"описывают как 1M. Отдельные тарифы отдают меньше по умолчанию — " +
		"полное окно включается параметром запроса."
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"точный дубль", long + long, long},
		{"дубль с хвостом-приложением", long + long + "\nдополнение после второй копии",
			long},
		{"без дубля", long, long},
		{"короткая строка не трогается", "ответ", "ответ"},
		{"похожее начало, но другой текст", long + " А дальше идёт совсем другой абзац.",
			long + " А дальше идёт совсем другой абзац."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := dedupeDoubledText(c.in); got != c.want {
				t.Errorf("dedupeDoubledText():\n got %q\nwant %q", short(got), short(c.want))
			}
		})
	}
}

// A chunk boundary can fall inside a multi-byte character: bytes must be
// accumulated and decoded once, never per chunk. "Привет" split mid-rune.
func TestParseSearchStream_MultibyteAcrossChunks(t *testing.T) {
	raw := []byte("Привет, мир")
	half := 7 // inside the multi-byte "т"
	sse := "data: [FUNCTION_START]\n" +
		"data: " + `{"function_result":"output","content":"` + b64(raw[:half]) + `"}` + "\n" +
		"data: " + `{"function_result":"output","content":"` + b64(raw[half:]) + `"}` + "\n" +
		"data: [FUNCTION_END]\n"

	text, _, err := parseSearchStream(strings.NewReader(sse))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if text != "Привет, мир" {
		t.Errorf("склейка чанков сломала кириллицу: %q", text)
	}
}

func TestParseSearchStream_SkipsBrokenEvent(t *testing.T) {
	sse := "data: [FUNCTION_START]\n" +
		"data: {это не json}\n" +
		"data: " + `{"function_result":"output","content":"` + b64([]byte("ok")) + `"}` + "\n" +
		"data: [FUNCTION_END]\n"
	text, _, err := parseSearchStream(strings.NewReader(sse))
	if err != nil {
		t.Fatalf("битое событие не должно ронять разбор: %v", err)
	}
	if text != "ok" {
		t.Errorf("got %q", text)
	}
}

func short(s string) string {
	if len(s) > 80 {
		return s[:80] + "…"
	}
	return s
}

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }
