//go:build !darwin

package player

import "github.com/egoist/mygo"

// keepAwake does nothing: only WebKit, on macOS, suspends a hidden page
// that pauses.
func keepAwake(*mygo.Window) {}
