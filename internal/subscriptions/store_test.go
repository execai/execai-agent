package subscriptions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDeriveKimiTier — Kimi tier detection from the subscription's model list.
func TestDeriveKimiTier(t *testing.T) {
	cases := []struct {
		name string
		sub  Subscription
		want string
	}{
		{"non-kimi", Subscription{Provider: SourceZAI, AvailableModels: []string{"k3"}}, ""},
		{"empty-models", Subscription{Provider: SourceKimi}, ""},
		{"only-k27", Subscription{Provider: SourceKimi, AvailableModels: []string{"kimi-for-coding"}}, "K2.7 Code"},
		{"k27+k3", Subscription{Provider: SourceKimi, AvailableModels: []string{"kimi-for-coding", "k3"}}, "K3"},
		{"all-three", Subscription{Provider: SourceKimi, AvailableModels: []string{"k3", "kimi-for-coding", "kimi-for-coding-highspeed"}}, "K3 + HighSpeed"},
		{"k3-only-no-k27", Subscription{Provider: SourceKimi, AvailableModels: []string{"k3"}}, "K3"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := deriveKimiTier(tc.sub); got != tc.want {
				t.Errorf("deriveKimiTier = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestSourceLabel_KimiWithTier — the status bar shows the tier from AvailableModels.
func TestSourceLabel_KimiWithTier(t *testing.T) {
	s := &Store{
		Active: SourceKimi,
		Subscriptions: map[string]Subscription{
			SourceKimi: {
				Provider:        SourceKimi,
				Plan:            "coding",
				AvailableModels: []string{"k3", "kimi-for-coding"},
			},
		},
	}
	label := s.SourceLabel()
	// Expect "kimi (K3)" — the tier derived from models takes priority over Plan="coding".
	if !strings.Contains(label, "K3") {
		t.Errorf("want label to include K3 tier, got %q", label)
	}
	if strings.Contains(label, "coding") {
		t.Errorf("tier should override Plan=coding, but label shows coding: %q", label)
	}
}

// TestSourceLabel_KimiFallbackToPlan — no AvailableModels → show Plan.
func TestSourceLabel_KimiFallbackToPlan(t *testing.T) {
	s := &Store{
		Active: SourceKimi,
		Subscriptions: map[string]Subscription{
			SourceKimi: {Provider: SourceKimi, Plan: "coding"},
		},
	}
	label := s.SourceLabel()
	if !strings.Contains(label, "coding") {
		t.Errorf("want fallback to Plan=coding, got %q", label)
	}
}

// TestSourceLabel_ExecAI — the default source.
func TestSourceLabel_ExecAI(t *testing.T) {
	s := &Store{}
	if got := s.SourceLabel(); got != "ExecAI" {
		t.Errorf("empty store → ExecAI, got %q", got)
	}
	s.Active = SourceExecAI
	if got := s.SourceLabel(); got != "ExecAI" {
		t.Errorf("explicit ExecAI, got %q", got)
	}
}

// Битый subscriptions.json не должен ронять агента.
//
// До 15.08 Load отдавал nil-хранилище, а десяток мест в коде пишет
// `subs, _ := Load()` и сразу зовёт методы — получалась паника на старте:
// агент не запускался вовсе. Правильное поведение — подняться на базовом
// источнике и сказать, что файл сломан.
func TestLoad_BrokenFileGivesUsableStore(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, "execai"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "execai", "subscriptions.json"),
		[]byte(`{"subscriptions": [ЭТО НЕ JSON`), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := Load()
	if err == nil {
		t.Error("ошибка обязана вернуться — иначе человек не узнает о поломке")
	}
	if s == nil {
		t.Fatal("хранилище nil — вызывающий код упадёт на первом же методе")
	}
	// И методы обязаны работать.
	if s.ActiveSubscription() != nil {
		t.Error("у пустого хранилища не может быть активной подписки")
	}
	if s.SourceLabel() == "" {
		t.Error("подпись источника должна быть осмысленной")
	}
	_ = s.List()
}

// И на nil-получателе тоже: это второй эшелон на случай, если кто-то
// соберёт Store сам и забудет проинициализировать.
func TestNilStoreMethodsDoNotPanic(t *testing.T) {
	var s *Store
	if s.ActiveSubscription() != nil || s.SourceLabel() != "ExecAI" || s.List() != nil {
		t.Error("методы nil-хранилища должны отвечать безопасными значениями")
	}
	if err := s.Activate("kimi"); err == nil {
		t.Error("активация на nil обязана вернуть ошибку, а не притвориться успешной")
	}
	s.Add(Subscription{Provider: "kimi"})
	s.Remove("kimi")
}
