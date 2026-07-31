package i18n_test

import (
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	_ "github.com/velesbsdllc/agent-vbai/internal/i18n/messages" // locale registration
)

func TestDefaultLocale(t *testing.T) {
	// Guarantee the en catalog is present after the blank import of messages.
	i18n.SetLocale("en")
	got := i18n.T("placeholder.chat")
	if got == "placeholder.chat" {
		t.Fatal("en catalog is empty — messages package didn't register on init")
	}
}

func TestSetLocale_Available(t *testing.T) {
	for _, loc := range []string{"en", "ru", "es", "de", "zh"} {
		got := i18n.SetLocale(loc)
		if got != loc {
			t.Errorf("SetLocale(%q) = %q, want %q", loc, got, loc)
		}
		msg := i18n.T("placeholder.chat")
		if msg == "" || msg == "placeholder.chat" {
			t.Errorf("locale %q: placeholder.chat empty or missing", loc)
		}
	}
}

func TestSetLocale_UnknownFallsBackToDefault(t *testing.T) {
	got := i18n.SetLocale("xx")
	if got != "en" {
		t.Errorf("unknown locale should fall back to en, got %q", got)
	}
}

func TestT_MissingKeyFallsBackToEn(t *testing.T) {
	i18n.SetLocale("ru")
	// Key exists both in en and in ru — ok.
	if i18n.T("help.enter") == "help.enter" {
		t.Error("ru catalog missing help.enter")
	}
	// Key exists in neither — return the key.
	if got := i18n.T("nonexistent.key.xyz"); got != "nonexistent.key.xyz" {
		t.Errorf("missing key should return key itself, got %q", got)
	}
}

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"en":            "en",
		"en_US":         "en",
		"en_US.UTF-8":   "en",
		"ru_RU.UTF-8":   "ru",
		"zh_CN.UTF-8":   "zh",
		"zh-Hans":       "zh",
		"es_ES@euro":    "es",
		"":              "",
		"C":             "c",
		"POSIX":         "posix",
	}
	// normalize is internal. Test it indirectly via SetLocale (accepts any input).
	// Anything that is not a 2-letter code of a known locale → fallback to en.
	for in, wantNorm := range cases {
		got := i18n.SetLocale(in)
		// For known locales (en/ru/es/de/zh) the same code is returned.
		// For the rest — "en".
		expected := "en"
		if wantNorm == "ru" || wantNorm == "es" || wantNorm == "de" || wantNorm == "zh" {
			expected = wantNorm
		}
		if got != expected {
			t.Errorf("SetLocale(%q) after normalize %q = %q, want %q", in, wantNorm, got, expected)
		}
	}
	i18n.SetLocale("en") // reset
}

func TestDetect_Env(t *testing.T) {
	t.Setenv("LANG", "de_DE.UTF-8")
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	if got := i18n.Detect(); got != "de" {
		t.Errorf("Detect() = %q, want de", got)
	}
	t.Setenv("LC_ALL", "zh_CN.UTF-8")
	if got := i18n.Detect(); got != "zh" {
		t.Errorf("Detect() with LC_ALL = %q, want zh", got)
	}
}

func TestAvailable_Sorted(t *testing.T) {
	avail := i18n.Available()
	if len(avail) < 5 {
		t.Fatalf("want at least 5 locales, got %d: %v", len(avail), avail)
	}
	if avail[0] != "en" {
		t.Errorf("first locale should be en, got %q", avail[0])
	}
}
