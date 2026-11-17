//go:build !darwin

// @author denovochen

package player

// Other platforms publish through MyGo's native media controls only.
func setNativeNowPlayingVisible(bool) {}
