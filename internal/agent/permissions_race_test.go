package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// Parallel agents (the main one and its subagents) each hold their own copy
// of permissions.json. Every "always" given at the same moment must survive:
// a copy that writes itself back whole would erase the others' grants.
func TestPermissions_ParallelSavesKeepEveryGrant(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)

	// Something granted before the agents started.
	seed, err := LoadPermissions()
	if err != nil {
		t.Fatal(err)
	}
	seed.AddTool("Read")
	if err := seed.Save(); err != nil {
		t.Fatal(err)
	}

	const n = 24
	copies := make([]*Permissions, n)
	for i := range copies {
		p, err := LoadPermissions()
		if err != nil {
			t.Fatal(err)
		}
		copies[i] = p
	}
	var wg sync.WaitGroup
	for i, p := range copies {
		wg.Add(1)
		go func(i int, p *Permissions) {
			defer wg.Done()
			if i%2 == 0 {
				p.AddTool(fmt.Sprintf("Tool%d", i))
			} else {
				p.AddExact(fmt.Sprintf("Bash|{\"command\":\"echo %d\"}", i))
			}
			if err := p.Save(); err != nil {
				t.Errorf("save %d: %v", i, err)
			}
		}(i, p)
	}
	wg.Wait()

	data, err := os.ReadFile(filepath.Join(dir, "execai", "permissions.json"))
	if err != nil {
		t.Fatal(err)
	}
	var disk struct {
		Tools []string `json:"always_allowed_tools"`
		Exact []string `json:"always_allowed_exact"`
	}
	if err := json.Unmarshal(data, &disk); err != nil {
		t.Fatalf("file is not valid JSON after a race: %v\n%s", err, data)
	}
	has := func(list []string, v string) bool {
		for _, x := range list {
			if x == v {
				return true
			}
		}
		return false
	}
	if !has(disk.Tools, "Read") {
		t.Error("the grant made before the agents started was lost")
	}
	for i := 0; i < n; i++ {
		if i%2 == 0 && !has(disk.Tools, fmt.Sprintf("Tool%d", i)) {
			t.Errorf("Tool%d lost", i)
		}
		if i%2 == 1 && !has(disk.Exact, fmt.Sprintf("Bash|{\"command\":\"echo %d\"}", i)) {
			t.Errorf("exact %d lost", i)
		}
	}
	// No temporary files left behind.
	entries, _ := os.ReadDir(filepath.Join(dir, "execai"))
	for _, e := range entries {
		if e.Name() != "permissions.json" {
			t.Errorf("leftover file %s", e.Name())
		}
	}

	// A copy loaded before the race sees the grants of the others at once.
	if !copies[1].HasTool("Tool0") {
		t.Error("a grant saved by another agent is not seen without a restart")
	}
}
