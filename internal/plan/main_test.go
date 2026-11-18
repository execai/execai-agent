package plan

import (
	"os"
	"testing"
)

// TestMain keeps the package away from the person's machine: texts link to
// the plan page of this installation (config.json, credentials.json), and a
// developer's own config would change what the tests see.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "execai-plan-test-")
	if err != nil {
		panic(err)
	}
	os.Setenv("XDG_CONFIG_HOME", dir)
	os.Setenv("HOME", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
