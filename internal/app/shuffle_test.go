package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"vinyl/internal/player"
	"vinyl/internal/spotify"
)

// shuffleApp is a test app showing a playlist whose second song cannot
// play, so a shuffled start has to pick among the first and the third.
func shuffleApp() *app {
	a := newTestApp()
	a.settings = defaultSettings()
	a.playlists["42"] = &spotify.Playlist{ID: "42", Name: "随机歌单", Tracks: []spotify.Track{
		{ID: "1", Name: "晴天", Artists: []spotify.Artist{{ID: "a", Name: "周杰伦"}}, Duration: 200000},
		{ID: "2", Name: "哑曲", Unplayable: true, Duration: 200000},
		{ID: "3", Name: "富士山下", Artists: []spotify.Artist{{ID: "b", Name: "陳奕迅"}}, Duration: 200000},
	}}
	a.route = route{page: pagePlaylist, playlist: "42"}
	return a
}

// Ordinary playlist play uses the restored shuffle mode, which it used to
// turn off, starting at the playable song the random source picked.
func TestPlaylistPlayKeepsRestoredShuffle(t *testing.T) {
	was := randomPick
	randomPick = func(int) int { return 1 }
	defer func() { randomPick = was }()

	a := shuffleApp()
	a.pl.Restore(player.Session{Queue: []spotify.Track{{ID: "old"}}, Order: []int{0}, Shuffle: true})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	click(t, tt, "播放")
	if cur := a.pl.Current(); !a.pl.Shuffle() || cur == nil || cur.ID != "3" {
		t.Fatalf("ordinary play: shuffle %v, track %v; want shuffle on and track 3", a.pl.Shuffle(), cur)
	}
}

// The shuffle button of a page of songs is a switch, as Spotify's: it turns
// shuffling on and off without playing anything, whichever the page, a
// playlist's, an artist's or the daily songs.
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
