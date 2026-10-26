package app

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"vinyl/internal/spotify"
)

const easonURI = "spotify:artist:2QcZxAgcs2I1q7CtCkl6MI"

// artistApp is a test app showing a playlist whose songs credit two
// artists, with the first's page already loaded, as Spotify would give it.
func artistApp() *app {
	a := newTestApp()
	a.settings = defaultSettings()
	a.playlists["42"] = &spotify.Playlist{ID: "42", Name: "飙升榜", Tracks: []spotify.Track{
		{ID: "1", Name: "孤勇者", Artists: []spotify.Artist{{ID: "2QcZxAgcs2I1q7CtCkl6MI", Name: "陳奕迅"}, {ID: "x", Name: "合唱团"}},
			Album: spotify.Album{ID: "spotify:album:9", Name: "孤勇者"}, Duration: 200000},
	}}
	a.artists[easonURI] = &spotify.ArtistPage{
		ArtistCard: spotify.ArtistCard{ID: easonURI, Name: "陳奕迅"},
		TopTracks:  []spotify.Track{{ID: "5", Name: "淘汰", Duration: 280000}, {ID: "6", Name: "富士山下", Duration: 260000}},
		Popular:    []spotify.Playlist{{ID: "spotify:album:7", Name: "認了吧", Note: "2007 · 专辑"}},
		Related:    []spotify.ArtistCard{{ID: "spotify:artist:2MV", Name: "張敬軒"}},
	}
	a.route = route{page: pagePlaylist, playlist: "42"}
	return a
}

// A click on an artist's name in a song's row opens their page, without
// playing the song.
func TestArtistNameInRowOpensTheirPage(t *testing.T) {
	a := artistApp()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("陳奕迅"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.route != (route{page: pageArtist, artist: easonURI}) {
		t.Fatalf("route %+v, want the artist's page", a.route)
	}
	if a.pl.Current() != nil {
		t.Error("the click on the name played the song")
	}
	tt.Frame()
	if !tt.HasText("淘汰") {
		t.Error("the artist's page lacks their top track")
	}
	// Back goes to the playlist.
	a.back()
	if a.route.page != pagePlaylist {
		t.Errorf("back went to %+v", a.route)
	}
}

// A song's menu opens the page of each of its artists, and its album's.
func TestSongMenuOpensArtistAndAlbum(t *testing.T) {
	a := artistApp()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.RightClick("孤勇者"); err != nil {
		t.Fatal(err)
	}
	want := []string{"下一首播放", "添加到待播清单", "开启电台", "-", "复制歌名", "复制歌名和歌手", "-", "复制链接", "-", "查看歌手：陳奕迅", "查看歌手：合唱团", "查看专辑：孤勇者", "-", "添加到歌单"}
	if got := tt.Menu(); !slices.Equal(got, want) {
		t.Fatalf("the menu shows %q, want %q", got, want)
	}
	if err := tt.ChooseMenuItem("查看歌手：合唱团"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.route != (route{page: pageArtist, artist: "spotify:artist:x"}) {
		t.Errorf("route %+v, want 合唱团's page", a.route)
	}
}

// On an artist's page, play plays their top tracks from the first, and a
// related artist's card opens that artist's page.
func TestArtistPagePlaysTopTracks(t *testing.T) {
	a := artistApp()
	a.route = route{page: pageArtist, artist: easonURI}
	tt := ui.NewTester(a.view, 1200, 1400)
	tt.Frame()
	if err := tt.Click("播放"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if cur := a.pl.Current(); cur == nil || cur.ID != "5" || len(a.pl.Queue()) != 2 {
		t.Fatalf("playing %v of %d, want 淘汰 of the 2 top tracks", cur, len(a.pl.Queue()))
	}
	if err := tt.Click("張敬軒"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.route.artist != "spotify:artist:2MV" {
		t.Errorf("route %+v, want 張敬軒's page", a.route)
	}
}

// An album's card on an artist's page, under the list of their songs,
// opens the album.
func TestArtistPageOpensAlbum(t *testing.T) {
	a := artistApp()
	a.route = route{page: pageArtist, artist: easonURI}
	a.loadingPl["spotify:album:7"] = true // as if loading: nothing asks Spotify
	tt := ui.NewTester(a.view, 1200, 1400)
	tt.Frame()
	if err := tt.Click("認了吧"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.route != (route{page: pagePlaylist, playlist: "spotify:album:7"}) {
		t.Errorf("route %+v, want the album's page", a.route)
	}
}
