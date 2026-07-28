package chat

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/velesbsdllc/agent-vbai/internal/config"
)

// loadInputHistory читает ~/.config/execai/input_history (по одной строке на запись).
// Возвращает максимум 200 последних. Молчит при ошибках.
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
		// Многострочные вводы кодируем через \n внутри одной строки как "\\n" → раскодируем.
		line := strings.ReplaceAll(sc.Text(), "\\n", "\n")
		out = append(out, line)
	}
	if len(out) > 200 {
		out = out[len(out)-200:]
	}
	return out
}

// saveInputHistory дописывает строку в файл истории.
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
	// Многострочные вводы кодируем чтоб одна строка = одна запись.
	enc := strings.ReplaceAll(line, "\n", "\\n")
	_, _ = f.WriteString(enc + "\n")
}
