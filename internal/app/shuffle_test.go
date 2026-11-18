package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"vinyl/internal/spotify"
)

// The shuffle button of a page of songs is a switch, as Spotify's: it turns
// shuffling on and off without playing anything, whichever the page, a
// playlist's, an artist's or the daily songs; the play button after it plays
// from a song at random.
func TestShuffleButtonIsASwitch(t *testing.T) {
	pages := map[string]func(*app){
		"playlist": func(a *app) {},
		"artist":   func(a *app) { a.route = route{page: pageArtist, artist: easonURI} },
		"daily": func(a *app) {
			a.daily = []spotify.Track{{ID: "t1", Name: "一首", Duration: 1000}, {ID: "t2", Name: "两首", Duration: 1000}}
			a.route = route{page: pageDaily}
		},
	}
	for name, open := range pages {
		a := artistApp()
		open(a)
		tt := ui.NewTester(a.view, 1200, 800)
		tt.Frame()
		click(t, tt, "开启随机播放")
		if !a.pl.Shuffle() || a.pl.Current() != nil {
			t.Fatalf("%s turned on: shuffle %v, playing %v", name, a.pl.Shuffle(), a.pl.Current())
		}
		click(t, tt, "关闭随机播放")
		if a.pl.Shuffle() || a.pl.Current() != nil {
			t.Fatalf("%s turned off: shuffle %v, playing %v", name, a.pl.Shuffle(), a.pl.Current())
		}
	}
}
