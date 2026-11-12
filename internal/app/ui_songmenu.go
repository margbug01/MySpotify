package app

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"vinyl/internal/l10n"
	"vinyl/internal/spotify"
)

// copyText puts text on the clipboard; tests replace it.
var copyText = func(text string) error { return mygo.Clipboard.Write(transfer.TextData(text)) }

// songMenu is three dots that open the menu of tr.
func (a *app) songMenu(c *ui.Context, tr *spotify.Track, size float32, color ui.Color) ui.Element {
	b := iconButton(c, iconMore, size, color, l10n.T("更多"))
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
			// With nothing playing, either plays it: no toast needed. Nor
			// when Spotify Connect could not take it and said why, such as
			// while it reconnects: saying it was queued hid that it was not.
			queued := a.pl.Current() != nil
			if m.Item(l10n.T("下一首播放")).Chosen() {
				a.pl.PlayNext(*tr)
				if queued && a.toast == "" {
					a.toast = l10n.T("将在下一首播放")
				}
			}
			if m.Item(l10n.T("添加到待播清单")).Chosen() {
				switch {
				case !a.pl.AddToQueue(*tr):
					a.toast = l10n.T("歌曲已在待播清单中")
				case queued && a.toast == "":
					a.toast = l10n.T("已添加到待播清单")
				}
			}
			if !tr.Episode && m.Item(l10n.T("开启电台")).Chosen() {
				a.startRadio(tr.URI(), tr.Name)
			}
			m.Separator()
		}
		if m.Item(l10n.T("复制歌名")).Chosen() {
			copyText(tr.Name)
		}
		if m.Item(l10n.T("复制歌名和歌手")).Chosen() {
			copyText(tr.Name + " - " + tr.ArtistNames())
		}
		m.Separator()
		if m.Item(l10n.T("复制链接")).Chosen() {
			copyText(tr.URL())
		}
		m.Separator()
		if here := (route{page: pageTrack, track: tr.ID}); !tr.Episode && a.route != here && m.Item(l10n.T("查看歌曲")).Chosen() {
			a.goTo(here)
		}
		for _, ar := range tr.Artists {
			if ar.ID != "" && m.Item(l10n.T("查看歌手：")+ar.Name).Chosen() {
				a.openArtist(ar.ID)
			}
		}
		album := l10n.T("查看专辑：")
		if tr.Episode {
			album = l10n.T("查看播客：")
		}
		if tr.Album.ID != "" && a.route != (route{page: pagePlaylist, playlist: tr.Album.ID}) && m.Item(album+tr.Album.Name).Chosen() {
			a.goTo(route{page: pagePlaylist, playlist: tr.Album.ID})
		}
		a.playlistItems(m, tr, pid)
	}
}
