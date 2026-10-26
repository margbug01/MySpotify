package app

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"vinyl/internal/spotify"
)

// What the home page showed shows again on the next start, until signing
// out forgets it.
func TestHomeSnapshotShowsAgain(t *testing.T) {
	file := filepath.Join(t.TempDir(), "home.json")
	a := newTestApp()
	a.homeFile = file
	a.profile.Email = "listener@example.com"
	a.homeSections = testSections(1)
	a.homeShortcuts = a.homeSections[0].Items[:1]
	a.charts = []spotify.Playlist{{ID: "spotify:playlist:c", Preview: []spotify.ChartLine{{First: "歌", Second: "人"}}}}
	a.daily = []spotify.Track{{ID: "t1", Name: "一首"}}
	a.mine = []spotify.Playlist{{ID: spotify.LikedURI, Name: "我喜欢的音乐"}}
	a.followed = []spotify.ArtistCard{{ID: "spotify:artist:a", Name: "艺人"}}
	a.keepHome()

	b := newTestApp()
	b.profile = nil
	b.homeFile = file
	b.restoreHome()
	if b.profile == nil || b.profile.UserID != "listener" || b.profile.Email != "listener@example.com" {
		t.Fatalf("profile = %+v", b.profile)
	}
	if len(b.homeSections) != 1 || len(b.homeSections[0].Items) != 6 || len(b.homeShortcuts) != 1 || len(b.charts) != 1 || b.charts[0].Preview[0].Second != "人" ||
		len(b.daily) != 1 || len(b.mine) != 1 || len(b.followed) != 1 {
		t.Fatalf("restored %+v %+v %+v %+v %+v", b.homeSections, b.charts, b.daily, b.mine, b.followed)
	}

	b.profile = nil
	b.keepHome()
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("signed out, home.json stays: %v", err)
	}
}

// Signing in as the one restoreHome showed keeps what shows, and the
// email its picture comes from, until they load again; as someone else,
// what was theirs goes.
func TestSignedInKeepsWhatShowedForTheSameListener(t *testing.T) {
	for _, tc := range []struct {
		user string
		keep bool
	}{{"listener", true}, {"someone-else", false}} {
		a := newTestApp()
		a.restored = true // no session.json to read
		a.profile.Email = "listener@example.com"
		a.homeSections = testSections(1)
		a.mine = []spotify.Playlist{{ID: spotify.LikedURI}}
		a.daily = []spotify.Track{{ID: "t1"}}
		a.signedIn(&spotify.Profile{UserID: tc.user})
		kept := len(a.homeSections) == 1 && len(a.mine) == 1 && len(a.daily) == 1
		if kept != tc.keep {
			t.Errorf("%s: kept = %v, want %v", tc.user, kept, tc.keep)
		}
		if email := a.profile.Email != ""; email != tc.keep {
			t.Errorf("%s: email kept = %v, want %v", tc.user, email, tc.keep)
		}
	}
}

// The grid over the sections shows what was played lately, then the liked
// songs and the listener's lists, topped up from the sections, each once,
// eight at most.
func TestGridShowsLatelyPlayedThenTopsUp(t *testing.T) {
	a := newTestApp()
	a.homeSections = testSections(2)
	a.homeShortcuts = []spotify.HomeItem{a.homeSections[1].Items[0]}
	a.mine = []spotify.Playlist{{ID: spotify.LikedURI, Name: "我喜欢的音乐"}}
	var got []string
	for _, it := range a.gridItems() {
		got = append(got, it.ID())
	}
	want := []string{"spotify:playlist:1-0", spotify.LikedURI,
		"spotify:playlist:0-0", "spotify:playlist:0-1", "spotify:playlist:0-2", "spotify:playlist:0-3", "spotify:playlist:0-4", "spotify:playlist:0-5"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("grid\n got %v\nwant %v", got, want)
	}
}

// An episode's card says when it came out, and how long it is.
func TestEpisodeNote(t *testing.T) {
	a := newTestApp()
	now := time.Date(2026, 10, 26, 17, 0, 0, 0, time.Local)
	for _, tc := range []struct {
		released string
		ms       int64
		want     string
	}{
		{"2026-10-26", 1285596, "10月26日 · 21 分钟"},
		{"2025-03-01", 3900000, "2025年3月1日 · 1 小时 5 分钟"},
		{"", 20000, ""},
	} {
		it := spotify.HomeItem{Episode: &spotify.Track{Episode: true, Duration: tc.ms}, Released: tc.released}
		if got := a.itemNote(&it, now); got != tc.want {
			t.Errorf("%s %d: %q, want %q", tc.released, tc.ms, got, tc.want)
		}
	}
}

// A chip loads its home once; chosen again, it goes back to all of it.
func TestChipChosenAgainShowsAll(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.homeSections = testSections(3)
	homeFeed = func(string) (*spotify.Home, error) { return &spotify.Home{}, nil }
	defer func() { homeFeed = spotify.HomeFeed }()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("播客"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.homeFacet != "podcasts-chip" || !a.loading["home:podcasts-chip"] {
		t.Fatalf("facet %q, loading %v", a.homeFacet, a.loading)
	}
	tt.Advance(time.Second)
	if err := tt.Click("播客"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.homeFacet != "" {
		t.Errorf("chosen again, facet = %q", a.homeFacet)
	}
	if !tt.HasText("分区 0") {
		t.Error("all of the home does not show again")
	}
}
