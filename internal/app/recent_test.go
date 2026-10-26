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

// What a queue was played from goes first among what was played lately
// once a song of it plays, not while it loads, and not at all if another
// queue took its place first; played from again, it moves up.
func TestPlayedFromGoesFirstOnceItPlays(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	list := spotify.Playlist{ID: "spotify:playlist:a", Name: "歌单", Tracks: []spotify.Track{{ID: "1"}, {ID: "2"}}}
	artist := spotify.ArtistCard{ID: "spotify:artist:b", Name: "歌手"}
	ids := func() []string {
		var out []string
		for _, it := range a.playedFrom {
			out = append(out, it.ID())
		}
		return out
	}
	plays := func() {
		out := audio.NewPlayer(&silence{})
		out.SetVolume(0)
		t.Cleanup(func() { out.Close() })
		a.pl.Attach(out)
		a.notePlayed()
	}
	a.playFrom(spotify.HomeItem{List: &list}, list.Tracks, 0)
	a.notePlayed() // loading, not playing
	if len(a.playedFrom) != 0 {
		t.Fatalf("kept %v before it played", ids())
	}
	plays()
	if got := ids(); !slices.Equal(got, []string{"spotify:playlist:a"}) {
		t.Fatalf("played from %v", got)
	}
	if a.playedFrom[0].List.Tracks != nil {
		t.Error("kept the list's songs")
	}
	a.playFrom(spotify.HomeItem{Artist: &artist}, []spotify.Track{{ID: "3"}}, 0)
	plays()
	a.playFrom(spotify.HomeItem{List: &list}, list.Tracks, 1)
	plays()
	if got := ids(); !slices.Equal(got, []string{"spotify:playlist:a", "spotify:artist:b"}) {
		t.Fatalf("played from %v, want the list then the artist", got)
	}
	// Played from the artist, but a search's songs played instead.
	a.playFrom(spotify.HomeItem{Artist: &artist}, []spotify.Track{{ID: "3"}}, 0)
	a.pl.PlayQueue([]spotify.Track{{ID: "9"}, {ID: "8"}}, 0)
	plays()
	if got := ids(); got[0] != "spotify:playlist:a" {
		t.Fatalf("played from %v: the artist went first for songs not theirs", got)
	}
}

// The home shows what Vinyl played first in Spotify's recently played, or
// in one of its own if Spotify's home has none, and first in the grid.
func TestHomeShowsWhatVinylPlayed(t *testing.T) {
	a := newTestApp()
	mine := spotify.Playlist{ID: "spotify:playlist:mine", Name: "听过的"}
	a.playedFrom = []spotify.HomeItem{{List: &mine}}
	happy := spotify.Playlist{ID: "spotify:playlist:happy", Name: "Happy Hits!"}
	a.homeSections = testSections(2)
	a.homeSections[1].Recent = true
	a.homeSections[1].Items = []spotify.HomeItem{{List: &happy}, {List: &mine}}
	got := a.withPlayedFrom(a.homeSections)
	if len(got) != 2 || got[1].Items[0].ID() != mine.ID || got[1].Items[1].ID() != happy.ID || len(got[1].Items) != 2 {
		t.Errorf("recently played = %+v", got[1].Items)
	}
	if a.homeSections[1].Items[0].ID() != happy.ID {
		t.Error("changed Spotify's section in place")
	}
	if grid := a.gridItems(); grid[0].ID() != mine.ID {
		t.Errorf("grid starts with %s", grid[0].ID())
	}
	a.homeSections[1].Recent = false
	got = a.withPlayedFrom(a.homeSections)
	if len(got) != 3 || !got[0].Recent || got[0].Items[0].ID() != mine.ID {
		t.Errorf("without Spotify's recently played: %d sections, first %+v", len(got), got[0])
	}
	if s := a.findSection("最近播放"); s == nil || s.Items[0].ID() != mine.ID {
		t.Error("its show all finds nothing")
	}
}
