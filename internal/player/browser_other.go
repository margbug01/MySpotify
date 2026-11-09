//go:build !linux

package player

func startLinuxBrowser(w *Web) (bool, error) { return false, nil }
