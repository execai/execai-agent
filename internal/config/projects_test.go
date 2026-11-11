package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProjectState_RoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	a := filepath.Join(home, "a")
	b := filepath.Join(home, "b")
	if st := LoadProjectState(a); st.ImportOffer != "" {
		t.Fatalf("fresh state not empty: %+v", st)
	}
	// Reading must not create anything.
	if _, err := os.Stat(filepath.Join(home, ".config")); !os.IsNotExist(err) {
		t.Fatalf("LoadProjectState created the config folder")
	}
	if err := SaveProjectState(a, ProjectState{ImportOffer: ImportOfferDeclined}); err != nil {
		t.Fatal(err)
	}
	if err := SaveProjectState(b, ProjectState{ImportOffer: ImportOfferAccepted}); err != nil {
		t.Fatal(err)
	}
	if st := LoadProjectState(a + string(filepath.Separator)); st.ImportOffer != ImportOfferDeclined {
		t.Errorf("a: %+v", st)
	}
	if st := LoadProjectState(b); st.ImportOffer != ImportOfferAccepted {
		t.Errorf("b: %+v", st)
	}
	info, err := os.Stat(filepath.Join(home, ".config", "execai", "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("projects.json mode %v, want 0600", info.Mode().Perm())
	}
}
