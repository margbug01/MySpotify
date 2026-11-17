//go:build !darwin

package player

// Other platforms publish through MyGo's native media controls only.
func setNativeNowPlayingVisible(bool) {}
