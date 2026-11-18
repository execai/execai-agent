//go:build windows

package uiwindow

import (
	"golang.org/x/sys/windows/registry"
)

// defaultBrowser — the ProgId the person chose for http links.
func defaultBrowser() string {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\Shell\Associations\UrlAssociations\http\UserChoice`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	id, _, err := k.GetStringValue("ProgId")
	if err != nil {
		return ""
	}
	return id
}

// appPath — where the registry's App Paths says an executable is: the
// person's own entry first, then the computer's.
func appPath(exe string) string {
	for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		k, err := registry.OpenKey(root, `SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\`+exe, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		p, _, err := k.GetStringValue("")
		k.Close()
		if err == nil && p != "" {
			return p
		}
	}
	return ""
}
