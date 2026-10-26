package app

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"vinyl/internal/spotify"
)

// The listener's own playlists: making them, renaming them, deleting them,
// and adding songs to them or taking songs out.

// These edit playlists with Spotify; tests replace them.
var (
	createPlaylist     = spotify.CreatePlaylist
	renamePlaylist     = spotify.RenamePlaylist
	addToPlaylist      = spotify.AddToPlaylist
	removeFromPlaylist = spotify.RemoveFromPlaylist
	deletePlaylist     = spotify.DeletePlaylist
)

// nameDialog asks for a playlist's name: a new one's, or one's new name.
type nameDialog struct {
	open   bool
	name   string
	rename string         // the playlist renamed, or "" to make one
	add    *spotify.Track // the song to add to the playlist made, if any
	busy   bool           // Spotify is making the playlist
}

// deleteDialog asks whether to delete one of the listener's playlists.
type deleteDialog struct {
	open bool
	id   string
	name string
}

// editable returns the listener's playlist id if they may edit its songs
// and name, or nil: the liked songs' are their hearts'.
func (a *app) editable(id string) *spotify.Playlist {
	for i := range a.mine {
		if a.mine[i].ID == id && !a.mine[i].Liked() {
			return &a.mine[i]
		}
	}
	return nil
}

// playlistChanged forgets what was loaded of playlist id, which Spotify
// changed: loaded anew if shown, and the sidebar's covers with it.
func (a *app) playlistChanged(id string) {
	delete(a.playlists, id)
	if a.route.page == pagePlaylist && a.route.playlist == id {
		a.loadPlaylist(id, false)
	}
	if a.profile != nil {
		a.loadMine(a.profile.UserID)
	}
}

// newPlaylist asks for the name of a playlist to make, then adds tr to it
// if set. Signed out, it asks the listener to sign in.
func (a *app) newPlaylist(tr *spotify.Track) {
	if a.profile == nil {
		a.showMain()
		a.openLogin()
		return
	}
	a.naming = nameDialog{open: true, add: tr}
}

// startRename asks for a new name for the listener's playlist pl.
func (a *app) startRename(pl *spotify.Playlist) {
	a.naming = nameDialog{open: true, name: pl.Name, rename: pl.ID}
}

// submitName makes the playlist named, or renames one: at once in the app,
// as Spotify may refuse, when renaming.
func (a *app) submitName() {
	d := &a.naming
	name := strings.TrimSpace(d.name)
	if name == "" || d.busy || a.profile == nil {
		return
	}
	if d.rename != "" {
		id := d.rename
		d.open = false
		pl := a.editable(id)
		if pl == nil || pl.Name == name {
			return
		}
		old := pl.Name
		a.setPlaylistName(id, name)
		go func() {
			err := renamePlaylist(id, name)
			a.post(func() {
				if err != nil {
					a.setPlaylistName(id, old)
					a.toast = "重命名失败，请稍后再试"
				}
			})
		}()
		return
	}
	d.busy = true
	add, user := d.add, a.profile.UserID
	go func() {
		pl, err := createPlaylist(user, name)
		a.post(func() {
			a.naming.busy = false
			if err != nil {
				a.toast = "创建歌单失败，请稍后再试"
				return
			}
			a.naming.open = false
			// Under the liked songs, as Spotify lists the newest first.
			at := 0
			if len(a.mine) > 0 && a.mine[0].Liked() {
				at = 1
			}
			a.mine = slices.Insert(a.mine, at, *pl)
			pl.Tracks = nil
			a.playlists[pl.ID] = pl
			a.open(route{page: pagePlaylist, playlist: pl.ID})
			if add != nil {
				a.addTo(pl.ID, add)
			}
		})
	}()
}

// confirmDelete asks whether to delete the listener's playlist pl.
func (a *app) confirmDelete(pl *spotify.Playlist) {
	a.deleting = deleteDialog{open: true, id: pl.ID, name: pl.Name}
}

// deleteChosen deletes the playlist the dialog asked about: at once from
// the sidebar, leaving its page, and then with Spotify, which may refuse:
// it comes back then.
func (a *app) deleteChosen() {
	id := a.deleting.id
	a.deleting.open = false
	i := slices.IndexFunc(a.mine, func(pl spotify.Playlist) bool { return pl.ID == id })
	if i < 0 || a.mine[i].Liked() || a.profile == nil {
		return
	}
	pl, user := a.mine[i], a.profile.UserID
	a.mine = slices.Delete(slices.Clone(a.mine), i, i+1)
	// Leave no way back to its page, and leave it for the page before.
	gone := func(r route) bool { return r.page == pagePlaylist && r.playlist == id }
	a.history = slices.DeleteFunc(a.history, gone)
	a.forward = slices.DeleteFunc(a.forward, gone)
	if gone(a.route) {
		r := route{page: pageHome}
		if n := len(a.history); n > 0 {
			r, a.history = a.history[n-1], a.history[:n-1]
		}
		a.show(r)
	}
	go func() {
		err := deletePlaylist(user, id)
		a.post(func() {
			if err != nil {
				a.mine = slices.Insert(a.mine, min(i, len(a.mine)), pl)
				a.toast = "删除失败，请稍后再试"
				return
			}
			delete(a.playlists, id)
			a.toast = "已删除歌单"
		})
	}()
}

func (a *app) setPlaylistName(id, name string) {
	if pl := a.editable(id); pl != nil {
		pl.Name = name
	}
	if pl := a.playlists[id]; pl != nil {
		pl.Name = name
	}
}

// addTo adds tr to the listener's playlist pid; a toast says how it went.
// Spotify would list it twice: one there already is not added again.
func (a *app) addTo(pid string, tr *spotify.Track) {
	if pl := a.playlists[pid]; pl != nil && slices.ContainsFunc(pl.Tracks, func(t spotify.Track) bool { return t.ID == tr.ID }) {
		a.toast = "歌曲已在歌单中"
		return
	}
	id := tr.ID
	go func() {
		err := addToPlaylist(pid, id)
		a.post(func() {
			if err != nil {
				a.toast = "添加失败，请稍后再试"
				return
			}
			a.toast = "已添加到歌单"
			a.playlistChanged(pid)
		})
	}()
}

// removeFrom takes tr, as listed, out of the listener's playlist pid, at
// once in the app and then with Spotify, which may refuse: the song comes
// back then.
func (a *app) removeFrom(pid string, tr *spotify.Track) {
	uid := tr.UID
	pl := a.playlists[pid]
	var before []spotify.Track
	if pl != nil {
		// A new list: the player may be playing the old one.
		before = pl.Tracks
		pl.Tracks = slices.DeleteFunc(slices.Clone(before), func(t spotify.Track) bool { return t.UID == uid })
	}
	go func() {
		err := removeFromPlaylist(pid, uid)
		a.post(func() {
			if err != nil {
				if pl != nil && a.playlists[pid] == pl {
					pl.Tracks = before
				}
				a.toast = "移除失败，请稍后再试"
				return
			}
			// The songs listed are right: only the sidebar's covers may
			// have changed.
			if a.profile != nil {
				a.loadMine(a.profile.UserID)
			}
		})
	}()
}

// playlistItems are the items of a song's menu that put it in the
// listener's playlists, or take it out of pid, the one it is listed in, if
// set.
func (a *app) playlistItems(m *ui.Menu, tr *spotify.Track, pid string) {
	if a.profile == nil {
		return
	}
	m.Separator()
	m.Submenu("添加到歌单", func(m *ui.Menu) {
		if m.Item("新建歌单…").Chosen() {
			a.newPlaylist(tr)
		}
		var to []*spotify.Playlist
		for i := range a.mine {
			if pl := &a.mine[i]; !pl.Liked() && pl.ID != pid {
				to = append(to, pl)
			}
		}
		if len(to) > 0 {
			m.Separator()
		}
		for _, pl := range to {
			if m.Item(pl.Name).Chosen() {
				a.addTo(pl.ID, tr)
			}
		}
	})
	if tr.UID != "" && a.editable(pid) != nil && m.Item("从歌单中移除").Chosen() {
		a.removeFrom(pid, tr)
	}
}

// playlistNameDialog asks for the name of a playlist to make, or a new
// name for one.
func (a *app) playlistNameDialog(c *ui.Context) {
	p := a.pal
	d := &a.naming
	ui.DialogBase(c.Key("playlist-name"), &d.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.35))
		panel.Width(360).Padding(24).Radius(18).Gap(16).
			Background(p.bg).Border(1, p.hairline).Shadow(0, 24, 60, -12, ui.RGBA(0, 0, 0, 0.4)).
			Transition(ui.ElementTransition{Animation: &ui.Snappy, Enter: &ui.Motion{Scale: 0.94}, Exit: &ui.Motion{Scale: 0.96}})
		title, action := "新建歌单", "创建"
		if d.rename != "" {
			title, action = "重命名歌单", "保存"
		}
		if d.busy {
			action = "正在创建…"
		}
		ui.Text(c, title).FontSize(17).FontWeight(700)
		in := ui.TextInput(c.Key("name"), &d.name).Placeholder("歌单名称").Label("歌单名称").FontSize(14).AutoFocus()
		if in.Submitted() {
			a.submitName()
		}
		ui.Row(c).Gap(10).Justify(ui.End).Children(func() {
			if pillButton(c.Key("cancel"), nil, "取消", p.hover.Over(p.bg), p.text).Clicked() {
				d.open = false
			}
			ok := pillButton(c.Key("ok"), nil, action, p.accent, p.onAccent)
			if strings.TrimSpace(d.name) == "" {
				ok.Disabled(true).Opacity(0.5)
			}
			if ok.Clicked() {
				a.submitName()
			}
		})
	})
}

// deletePlaylistDialog asks whether to delete a playlist.
func (a *app) deletePlaylistDialog(c *ui.Context) {
	p := a.pal
	d := &a.deleting
	ui.DialogBase(c.Key("playlist-delete"), &d.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.35))
		panel.Width(360).Padding(24).Radius(18).Gap(8).
			Background(p.bg).Border(1, p.hairline).Shadow(0, 24, 60, -12, ui.RGBA(0, 0, 0, 0.4)).
			Transition(ui.ElementTransition{Animation: &ui.Snappy, Enter: &ui.Motion{Scale: 0.94}, Exit: &ui.Motion{Scale: 0.96}})
		ui.Text(c, "删除歌单").FontSize(17).FontWeight(700)
		ui.Text(c, "确定删除歌单“"+d.name+"”吗？它会从你的资料库里移除。").FontSize(13).TextColor(p.muted).LineHeight(1.45)
		ui.Row(c).Gap(10).Justify(ui.End).Margin(8, 0, 0, 0).Children(func() {
			if pillButton(c.Key("cancel"), nil, "取消", p.hover.Over(p.bg), p.text).Clicked() {
				d.open = false
			}
			if pillButton(c.Key("ok"), nil, "删除", p.accent, p.onAccent).Clicked() {
				a.deleteChosen()
			}
		})
	})
}
