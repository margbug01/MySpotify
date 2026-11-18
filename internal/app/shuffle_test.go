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

// The shuffle button plays the playable track the random source picked,
// not the playlist's first, and turns the player's shuffle on.
func TestPlaylistShuffleStartsWhereRandomPicks(t *testing.T) {
	was := randomPick
	randomPick = func(n int) int {
		if n != 2 {
			t.Fatalf("randomPick(%d), want 2: one playable song of two", n)
		}
		return 1
	}
	defer func() { randomPick = was }()

	a := shuffleApp()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("随机播放"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.pl.Shuffle() {
		t.Error("the shuffle button left the player's shuffle off")
	}
	if len(a.pl.Queue()) != 3 {
		t.Errorf("the queue holds %d songs, want the playlist's three", len(a.pl.Queue()))
	}
	if cur := a.pl.Current(); cur == nil || cur.ID != "3" {
		t.Errorf("the shuffle started at %v, want the song the random source picked (富士山下)", cur)
	}
}

// Ordinary playlist play uses the restored shuffle mode, which it used to turn off.
func TestPlaylistPlayKeepsRestoredShuffle(t *testing.T) {
	was := randomPick
	randomPick = func(int) int { return 1 }
	defer func() { randomPick = was }()

	a := shuffleApp()
	a.pl.Restore(player.Session{Queue: []spotify.Track{{ID: "old"}}, Order: []int{0}, Shuffle: true})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("播放"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if cur := a.pl.Current(); !a.pl.Shuffle() || cur == nil || cur.ID != "3" {
		t.Fatalf("ordinary play: shuffle %v, track %v; want shuffle on and track 3", a.pl.Shuffle(), cur)
	}
}

// A shuffled queue still starts at the playlist's first when the random
// source picks it, as it may.
func TestPlaylistShuffleCanStartAtFirst(t *testing.T) {
	was := randomPick
	randomPick = func(n int) int { return 0 }
	defer func() { randomPick = was }()

	a := shuffleApp()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("随机播放"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if cur := a.pl.Current(); cur == nil || cur.ID != "1" {
		t.Errorf("the shuffle started at %v, want the first song (晴天)", cur)
	}
}
