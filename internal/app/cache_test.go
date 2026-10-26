package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// Choosing the cache's size in the settings keeps it.
func TestCacheSizeChosen(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.cacheUsed = 0
	tt := ui.NewTester(a.settingsView, 560, 1400)
	tt.Frame()
	if err := tt.Click("1 GB"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(300 * time.Millisecond)
	tt.Frame()
	if err := tt.Click("512 MB"); err != nil {
		t.Fatalf("%v; shows %q", err, tt.Texts())
	}
	tt.Frame()
	tt.Frame()
	if a.settings.CacheMB != 512 {
		t.Errorf("cache of %d MB, want 512", a.settings.CacheMB)
	}
}
