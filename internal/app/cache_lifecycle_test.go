package app

import (
	"testing"
	"vinyl/internal/player"
)

// Cache controls remain usable before a playback window has been created.
func TestCacheWithoutPlaybackWindow(t *testing.T) {
	cache := player.NewCache(t.TempDir(), 1<<20)
	app := &app{pl: &player.Player{Cache: cache}}
	if app.partCache() != cache {
		t.Fatal("cache depends on an open playback window")
	}
	app.partCache().Clear()
	if app.partCache().Size() != 0 {
		t.Fatal("cache was not cleared")
	}
	if app.pl.Web != nil {
		t.Fatal("cache controls created a playback window")
	}
}
