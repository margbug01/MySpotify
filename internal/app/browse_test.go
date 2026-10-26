package app

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"

	"vinyl/internal/spotify"
)

// A tile of the browse page opens its category, whose rows show a row of
// cards each, all of them on asking.
func TestBrowseCategory(t *testing.T) {
	asked := make(chan string, 1)
	real := categoryDetail
	categoryDetail = func(uri string) (*spotify.CategoryPage, error) { asked <- uri; return nil, spotify.ErrNotFound }
	defer func() { categoryDetail = real }()

	const pop = "spotify:page:pop"
	a := newTestApp()
	a.settings = defaultSettings()
	a.cats = []spotify.Category{{ID: pop, Name: "流行", Color: "#dc148c"}, {ID: "spotify:page:rock", Name: "摇滚"}}
	a.route = route{page: pageBrowse}
	tt := ui.NewTester(a.view, 1200, 900)
	tt.Frame()
	if err := tt.Click("流行"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.route != (route{page: pageCategory, category: pop}) || <-asked != pop {
		t.Fatalf("on %+v", a.route)
	}

	// As the page loaded: a row of twelve playlists.
	var lists []spotify.Playlist
	for i := range 12 {
		lists = append(lists, spotify.Playlist{ID: fmt.Sprint("spotify:playlist:", i), Name: fmt.Sprint("歌单", i)})
	}
	a.catPages = map[string]*spotify.CategoryPage{pop: {Name: "流行", Shelves: []spotify.Shelf{{Title: "热门流行", Lists: lists}}}}
	tt.Frame()
	if _, ok := tt.Find("歌单11"); ok {
		t.Fatal("the whole row shows before asking")
	}
	if err := tt.Click("显示全部"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if _, ok := tt.Find("歌单11"); !ok {
		t.Error("the whole row does not show after asking")
	}
}

// The name of who made a playlist opens their profile, and the account
// menu the listener's own.
func TestProfileLinks(t *testing.T) {
	asked := make(chan string, 2)
	real := userProfile
	userProfile = func(id string) (*spotify.UserPage, error) { asked <- id; return nil, spotify.ErrNotFound }
	defer func() { userProfile = real }()

	a := newTestApp()
	a.settings = defaultSettings()
	pl := &spotify.Playlist{ID: "42", Name: "热歌", Owner: "spotify", Tracks: []spotify.Track{{ID: "1", Name: "一张相片"}}}
	pl.Creator.Nickname = "Spotify"
	a.playlists["42"] = pl
	a.route = route{page: pagePlaylist, playlist: "42"}
	tt := ui.NewTester(a.view, 1200, 900)
	tt.Frame()
	if err := tt.Click("Spotify"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.route != (route{page: pageUser, user: "spotify"}) || <-asked != "spotify" {
		t.Fatalf("on %+v", a.route)
	}
	if err := tt.Click(a.profile.Nickname); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("个人主页"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.route != (route{page: pageUser, user: a.profile.UserID}) || <-asked != a.profile.UserID {
		t.Fatalf("from the account menu: on %+v", a.route)
	}
}
