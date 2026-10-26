package app

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// editTestApp is signed in, with liked songs, two playlists of their own
// and one saved, and shows their playlist 10.
func editTestApp() *app {
	a := newTestApp()
	a.settings = defaultSettings()
	a.profile = &netease.Profile{UserID: 1, Nickname: "小明"}
	a.mine = []netease.Playlist{
		{ID: 5, Name: "小明喜欢的音乐", SpecialType: 5},
		{ID: 10, Name: "通勤"},
		{ID: 11, Name: "睡前"},
	}
	a.saved = []netease.Playlist{{ID: 20, Name: "飙升榜"}}
	a.playlists[10] = &netease.Playlist{ID: 10, Name: "通勤", Tracks: []netease.Track{
		{ID: 1, Name: "一张相片", Artists: []netease.Artist{{Name: "林宥嘉"}}, Duration: 200000},
		{ID: 2, Name: "宝船", Artists: []netease.Artist{{Name: "DOUDOU"}}, Duration: 200000},
	}}
	a.route = route{page: pagePlaylist, playlist: 10}
	return a
}

// Choosing one of the listener's playlists from a song's menu adds the
// song to it, and the menu offers neither the liked songs nor the
// playlist the song is listed in.
func TestAddSongToPlaylist(t *testing.T) {
	type add struct{ pid, song int64 }
	calls := make(chan add, 1)
	addToPlaylist = func(pid int64, songs ...int64) error { calls <- add{pid, songs[0]}; return nil }
	defer func() { addToPlaylist = netease.AddToPlaylist }()

	a := editTestApp()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.RightClick("宝船"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("添加到歌单", "通勤"); err == nil {
		t.Error("offered to add the song to the playlist it is in")
	}
	if err := tt.ChooseMenuItem("添加到歌单", "小明喜欢的音乐"); err == nil {
		t.Error("offered to add the song to the liked songs")
	}
	if err := tt.ChooseMenuItem("添加到歌单", "睡前"); err != nil {
		t.Fatalf("%v; the menu shows %q", err, tt.Menu())
	}
	tt.Frame()
	if got := <-calls; got != (add{11, 2}) {
		t.Errorf("added %+v, want song 2 to playlist 11", got)
	}
}

// Removing a song from the listener's playlist takes its row away at once,
// and asks NetEase to take it out.
func TestRemoveSongFromPlaylist(t *testing.T) {
	type del struct{ pid, song int64 }
	calls := make(chan del, 1)
	removeFromPlaylist = func(pid int64, songs ...int64) error { calls <- del{pid, songs[0]}; return nil }
	defer func() { removeFromPlaylist = netease.RemoveFromPlaylist }()

	a := editTestApp()
	// Playing the playlist: the queue keeps the song.
	a.pl.Cue(a.playlists[10].Tracks)
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.RightClick("宝船"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("从歌单中移除"); err != nil {
		t.Fatalf("%v; the menu shows %q", err, tt.Menu())
	}
	tt.Frame()
	if got := <-calls; got != (del{10, 2}) {
		t.Errorf("removed %+v, want song 2 from playlist 10", got)
	}
	if _, ok := tt.Find("宝船"); ok {
		t.Error("the song's row is still listed")
	}
	if q := a.pl.Queue(); len(q) != 2 || q[1].ID != 2 {
		t.Errorf("removing the song changed the queue playing: %v", q)
	}
}

// A playlist the listener saved, not made, offers no removing.
func TestNoRemovingFromSavedPlaylist(t *testing.T) {
	a := editTestApp()
	a.playlists[20] = &netease.Playlist{ID: 20, Name: "飙升榜", Tracks: a.playlists[10].Tracks}
	a.route = route{page: pagePlaylist, playlist: 20}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.RightClick("宝船"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("从歌单中移除"); err == nil {
		t.Error("offered to remove a song from a saved playlist")
	}
}

// The sidebar's plus asks for a name, and Enter makes the playlist so
// named.
func TestCreatePlaylist(t *testing.T) {
	names := make(chan string, 1)
	createPlaylist = func(name string) (*netease.Playlist, error) {
		names <- name
		return &netease.Playlist{ID: 12, Name: name}, nil
	}
	defer func() { createPlaylist = netease.CreatePlaylist }()

	a := editTestApp()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("新建歌单"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(300 * time.Millisecond)
	tt.Frame()
	tt.Type("  周末  ")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if got := <-names; got != "周末" {
		t.Errorf("made a playlist named %q, want 周末", got)
	}
}

// Choosing 新建歌单… from a song's menu makes the playlist, then adds the
// song to it.
func TestCreatePlaylistForSong(t *testing.T) {
	a := editTestApp()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.RightClick("宝船"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("添加到歌单", "新建歌单…"); err != nil {
		t.Fatalf("%v; the menu shows %q", err, tt.Menu())
	}
	tt.Frame()
	if !a.naming.open || a.naming.add == nil || a.naming.add.ID != 2 {
		t.Fatalf("the dialog is %+v, want it open to add song 2", a.naming)
	}
}

// Renaming a playlist from the sidebar's menu shows its new name at once,
// and asks NetEase to rename it.
func TestRenamePlaylist(t *testing.T) {
	type rename struct {
		id   int64
		name string
	}
	calls := make(chan rename, 1)
	renamePlaylist = func(id int64, name string) error { calls <- rename{id, name}; return nil }
	defer func() { renamePlaylist = netease.RenamePlaylist }()

	a := editTestApp()
	a.route = route{page: pageHome}
	a.homeLoading = true
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.RightClick("睡前"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("重命名…"); err != nil {
		t.Fatalf("%v; the menu shows %q", err, tt.Menu())
	}
	tt.Advance(300 * time.Millisecond)
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("助眠")
	if err := tt.Click("保存"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := <-calls; got != (rename{11, "助眠"}) {
		t.Errorf("renamed %+v, want playlist 11 to 助眠", got)
	}
	if a.mine[2].Name != "助眠" {
		t.Errorf("the sidebar names it %q", a.mine[2].Name)
	}
	if _, ok := tt.Find("助眠"); !ok {
		t.Error("the new name does not show")
	}
}

// The liked songs can't be renamed: they offer no menu.
func TestNoRenamingLikedSongs(t *testing.T) {
	a := editTestApp()
	a.route = route{page: pageHome}
	a.homeLoading = true
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	tt.RightClick("小明喜欢的音乐")
	if err := tt.ChooseMenuItem("重命名…"); err == nil {
		t.Error("offered to rename the liked songs")
	}
}

// Deleting the playlist shown, once confirmed, takes it from the sidebar
// and goes back to the page before, with no way forward to it.
func TestDeletePlaylist(t *testing.T) {
	ids := make(chan int64, 1)
	deletePlaylist = func(id int64) error { ids <- id; return nil }
	defer func() { deletePlaylist = netease.DeletePlaylist }()

	a := editTestApp()
	a.homeLoading = true
	a.route = route{page: pageCharts}
	a.open(route{page: pagePlaylist, playlist: 10})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("更多"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("删除歌单…"); err != nil {
		t.Fatalf("%v; the menu shows %q", err, tt.Menu())
	}
	tt.Advance(300 * time.Millisecond)
	tt.Frame()
	select {
	case <-ids:
		t.Fatal("deleted before it was confirmed")
	default:
	}
	if err := tt.Click("删除"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := <-ids; got != 10 {
		t.Errorf("deleted playlist %d, want 10", got)
	}
	if a.editable(10) != nil {
		t.Error("the sidebar still lists it")
	}
	if a.route.page != pageCharts || len(a.history) != 0 || len(a.forward) != 0 {
		t.Errorf("on %+v, history %v, forward %v; want the charts with nowhere to go", a.route, a.history, a.forward)
	}
}

// Cancelling the confirmation deletes nothing.
func TestCancelDeletingPlaylist(t *testing.T) {
	deletePlaylist = func(int64) error { t.Error("deleted the playlist"); return nil }
	defer func() { deletePlaylist = netease.DeletePlaylist }()

	a := editTestApp()
	a.route = route{page: pageHome}
	a.homeLoading = true
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.RightClick("睡前"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("删除歌单…"); err != nil {
		t.Fatalf("%v; the menu shows %q", err, tt.Menu())
	}
	tt.Advance(300 * time.Millisecond)
	tt.Frame()
	if err := tt.Click("取消"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(300 * time.Millisecond)
	tt.Frame()
	if a.editable(11) == nil {
		t.Error("the playlist left the sidebar")
	}
}

// With NM_FRAMES set, saves the listener's playlist with its sidebar, and
// the dialogs that name and delete playlists, to look at.
func TestPlaylistEditFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	a := editTestApp()
	tt := ui.NewTester(a.view, 1200, 800)
	save := func(name string) {
		tt.Advance(400 * time.Millisecond)
		tt.Frame()
		f, _ := os.Create(filepath.Join(dir, name+".png"))
		defer f.Close()
		png.Encode(f, tt.Image())
	}
	for _, dark := range []bool{false, true} {
		suffix := "-light"
		if dark {
			suffix = "-dark"
		}
		tt.SetDark(dark)
		save("playlist" + suffix)
		a.newPlaylist(nil)
		save("create" + suffix)
		a.naming.open = false
		a.startRename(a.editable(11))
		save("rename" + suffix)
		a.naming.open = false
		a.confirmDelete(a.editable(11))
		save("delete" + suffix)
		a.deleting.open = false
	}
}
