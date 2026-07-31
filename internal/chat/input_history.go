package chat

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/velesbsdllc/agent-vbai/internal/config"
)

// loadInputHistory reads ~/.config/execai/input_history (one line per entry).
// Returns at most the 200 most recent. Silent on errors.
func loadInputHistory() []string {
	dir, err := config.Dir()
	if err != nil {
		return nil
	}
	f, err := os.Open(filepath.Join(dir, "input_history"))
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		// Multiline inputs are encoded with \n inside a single line as "\\n" → decode.
		line := strings.ReplaceAll(sc.Text(), "\\n", "\n")
		out = append(out, line)
	}
	if len(out) > 200 {
		out = out[len(out)-200:]
	}
	return out
}

// saveInputHistory appends a line to the history file.
func saveInputHistory(line string) {
	dir, err := config.Dir()
	if err != nil {
		return
	}
	_ = os.MkdirAll(dir, 0o700)
	f, err := os.OpenFile(filepath.Join(dir, "input_history"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	// Encode multiline inputs so that one line = one entry.
	enc := strings.ReplaceAll(line, "\n", "\\n")
	_, _ = f.WriteString(enc + "\n")
}
