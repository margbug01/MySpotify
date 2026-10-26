package app

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"vinyl/internal/spotify"
)

func TestSongMenuCopies(t *testing.T) {
	var copied string
	real := copyText
	copyText = func(s string) error { copied = s; return nil }
	defer func() { copyText = real }()

	a := newTestApp()
	a.settings = defaultSettings()
	tr := spotify.Track{ID: "1859245776", Name: "宝船", Artists: []spotify.Artist{{Name: "DOUDOU"}, {Name: "小明"}}, Duration: 200000}
	a.pl.Cue([]spotify.Track{tr})
	a.pl.SetLyrics(nil)
	a.nowPlaying = true
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Advance(time.Second)
	tt.Frame()

	for _, c := range []struct{ item, want string }{
		{"复制歌名", "宝船"},
		{"复制歌名和歌手", "宝船 - DOUDOU, 小明"},
		{"复制链接", "https://open.spotify.com/track/1859245776"},
	} {
		if err := tt.Click("更多"); err != nil {
			t.Fatal(err)
		}
		if err := tt.ChooseMenuItem(c.item); err != nil {
			t.Fatalf("%v; the menu shows %q", err, tt.Menu())
		}
		tt.Frame()
		if copied != c.want {
			t.Errorf("%s copied %q, want %q", c.item, copied, c.want)
		}
	}
}

func TestSongMenuOnRightClick(t *testing.T) {
	var copied string
	real := copyText
	copyText = func(s string) error { copied = s; return nil }
	defer func() { copyText = real }()

	a := newTestApp()
	a.settings = defaultSettings()
	a.playlists["42"] = &spotify.Playlist{ID: "42", Name: "飙升榜", Tracks: []spotify.Track{
		{ID: "1", Name: "一张相片", Artists: []spotify.Artist{{Name: "林宥嘉"}}, Duration: 200000},
		{ID: "2", Name: "宝船", Artists: []spotify.Artist{{Name: "DOUDOU"}}, Duration: 200000},
	}}
	a.route = route{page: pagePlaylist, playlist: "42"}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()

	if err := tt.RightClick("宝船"); err != nil {
		t.Fatal(err)
	}
	if got, want := tt.Menu(), []string{"下一首播放", "添加到待播清单", "开启电台", "-", "复制歌名", "复制歌名和歌手", "-", "复制链接", "-", "添加到歌单"}; !slices.Equal(got, want) {
		t.Fatalf("the menu shows %q, want %q", got, want)
	}
	if err := tt.ChooseMenuItem("复制链接"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if want := "https://open.spotify.com/track/2"; copied != want {
		t.Errorf("copied %q, want %q", copied, want)
	}
	if a.pl.Current() != nil {
		t.Error("the right click played the song")
	}
}

// A song's menu plays it after the one playing, or queues it at the end.
func TestSongMenuQueues(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.pl.Cue([]spotify.Track{{ID: "8", Name: "正在放"}, {ID: "9", Name: "接着放"}})
	a.playlists["42"] = &spotify.Playlist{ID: "42", Name: "飙升榜", Tracks: []spotify.Track{
		{ID: "1", Name: "一张相片", Artists: []spotify.Artist{{Name: "林宥嘉"}}, Duration: 200000},
		{ID: "2", Name: "宝船", Artists: []spotify.Artist{{Name: "DOUDOU"}}, Duration: 200000},
	}}
	a.route = route{page: pagePlaylist, playlist: "42"}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	for _, c := range []struct{ song, item string }{{"宝船", "添加到待播清单"}, {"一张相片", "下一首播放"}} {
		if err := tt.RightClick(c.song); err != nil {
			t.Fatal(err)
		}
		if err := tt.ChooseMenuItem(c.item); err != nil {
			t.Fatalf("%v; the menu shows %q", err, tt.Menu())
		}
		tt.Frame()
	}
	order, at := a.pl.PlayOrder()
	var got []string
	for _, k := range order[at:] {
		got = append(got, a.pl.Queue()[k].Name)
	}
	if want := []string{"正在放", "一张相片", "接着放", "宝船"}; !slices.Equal(got, want) {
		t.Errorf("plays %v, want %v", got, want)
	}
}
