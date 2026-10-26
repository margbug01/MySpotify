package app

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"vinyl/internal/spotify"
)

// The listener's playlists in the tests.
const (
	pl10 = "spotify:playlist:10"
	pl11 = "spotify:playlist:11"
	pl12 = "spotify:playlist:12"
	pl20 = "spotify:playlist:20"
)

// editTestApp is signed in, with liked songs, two playlists of their own
// and one saved, and shows their playlist 10.
func editTestApp() *app {
	a := newTestApp()
	a.settings = defaultSettings()
	a.profile = &spotify.Profile{UserID: "1", Nickname: "小明"}
	a.mine = []spotify.Playlist{
		{ID: spotify.LikedURI, Name: "我喜欢的音乐"},
		{ID: pl10, Name: "通勤"},
		{ID: pl11, Name: "睡前"},
	}
	a.saved = []spotify.Playlist{{ID: pl20, Name: "飙升榜"}}
	a.playlists[pl10] = &spotify.Playlist{ID: pl10, Name: "通勤", Tracks: []spotify.Track{
		{ID: "1", UID: "u1", Name: "一张相片", Artists: []spotify.Artist{{Name: "林宥嘉"}}, Duration: 200000},
		{ID: "2", UID: "u2", Name: "宝船", Artists: []spotify.Artist{{Name: "DOUDOU"}}, Duration: 200000},
	}}
	a.route = route{page: pagePlaylist, playlist: pl10}
	return a
}

// Choosing one of the listener's playlists from a song's menu adds the
// song to it, and the menu offers neither the liked songs nor the
// playlist the song is listed in.
func TestAddSongToPlaylist(t *testing.T) {
	type add struct{ pid, song string }
	calls := make(chan add, 1)
	addToPlaylist = func(pid string, songs ...string) error { calls <- add{pid, songs[0]}; return nil }
	defer func() { addToPlaylist = spotify.AddToPlaylist }()

	a := editTestApp()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.RightClick("宝船"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("添加到歌单", "通勤"); err == nil {
		t.Error("offered to add the song to the playlist it is in")
	}
	if err := tt.ChooseMenuItem("添加到歌单", "我喜欢的音乐"); err == nil {
		t.Error("offered to add the song to the liked songs")
	}
	if err := tt.ChooseMenuItem("添加到歌单", "睡前"); err != nil {
		t.Fatalf("%v; the menu shows %q", err, tt.Menu())
	}
	tt.Frame()
	if got := <-calls; got != (add{pl11, "2"}) {
		t.Errorf("added %+v, want song 2 to playlist 11", got)
	}
}

// Removing a song from the listener's playlist takes its row away at once,
// and asks Spotify to take out its place in the list.
func TestRemoveSongFromPlaylist(t *testing.T) {
	type del struct{ pid, uid string }
	calls := make(chan del, 1)
	removeFromPlaylist = func(pid string, uids ...string) error { calls <- del{pid, uids[0]}; return nil }
	defer func() { removeFromPlaylist = spotify.RemoveFromPlaylist }()

	a := editTestApp()
	// Playing the playlist: the queue keeps the song.
	a.pl.Cue(a.playlists[pl10].Tracks)
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.RightClick("宝船"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("从歌单中移除"); err != nil {
		t.Fatalf("%v; the menu shows %q", err, tt.Menu())
	}
	tt.Frame()
	if got := <-calls; got != (del{pl10, "u2"}) {
		t.Errorf("removed %+v, want song 2 (u2) from playlist 10", got)
	}
	if _, ok := tt.Find("宝船"); ok {
		t.Error("the song's row is still listed")
	}
	if q := a.pl.Queue(); len(q) != 2 || q[1].ID != "2" {
		t.Errorf("removing the song changed the queue playing: %v", q)
	}
}

// A playlist the listener saved, not made, offers no removing.
func TestNoRemovingFromSavedPlaylist(t *testing.T) {
	a := editTestApp()
	a.playlists[pl20] = &spotify.Playlist{ID: pl20, Name: "飙升榜", Tracks: a.playlists[pl10].Tracks}
	a.route = route{page: pagePlaylist, playlist: pl20}
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
	createPlaylist = func(user, name string) (*spotify.Playlist, error) {
		names <- user + ":" + name
		return &spotify.Playlist{ID: pl12, Name: name}, nil
	}
	defer func() { createPlaylist = spotify.CreatePlaylist }()

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
	if got := <-names; got != "1:周末" {
		t.Errorf("made %q, want listener 1's 周末", got)
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
	if !a.naming.open || a.naming.add == nil || a.naming.add.ID != "2" {
		t.Fatalf("the dialog is %+v, want it open to add song 2", a.naming)
	}
}

// Renaming a playlist from the sidebar's menu shows its new name at once,
// and asks Spotify to rename it.
func TestRenamePlaylist(t *testing.T) {
	type rename struct{ id, name string }
	calls := make(chan rename, 1)
	renamePlaylist = func(id, name string) error { calls <- rename{id, name}; return nil }
	defer func() { renamePlaylist = spotify.RenamePlaylist }()

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
	if got := <-calls; got != (rename{pl11, "助眠"}) {
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
	tt.RightClick("我喜欢的音乐")
	if err := tt.ChooseMenuItem("重命名…"); err == nil {
		t.Error("offered to rename the liked songs")
	}
}

// Deleting the playlist shown, once confirmed, takes it from the sidebar
// and goes back to the page before, with no way forward to it.
func TestDeletePlaylist(t *testing.T) {
	ids := make(chan string, 1)
	deletePlaylist = func(user, id string) error { ids <- id; return nil }
	defer func() { deletePlaylist = spotify.DeletePlaylist }()

	a := editTestApp()
	a.homeLoading = true
	a.route = route{page: pageCharts}
	a.open(route{page: pagePlaylist, playlist: pl10})
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
	if got := <-ids; got != pl10 {
		t.Errorf("deleted playlist %s, want 10", got)
	}
	if a.editable(pl10) != nil {
		t.Error("the sidebar still lists it")
	}
	if a.route.page != pageCharts || len(a.history) != 0 || len(a.forward) != 0 {
		t.Errorf("on %+v, history %v, forward %v; want the charts with nowhere to go", a.route, a.history, a.forward)
	}
}

// Cancelling the confirmation deletes nothing.
func TestCancelDeletingPlaylist(t *testing.T) {
	deletePlaylist = func(string, string) error { t.Error("deleted the playlist"); return nil }
	defer func() { deletePlaylist = spotify.DeletePlaylist }()

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
	if a.editable(pl11) == nil {
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
		a.startRename(a.editable(pl11))
		save("rename" + suffix)
		a.naming.open = false
		a.confirmDelete(a.editable(pl11))
		save("delete" + suffix)
		a.deleting.open = false
	}
}

// An album, or a playlist someone else made, is saved from its page and
// shows in the sidebar at once; saved, it is taken out again from its page
// or from the sidebar's menu.
func TestSaveAlbumAndPlaylist(t *testing.T) {
	type call struct {
		uri  string
		save bool
	}
	calls := make(chan call, 4)
	saveToLibrary = func(uri string, save bool) error { calls <- call{uri, save}; return nil }
	followPlaylist = func(user, uri string, follow bool) error { calls <- call{user + ":" + uri, follow}; return nil }
	defer func() { saveToLibrary, followPlaylist = spotify.Save, spotify.FollowPlaylist }()
	next := func() call {
		select {
		case c := <-calls:
			return c
		case <-time.After(2 * time.Second):
			t.Fatal("Spotify not asked")
			return call{}
		}
	}

	const album = "spotify:album:9"
	a := editTestApp()
	a.playlists[album] = &spotify.Playlist{ID: album, Name: "大小说家", Tracks: a.playlists[pl10].Tracks}
	a.route = route{page: pagePlaylist, playlist: album}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("收藏"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := next(); got != (call{album, true}) || !a.isSaved(album) {
		t.Fatalf("saved %+v, in the sidebar %v", got, a.isSaved(album))
	}
	if err := tt.Click("已收藏"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := next(); got != (call{album, false}) || a.isSaved(album) {
		t.Fatalf("unsaved %+v, in the sidebar %v", got, a.isSaved(album))
	}

	// The listener's own playlist offers no saving.
	a.route = route{page: pagePlaylist, playlist: pl10}
	tt.Frame()
	if _, ok := tt.Find("收藏"); ok {
		t.Error("offered to save the listener's own playlist")
	}
	if err := tt.RightClick("飙升榜"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("取消收藏"); err != nil {
		t.Fatalf("%v; the menu shows %q", err, tt.Menu())
	}
	tt.Frame()
	if got := next(); got != (call{"1:" + pl20, false}) || a.isSaved(pl20) {
		t.Fatalf("from the sidebar %+v, still listed %v", got, a.isSaved(pl20))
	}
}
