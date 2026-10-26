package app

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"vinyl/internal/spotify"
)

// A song's menu opens its page, whose play button plays it and then the
// songs like it, and whose credits open the pages of who made it.
func TestTrackPage(t *testing.T) {
	asked := make(chan string, 1)
	real := trackDetail
	trackDetail = func(id string) (*spotify.TrackPage, error) { asked <- id; return nil, spotify.ErrNotFound }
	defer func() { trackDetail = real }()

	a := newTestApp()
	a.settings = defaultSettings()
	a.playlists["42"] = &spotify.Playlist{ID: "42", Name: "飙升榜", Tracks: []spotify.Track{
		{ID: "1", Name: "一张相片", Artists: []spotify.Artist{{Name: "林宥嘉"}}, Duration: 200000},
	}}
	a.route = route{page: pagePlaylist, playlist: "42"}
	tt := ui.NewTester(a.view, 1200, 900)
	tt.Frame()
	if err := tt.RightClick("一张相片"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("查看歌曲"); err != nil {
		t.Fatalf("%v; the menu shows %q", err, tt.Menu())
	}
	tt.Frame()
	if a.route != (route{page: pageTrack, track: "1"}) || <-asked != "1" {
		t.Fatalf("on %+v", a.route)
	}

	// As the page loaded.
	a.tracks["1"] = &spotify.TrackPage{
		Track:       spotify.Track{ID: "1", Name: "一张相片", Artists: []spotify.Artist{{ID: "a", Name: "林宥嘉"}}, Duration: 200000},
		Plays:       12345678,
		Recommended: []spotify.Track{{ID: "7", Name: "说谎", Duration: 200000}, {ID: "8", Name: "浪费", Duration: 200000}},
		Credits:     []spotify.Credit{{Name: "林夕", Artist: "spotify:artist:44z3XbvziSONKWahFuJ1qN", Roles: []string{"作词者"}}},
	}
	tt.Frame()
	if err := tt.Click("播放"); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, tr := range a.pl.Queue() {
		ids = append(ids, tr.ID)
	}
	if !slices.Equal(ids, []string{"1", "7", "8"}) || a.pl.Current().ID != "1" {
		t.Fatalf("queue %v, playing %v", ids, a.pl.Current())
	}
	if err := tt.Click("林夕"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.route != (route{page: pageArtist, artist: "spotify:artist:44z3XbvziSONKWahFuJ1qN"}) {
		t.Errorf("the credit opened %+v", a.route)
	}
}
