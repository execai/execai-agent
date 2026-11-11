//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/uiwindow"
)

// A real process in place of the browser: a script that writes down the
// arguments and the environment it was started with. Neither holds the
// one-time key — the two things about a process that other programs can
// read (the process list; on Linux /proc/<pid>/cmdline and, for the same
// account, /proc/<pid>/environ).
func TestUI_TheKeyIsNotInTheProcessList(t *testing.T) {
	r := newUIRig(t)
	dump := filepath.Join(t.TempDir(), "started-with")
	script := filepath.Join(t.TempDir(), "browser")
	body := "#!/bin/sh\n{ for a in \"$@\"; do printf 'arg %s\\n' \"$a\"; done; env | sed 's/^/env /'; } > '" + dump + ".tmp'\n" +
		"mv '" + dump + ".tmp' '" + dump + "'\nexec sleep 30\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	r.chrome = uiwindow.Browser{ID: "chrome", Name: "Test Browser", Path: script}
	r.deps.start = uiwindow.StartProcess
	done, cancel := r.run(uiFlags{})
	var got []byte
	r.wait("the browser to start", func() bool {
		b, err := os.ReadFile(dump)
		got = b
		return err == nil
	})
	var jump string
	for _, line := range strings.Split(string(got), "\n") {
		if p, ok := strings.CutPrefix(line, "arg --app=file://"); ok {
			jump = p
		}
	}
	if jump == "" {
		t.Fatalf("the browser was not started with a file:\n%s", got)
	}
	data, err := os.ReadFile(jump)
	if err != nil {
		t.Fatal(err)
	}
	m := jumpTarget.FindSubmatch(data)
	if m == nil {
		t.Fatalf("the file does not send the window to the panel:\n%s", data)
	}
	key := string(m[1])
	secret := key[strings.Index(key, "?t=")+3:]
	if len(secret) != 64 {
		t.Fatalf("key %q", key)
	}
	for _, line := range strings.Split(string(got), "\n") {
		if strings.Contains(line, secret) || strings.Contains(line, "?t=") {
			t.Errorf("the key is in what the process was started with: %s", line)
		}
	}
	cancel()
	if err := finished(t, done); err != nil {
		t.Fatal(err)
	}
}
