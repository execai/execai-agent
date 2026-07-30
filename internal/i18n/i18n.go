// Package i18n is a simple localizer for the execai TUI.
//
// Usage:
//
//	import "github.com/velesbsdllc/agent-vbai/internal/i18n"
//
//	i18n.T("welcome.hero")                       // string in the active locale
//	i18n.Tf("stats.tokens", 1234, 567)           // fmt.Sprintf with localization
//
// Locale auto-detection at startup — from $LC_ALL / $LC_MESSAGES / $LANG.
// Override via config.Locale or /lang <code>. Fallback → English.
//
// Supported locales are registered by package initialization of
// messages/{en,ru,es,de,zh}. To add a new language → create messages/<code>.go
// with an init() that calls Register(<code>, map).
package i18n

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

// DefaultLocale is used when nothing else matched.
const DefaultLocale = "en"

var (
	mu       sync.RWMutex
	catalogs = map[string]map[string]string{} // "en" → {key: text}
	active   = DefaultLocale                  // current locale
)

// Register is called from messages/*.go in init() to register a locale.
// The catalog for a single locale is a plain map[key]text.
// Multiple Register calls for the same locale MERGE (later keys win) —
// this lets catalogs be split across several files (en.go, en_subs.go, …).
func Register(locale string, messages map[string]string) {
	mu.Lock()
	defer mu.Unlock()
	if existing, ok := catalogs[locale]; ok {
		for k, v := range messages {
			existing[k] = v
		}
		return
	}
	catalogs[locale] = messages
}

// SetLocale activates a locale. If missing — silently fall back to the default.
// Returns the locale actually selected (after fallback).
func SetLocale(locale string) string {
	mu.Lock()
	defer mu.Unlock()
	locale = normalize(locale)
	if _, ok := catalogs[locale]; ok {
		active = locale
		return locale
	}
	active = DefaultLocale
	return DefaultLocale
}

// Locale returns the currently active locale.
func Locale() string {
	mu.RLock()
	defer mu.RUnlock()
	return active
}

// Available returns the list of registered locales (sorted for the UI).
func Available() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(catalogs))
	for k := range catalogs {
		out = append(out, k)
	}
	// Order: en first, then alphabetical.
	sortLocales(out)
	return out
}

// T returns the string for a key in the active locale. If absent — fall back
// to en, and if absent there too — return the key itself (so the developer
// notices the missing key).
func T(key string) string {
	mu.RLock()
	defer mu.RUnlock()
	if cat, ok := catalogs[active]; ok {
		if v, ok := cat[key]; ok {
			return v
		}
	}
	if active != DefaultLocale {
		if cat, ok := catalogs[DefaultLocale]; ok {
			if v, ok := cat[key]; ok {
				return v
			}
		}
	}
	return key
}

// Tf is like T, but applies fmt.Sprintf to the template.
func Tf(key string, args ...any) string {
	return fmt.Sprintf(T(key), args...)
}

// Detect guesses the locale from env variables: LC_ALL / LC_MESSAGES / LANG.
// Returns a two-letter code or "" on failure.
//
//	Detect() == "ru"  // if LANG=ru_RU.UTF-8
//	Detect() == "en"  // if LANG=en_US.UTF-8 or C
//	Detect() == ""    // if nothing is set
func Detect() string {
	for _, env := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		v := os.Getenv(env)
		if v == "" {
			continue
		}
		return normalize(v)
	}
	return ""
}

// normalize reduces "ru_RU.UTF-8", "en-US", "zh-CN" to "ru"/"en"/"zh".
func normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	// Cut off '.', '_', '-' (locale modifier/encoding/region)
	for _, sep := range []string{".", "_", "-", "@"} {
		if i := strings.Index(s, sep); i > 0 {
			s = s[:i]
		}
	}
	// Some language branches:
	//   zh_CN, zh_TW, zh-Hans, zh-Hant → all → "zh" (we only have Simplified for now)
	//   pt_BR, pt_PT → "pt"
	//   sr_Latn → "sr"
	return s
}

func sortLocales(s []string) {
	// Put en first.
	for i, v := range s {
		if v == DefaultLocale {
			s[0], s[i] = s[i], s[0]
			break
		}
	}
	// The rest — simple insertion sort (few elements).
	for i := 2; i < len(s); i++ {
		for j := i; j > 1 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
