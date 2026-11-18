//go:build !linux && !darwin && !windows

package sync

// osDirIdentity — no way to tell folders apart here: no base is trusted.
func osDirIdentity(string) string { return "" }
