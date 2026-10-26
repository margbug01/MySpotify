package app

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"vinyl/internal/spotify"
)

// copyText puts text on the clipboard; tests replace it.
var copyText = func(text string) error { return mygo.Clipboard.Write(transfer.TextData(text)) }

// songMenu is three dots that open the menu of tr.
func (a *app) songMenu(c *ui.Context, tr *spotify.Track, size float32, color ui.Color) ui.Element {
	b := iconButton(c, iconMore, size, color, "更多")
	b.Menu(a.songItems(tr, ""))
	return b
}

// songItems builds the menu of what can be done with tr, behind its dots
// or a right click on its row: playing it next or queueing it; copying its
// title, its title with its artists, or the link to its page; going to its
// artists or album; and, signed in, adding it to a playlist or taking it
// out of pid, the playlist it is listed in, if set.
func (a *app) songItems(tr *spotify.Track, pid string) func(m *ui.Menu) {
	return func(m *ui.Menu) {
		if tr.Playable() {
			// With nothing playing, either plays it: no toast needed.
			queued := a.pl.Current() != nil
			if m.Item("下一首播放").Chosen() {
				a.pl.PlayNext(*tr)
				if queued {
					a.toast = "将在下一首播放"
				}
			}
			if m.Item("添加到待播清单").Chosen() {
				switch {
				case !a.pl.AddToQueue(*tr):
					a.toast = "歌曲已在待播清单中"
				case queued:
					a.toast = "已添加到待播清单"
				}
			}
			if m.Item("开启电台").Chosen() {
				a.startRadio(tr.URI(), tr.Name)
			}
			m.Separator()
		}
		if m.Item("复制歌名").Chosen() {
			copyText(tr.Name)
		}
		if m.Item("复制歌名和歌手").Chosen() {
			copyText(tr.Name + " - " + tr.ArtistNames())
		}
		m.Separator()
		if m.Item("复制链接").Chosen() {
			copyText(tr.URL())
		}
		m.Separator()
		if here := (route{page: pageTrack, track: tr.ID}); a.route != here && m.Item("查看歌曲").Chosen() {
			a.goTo(here)
		}
		for _, ar := range tr.Artists {
			if ar.ID != "" && m.Item("查看歌手："+ar.Name).Chosen() {
				a.openArtist(ar.ID)
			}
		}
		if tr.Album.ID != "" && m.Item("查看专辑："+tr.Album.Name).Chosen() {
			a.goTo(route{page: pagePlaylist, playlist: tr.Album.ID})
		}
		a.playlistItems(m, tr, pid)
	}
}
