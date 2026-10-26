package app

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"vinyl/internal/netease"
)

// skeletonScenes are the views that load, as they show while they do.
var skeletonScenes = []struct {
	name string
	set  func(a *app)
}{
	{"home", func(a *app) {}},
	{"charts", func(a *app) { a.route = route{page: pageCharts} }},
	{"daily", func(a *app) { a.route = route{page: pageDaily} }},
	{"playlist", func(a *app) {
		a.route = route{page: pagePlaylist, playlist: 7}
		a.loadingPl[7] = true
	}},
	{"search", func(a *app) {
		a.route = route{page: pageSearch}
		a.searched, a.searching = "春日影", true
	}},
	{"login", func(a *app) { a.login = loginState{open: true} }},
	{"comments", func(a *app) {
		a.nowPlaying, a.npPane = true, paneComments
		a.comments = commentState{track: 1, loading: true}
	}},
	{"more-comments", func(a *app) {
		a.nowPlaying, a.npPane = true, paneComments
		a.comments = commentState{track: 1, loading: true, loaded: true, total: 1, latest: []netease.Comment{{ID: 1, Content: "好听"}}}
	}},
	{"lyrics", func(a *app) { a.nowPlaying, a.npPane = true, paneLyrics }},
	{"stage", func(a *app) { a.nowPlaying, a.npPane = true, paneStage }},
}

func skeletonApp(set func(a *app)) *app {
	a := newTestApp()
	a.settings = defaultSettings()
	a.homeLoading = true
	a.pl.Cue([]netease.Track{{ID: 1, Name: "春日影"}})
	set(a)
	return a
}

// With NM_FRAMES set, saves each skeleton in the light and the dark, the
// shimmer partway across.
func TestSkeletonFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	for _, s := range skeletonScenes {
		for _, dark := range []bool{false, true} {
			a := skeletonApp(s.set)
			tt := ui.NewTester(a.view, 1200, 800)
			tt.SetDark(dark)
			tt.Advance(450 * time.Millisecond)
			tt.Frame()
			name := s.name
			if dark {
				name += "-dark"
			}
			f, _ := os.Create(filepath.Join(dir, "skeleton-"+name+".png"))
			png.Encode(f, tt.Image())
			f.Close()
		}
	}
}
