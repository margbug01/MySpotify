//go:build !windows

package app

func ensureWindowsStartMenuShortcut(string) error { return nil }
