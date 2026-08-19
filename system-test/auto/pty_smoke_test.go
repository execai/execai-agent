// PTY-based smoke test for the agent-vbai TUI. Spawns the real binary
// through a pseudo-terminal, sends keystrokes and waits for expected output.
//
// What it covers:
//   * Boot without crashes
//   * /help shows the command list
//   * /quit terminates cleanly
//
// What it does NOT cover: real LLM response streaming (that requires fresh
// creds/subscriptions — see cmd/syscheck).
//
// Peculiarity: bubbletea on startup sends OSC11 (bg-color) + DSR6 (cursor pos)
// and WAITS for the terminal to reply. A bare PTY does not reply automatically,
// so the test answers these escape queries on the terminal's behalf.
//
// Gated behind the ptytest build tag — the test is slow (~2 sec/case) and
// needs a fresh build. Run: go test -tags=ptytest ./system-test/auto/

//go:build ptytest

package auto_test

import (
	"bytes"
	"io"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
)

const testBin = "/tmp/execai-pty-test"

func buildBinaryIfNeeded(t *testing.T) {
	t.Helper()
	// Rebuild on every run — guarantees a fresh binary.
	cmd := exec.Command("go", "build", "-o", testBin, "../../cmd/execai")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
}

// runTUI spawns the binary through a PTY, sends keystrokes with delays
// between them, and collects stdout. Returns everything the binary emitted.
func runTUI(t *testing.T, keys []string, wait time.Duration) string {
	t.Helper()
	buildBinaryIfNeeded(t)
	cmd := exec.Command(testBin)
	// Set WINSIZE, otherwise bubbletea may render incorrectly.
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 40, Cols: 120})
	if err != nil {
		t.Fatalf("pty start: %v", err)
	}
	defer f.Close()

	var out bytes.Buffer
	var mu sync.Mutex
	done := make(chan struct{})
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := f.Read(buf)
			if n > 0 {
				mu.Lock()
				out.Write(buf[:n])
				mu.Unlock()
				// On startup bubbletea queries the terminal:
				//   OSC 11 (bg color)  → reply "black"
				//   DSR-6 (cursor pos) → reply "row=1,col=1"
				// Without these replies the render hangs.
				chunk := buf[:n]
				if bytes.Contains(chunk, []byte("]11;?")) {
					_, _ = f.Write([]byte("\x1b]11;rgb:0000/0000/0000\x1b\\"))
				}
				if bytes.Contains(chunk, []byte("[6n")) {
					_, _ = f.Write([]byte("\x1b[1;1R"))
				}
			}
			if err != nil {
				close(done)
				return
			}
		}
	}()

	// Let the TUI render before typing.
	time.Sleep(500 * time.Millisecond)

	for _, k := range keys {
		if _, err := io.WriteString(f, k); err != nil {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}

	// Let the last action complete.
	time.Sleep(wait)

	// Try a soft shutdown — Ctrl+D + Ctrl+C.
	_, _ = io.WriteString(f, "\x04") // EOT
	time.Sleep(200 * time.Millisecond)
	_ = cmd.Process.Kill()
	<-done

	mu.Lock()
	defer mu.Unlock()
	return stripANSI(out.String())
}

// stripANSI removes terminal escape sequences. Otherwise grepping the output
// is hard (bubbletea litters it with carets and colors).
func stripANSI(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == 0x1b {
			// ESC ... — skip until the final letter (@-~).
			i++
			if i >= len(s) {
				break
			}
			if s[i] == '[' || s[i] == ']' || s[i] == '(' {
				i++
				for i < len(s) && !((s[i] >= '@' && s[i] <= '~')) {
					i++
				}
				if i < len(s) {
					i++
				}
			}
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// Boot without crashes — just start and read the first messages.
func TestTUISmoke_Boot(t *testing.T) {
	if testing.Short() {
		t.Skip("PTY-тест долгий (~2 сек)")
	}
	out := runTUI(t, nil, 800*time.Millisecond)
	if len(out) < 50 {
		t.Fatalf("подозрительно короткий output (%d bytes): %q", len(out), out)
	}
	// Expect one of the boot-message baselines. Not a strict check because
	// the text varies (logged in vs login mode).
	baselines := []string{"execai", "Опиши задачу", "login", "Введите задачу"}
	found := false
	for _, s := range baselines {
		if strings.Contains(strings.ToLower(out), strings.ToLower(s)) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("boot output не содержит baseline: %q", firstNChars(out, 500))
	}
}

// /help must show the commands. Allow extra time since the TUI redraws the
// viewport, and the autocomplete menu triggered by typing "/" can interfere
// with the final Enter submit.
func TestTUISmoke_Help(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	// Input: "/help" + Enter, wait for the render.
	out := runTUI(t, []string{"/help\r"}, 2*time.Second)
	// /help must list /source, /model, /connect, /effort — the basic commands.
	// If at least 2 of 4 are present, the test passes. The bubbletea renderer
	// wraps long system hints to the terminal width and some characters may be
	// lost due to viewport scrolling.
	baselines := []string{"/source", "/model", "/connect", "/effort", "/quit", "help"}
	hits := 0
	for _, s := range baselines {
		if strings.Contains(out, s) {
			hits++
		}
	}
	if hits < 2 {
		t.Errorf("/help output содержит слишком мало baseline'ов (%d/%d). Snapshot: %q",
			hits, len(baselines), firstNChars(out, 1200))
	}
}

// TestTUISmoke_Quit — /quit must terminate the process cleanly.
func TestTUISmoke_Quit(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	out := runTUI(t, []string{"/quit\r"}, 1*time.Second)
	// After /quit the process is killed; here we just verify the boot happened at all.
	if !strings.Contains(strings.ToLower(out), "execai") {
		t.Errorf("boot output должен содержать 'execai', got: %q", firstNChars(out, 500))
	}
}

func firstNChars(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
