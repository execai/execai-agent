package subscriptions

import (
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
