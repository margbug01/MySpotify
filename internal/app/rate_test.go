package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"vinyl/internal/spotify"
)

// An episode's speed, chosen from the menu of the button where shuffle
// goes, is how fast it plays, and is kept for the episodes after.
func TestChoosingAnEpisodesSpeed(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.pl.Cue([]spotify.Track{{ID: "ep", Name: "Episode", Episode: true, Duration: 3600 * 1000}})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("播放速度"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("1.5×"); err != nil {
		t.Fatalf("%v; the menu shows %q", err, tt.Menu())
	}
	tt.Frame()
	if a.pl.Rate() != 1.5 || a.settings.PodcastRate != 1.5 {
		t.Fatalf("rate %v, kept %v", a.pl.Rate(), a.settings.PodcastRate)
	}
}
