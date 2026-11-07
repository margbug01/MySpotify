package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"vinyl/internal/spotify"
)

// A podcast's page lists its episodes, which queue and play as songs do,
// but have no radio, page, playlist or heart of a song.
func TestPodcastEpisodes(t *testing.T) {
	const show = "spotify:show:1"
	a := newTestApp()
	a.settings = defaultSettings()
	a.mine = []spotify.Playlist{{ID: spotify.LikedURI, Name: "我喜欢的音乐"}, {ID: "spotify:playlist:10", Name: "通勤"}}
	ep := spotify.Track{ID: "e1", Episode: true, Name: "第一集", Artists: []spotify.Artist{{Name: "德云社"}},
		Album: spotify.Album{ID: show, Name: "相声精选"}, Duration: 2000000, Released: "2024-03-09"}
	a.playlists[show] = &spotify.Playlist{ID: show, Name: "相声精选", TrackCount: 300, Tracks: []spotify.Track{ep}}
	a.route = route{page: pagePlaylist, playlist: show}
	tt := ui.NewTester(a.view, 1200, 900)
	tt.Frame()
	// A song's row shows its heart under the pointer; an episode's doesn't.
	r, _ := tt.Find("第一集")
	tt.Move(r.X+4, r.Y+4)
	tt.Frame()
	if _, ok := tt.Find("喜欢"); ok {
		t.Error("an episode shows a heart")
	}
	if !tt.HasText("2024-03-09") {
		t.Errorf("the release date does not show: %q", tt.Texts())
	}
	a.pl.Cue([]spotify.Track{{ID: "8", Name: "正在放"}})
	tt.Frame()
	if !tt.HasText("300 集") {
		t.Errorf("the count shows %q", tt.Texts())
	}
	for _, item := range []string{"开启电台", "查看歌曲", "添加到歌单", "查看播客：相声精选"} {
		if err := tt.RightClick("第一集"); err != nil {
			t.Fatal(err)
		}
		if err := tt.ChooseMenuItem(item); err == nil {
			t.Errorf("an episode's menu offers %s", item)
		}
		tt.Key(0, ui.KeyEscape)
		tt.Frame()
	}
	if err := tt.RightClick("第一集"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("下一首播放"); err != nil {
		t.Fatalf("%v; the menu shows %q", err, tt.Menu())
	}
	tt.Frame()
	if q := a.pl.Queue(); len(q) != 2 || !q[1].Episode {
		t.Fatalf("queue %+v", q)
	}
}

// An episode's release date shows as its weekday in the current calendar
// week, mm-dd in the current year, and yyyy-mm-dd in earlier years.
func TestEpisodeDate(t *testing.T) {
	// 2026-11-07 is a Saturday; the week's Monday is 2026-11-02.
	now := time.Date(2026, 11, 7, 9, 0, 0, 0, time.Local)
	for _, tc := range []struct {
		released string
		want     string
	}{
		{"2026-11-07", "星期六"},
		{"2026-11-02", "星期一"},
		{"2026-11-01", "11-01"},
		{"2026-03-05", "03-05"},
		{"2025-12-31", "2025-12-31"},
		{"", ""},
	} {
		if got := episodeDate(tc.released, now); got != tc.want {
			t.Errorf("episodeDate(%q) = %q, want %q", tc.released, got, tc.want)
		}
	}
}
