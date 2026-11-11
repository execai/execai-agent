//go:build !linux && !darwin && !windows

package uiwindow

func defaultBrowser() string { return "" }

func appPath(string) string { return "" }
