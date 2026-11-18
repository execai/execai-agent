package chat

import (
	"os"
	"path/filepath"
	"testing"

	agentsync "github.com/velesbsdllc/agent-vbai/internal/sync"
)

// Б-1 of the second loss review: the chat wrote the memory index template
// at its start, before sync's first exchange. In a folder whose index was
// deleted the template stood for the index, and an index without entries
// went up as «every entry removed» — on every computer. With sync on the
// template waits for the first full exchange; with sync off it is written
// at the start as before.
func TestMemoryIndex_TemplateWaitsForTheFirstExchange(t *testing.T) {
	paths, err := agentsync.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	index := filepath.Join(paths.MemoryDir(), "MEMORY.md")
	reset := func(syncOn bool) {
		_ = os.Remove(index)
		_ = os.Remove(paths.StateFile())
		if syncOn {
			if err := os.MkdirAll(paths.ConfigDir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(paths.StateFile(), []byte(`{"enabled":true}`), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	t.Cleanup(func() { reset(false) })

	reset(false)
	ensureMemoryIndexAtBoot()
	if _, err := os.Stat(index); err != nil {
		t.Fatalf("sync off: no template at the start: %v", err)
	}

	reset(true)
	ensureMemoryIndexAtBoot()
	if _, err := os.Stat(index); err == nil {
		t.Fatal("sync on: the template was written before the first exchange")
	}
	m := &tuiModel{}
	if _, ok := m.updateSync(syncExchangedMsg{}); !ok {
		t.Fatal("the message is not handled")
	}
	if _, err := os.Stat(index); err != nil {
		t.Fatalf("no template after the first exchange: %v", err)
	}
	// Once per chat: an index the person removes later is not written
	// again behind sync's back.
	_ = os.Remove(index)
	m.updateSync(syncExchangedMsg{})
	if _, err := os.Stat(index); err == nil {
		t.Error("the template was written again after a later exchange")
	}
}
