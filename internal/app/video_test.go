package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"vinyl/internal/spotify"
)

// Now Playing offers the music video of a song that has one, not of one
// that hasn't, and its button turns music videos on and off.
func TestMVButtonTurnsVideosOnAndOff(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.pl.Cue([]spotify.Track{{ID: "1", Name: "APT.", Duration: 170000}})
	a.pl.SetLyrics(nil)
	tt := ui.NewTester(a.view, 1200, 800)
	a.nowPlaying = true
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	if _, ok := tt.Find("播放 MV"); ok {
		t.Fatal("MV offered for a song without one")
	}
	a.pl.SetMedia(&spotify.Media{Video: "160a9d67"})
	tt.Frame()
	if err := tt.Click("播放 MV"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.pl.VideoOn() {
		t.Fatal("music videos did not turn on")
	}
	if err := tt.Click("关闭 MV"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.pl.VideoOn() {
		t.Fatal("music videos did not turn off")
	}
}
