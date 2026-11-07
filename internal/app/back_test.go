package app

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"vinyl/internal/spotify"
)

// The back and forward buttons show a page where it was left, a list of
// tracks and a scrolled page alike, while opening a page anew starts at its
// top: every page used to come back at its top.
func TestBackKeepsThePlace(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	tracks := make([]spotify.Track, 200)
	for i := range tracks {
		tracks[i] = spotify.Track{ID: strconv.Itoa(i + 1), Name: fmt.Sprintf("曲目%d", i+1)}
	}
	a.playlists["42"] = &spotify.Playlist{ID: "42", Name: "飙升榜", Tracks: tracks}
	for i := range 40 {
		a.charts = append(a.charts, spotify.Playlist{ID: strconv.Itoa(100 + i), Name: fmt.Sprintf("榜单%d", i+1)})
	}
	tt := ui.NewTester(a.view, 1200, 800)
	settle := func() {
		tt.Frame()
		tt.Advance(time.Second)
		tt.Frame()
	}
	// at returns where a row or a card shows.
	at := func(label string) float32 {
		r, ok := tt.Find(label)
		if !ok {
			t.Fatalf("%s is not built on %+v", label, a.route)
		}
		return r.Y
	}
	scroll := func() {
		settle()
		tt.Wheel(700, 400, 0, 2400)
		settle()
	}

	a.open(route{page: pageCharts})
	settle()
	top := at("榜单20")
	scroll()
	left := a.pageScroll.Y
	if left == 0 {
		t.Fatal("the wheel did not scroll the charts")
	}

	a.open(route{page: pagePlaylist, playlist: "42"})
	scroll()
	row := at("曲目45")

	tt.Click("返回")
	settle()
	if a.route.page != pageCharts {
		t.Fatalf("back went to %+v", a.route)
	}
	if a.pageScroll.Y != left {
		t.Errorf("back shows the charts scrolled to %v, they were left at %v", a.pageScroll.Y, left)
	}

	tt.Click("前进")
	settle()
	if got := at("曲目45"); got != row {
		t.Errorf("forward shows the row at %v, it was left at %v", got, row)
	}

	a.open(route{page: pageCharts})
	settle()
	if got := at("榜单20"); got != top {
		t.Errorf("opened anew, shows the card at %v, its place at the top is %v", got, top)
	}
}

// Back from an album opened on the home shows the home where it was, as the
// listener left it to look at the album.
func TestBackFromAlbumToHome(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.homeSections = testSections(12)
	it := &a.homeSections[6].Items[0]
	a.playlists[it.List.ID] = &spotify.Playlist{ID: it.List.ID, Name: it.List.Name}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Wheel(700, 400, 0, 2400)
	tt.Advance(time.Second)
	tt.Frame()
	left := a.pageScroll.Y
	// A section's title in view, to see the page itself comes back there.
	title := ""
	var before ui.Rect
	for i := range a.homeSections {
		s := a.homeSections[i].Title
		if r, ok := tt.Find(s); ok && r.Y > 100 && r.Y < 600 {
			title, before = s, r
			break
		}
	}
	if left == 0 || title == "" {
		t.Fatalf("scrolled to %v, section title in view %q", left, title)
	}

	a.openItem(it)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	if a.route.page != pagePlaylist {
		t.Fatalf("the card opened %+v", a.route)
	}

	tt.Click("返回")
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	after, _ := tt.Find(title)
	if a.pageScroll.Y != left || after.Y != before.Y {
		t.Errorf("home at %v with %s at %v, was at %v with it at %v", a.pageScroll.Y, title, after.Y, left, before.Y)
	}
}
