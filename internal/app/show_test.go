package app

import (
	"testing"

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
		Album: spotify.Album{ID: show, Name: "相声精选"}, Duration: 2000000}
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
