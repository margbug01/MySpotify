//go:build !darwin

package app

// serveScripts does nothing: AppleScript is macOS's.
func (a *app) serveScripts() {}
