//go:build darwin

package uiwindow

// defaultBrowser — the bundle id LaunchServices opens http links with;
// empty when the person never changed it (Safari).
func defaultBrowser() string {
	return parseLSHandlers(output("defaults", "read",
		"com.apple.LaunchServices/com.apple.launchservices.secure", "LSHandlers"))
}

func appPath(string) string { return "" }
