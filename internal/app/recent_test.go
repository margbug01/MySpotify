package app

import (
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/audio"
	"github.com/egoist/mygo/ui"

	"vinyl/internal/spotify"
)

// A song goes first among those played once it plays, not while it only
// loads; played again, it moves up rather than showing twice.
func TestPlayedKeepsTheLatestFirst(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	ids := func() []string {
		var out []string
		for _, t := range a.played {
			out = append(out, t.ID)
		}
		return out
	}
	play := func(id string) {
		a.pl.Cue([]spotify.Track{{ID: id, UID: "u" + id}})
		a.notePlayed() // cued, not playing
		out := audio.NewPlayer(&silence{})
		out.SetVolume(0)
		defer out.Close()
		a.pl.Attach(out)
		a.notePlayed()
		a.notePlayed() // as the player says something else changed
	}
	play("1")
	play("2")
	play("1")
	if got := ids(); !slices.Equal(got, []string{"1", "2"}) {
		t.Fatalf("played %v, want 1 then 2", got)
	}
	if a.played[0].UID != "" {
		t.Error("kept a playlist's place of the song")
	}
}

// With NM_FRAMES set, saves the page of what was played, here and in
// Spotify's apps.
func TestRecentFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	a := newTestApp()
	a.settings = defaultSettings()
	a.played = []spotify.Track{
		{ID: "1", Name: "一张相片", Artists: []spotify.Artist{{Name: "林宥嘉"}}, Album: spotify.Album{Name: "大小说家"}, Duration: 200000},
		{ID: "2", Name: "宝船", Artists: []spotify.Artist{{Name: "DOUDOU"}}, Album: spotify.Album{Name: "宝船"}, Duration: 180000},
	}
	a.recentLists = []spotify.Playlist{{ID: "spotify:playlist:1", Name: "富士山下 电台", Creator: struct{ Nickname string }{"Spotify"}}, {ID: "spotify:album:2", Name: "U87", Note: "专辑"}}
	a.recentArtists = []spotify.ArtistCard{{ID: "spotify:artist:1", Name: "陳奕迅"}}
	a.route = route{page: pageRecent}
	tt := ui.NewTester(a.view, 1200, 900)
	tt.Advance(time.Second)
	tt.Frame()
	f, _ := os.Create(filepath.Join(dir, "recent.png"))
	png.Encode(f, tt.Image())
	f.Close()
}
