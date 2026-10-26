package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"vinyl/internal/spotify"
)

// A radio from the song playing plays on from it, with the radio's songs
// queued after; the chip over its title in Now Playing leaves the radio,
// the song playing on.
func TestRadioFromPlayingSong(t *testing.T) {
	seeds := make(chan string, 2)
	real := radioTracks
	radioTracks = func(seed string) ([]spotify.Track, error) { seeds <- seed; return nil, nil }
	defer func() { radioTracks = real }()

	a := newTestApp()
	a.settings = defaultSettings()
	a.pl.Cue([]spotify.Track{{ID: "1", Name: "晚风"}, {ID: "2", Name: "下一首"}})
	a.startRadio("spotify:track:1", "晚风")
	if got := <-seeds; got != "spotify:track:1" {
		t.Fatalf("asked the radio of %s", got)
	}
	// As the radio's songs come back.
	a.radioLoaded(radioName("spotify:track:1", "晚风"), "spotify:track:1",
		[]spotify.Track{{ID: "1", Name: "晚风"}, {ID: "7", Name: "像晚风的歌"}}, nil)
	if q := a.pl.Queue(); a.pl.Radio() == "" || a.pl.Current().ID != "1" || len(q) != 2 || q[1].ID != "7" {
		t.Fatalf("radio %q, current %v, queue %v", a.pl.Radio(), a.pl.Current(), q)
	}
	if a.pl.Loading() {
		t.Fatal("started the song playing over")
	}

	a.nowPlaying = true
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Advance(time.Second)
	tt.Frame()
	if !tt.HasText("晚风 电台") {
		t.Fatalf("no chip: %q", tt.Texts())
	}
	if err := tt.Click("退出电台"); err != nil {
		t.Fatal(err)
	}
	if a.pl.Radio() != "" || a.pl.Current().ID != "1" || len(a.pl.Queue()) != 1 {
		t.Fatalf("after leaving: radio %q, current %v, queue %d", a.pl.Radio(), a.pl.Current(), len(a.pl.Queue()))
	}
}
