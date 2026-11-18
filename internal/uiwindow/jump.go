package uiwindow

import (
	"encoding/json"
	"html"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// The one-time key never goes into a command line. The arguments of a
// running process are open to every account of the computer (on Linux
// /proc/<pid>/cmdline is world-readable), and a browser started with
// --app=http://127.0.0.1:…/?t=<key> shows the key there for the half second
// it needs to come up — long enough for a program that watches the process
// list to sign in first. So the window is started with the address of a
// file instead: the file sits in the window's own profile directory (0700)
// and is readable by the person alone (0600); all it does is send the
// window on to the address with the key. It is removed as soon as the key
// has been used, after a while if it never is, and when the panel stops.

// jumpName — the file's name. One name: Chromium keeps the place and the
// size of an app window by the address it was started with.
const jumpName = "execai-open.html"

// WriteJump writes the file that takes the window to keyURL into the
// window's profile directory and returns its path.
func WriteJump(profile, keyURL string) (string, error) {
	if err := os.MkdirAll(profile, 0o700); err != nil {
		return "", err
	}
	// The directory may be older than this code, or made by a browser.
	if err := os.Chmod(profile, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(profile, jumpName)
	// Whatever is there — a file of an earlier run, a link somebody put in
	// its place — is removed, and the new file is created, never opened:
	// the key is not written into anything that existed before.
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	script, _ := json.Marshal(keyURL) // <, > and & come out as \u escapes
	page := "<!DOCTYPE html>\n<html><head><meta charset=\"utf-8\"><meta name=\"referrer\" content=\"no-referrer\"><title>ExecAI</title>\n" +
		// replace, not assign: the file leaves no entry in the window's history.
		"<script>location.replace(" + string(script) + ");</script>\n" +
		"<noscript><meta http-equiv=\"refresh\" content=\"0;url=" + html.EscapeString(keyURL) + "\"></noscript>\n" +
		"</head><body></body></html>\n"
	_, werr := f.WriteString(page)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		_ = os.Remove(path)
		return "", werr
	}
	return path, nil
}

// RemoveJump deletes the file; a missing one is fine.
func RemoveJump(path string) {
	if path != "" {
		_ = os.Remove(path)
	}
}

// FileURL — the file: address of a path of the given system.
func FileURL(path, goos string) string {
	if goos == "windows" {
		path = "/" + strings.ReplaceAll(path, `\`, "/")
	}
	return (&url.URL{Scheme: "file", Path: path}).String()
}
