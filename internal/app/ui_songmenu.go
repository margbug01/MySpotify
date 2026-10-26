package app

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"slices"

	"vinyl/internal/spotify"
)

// copyText puts text on the clipboard; tests replace it.
var copyText = func(text string) error { return mygo.Clipboard.Write(transfer.TextData(text)) }

// songMenu is three dots that open the menu of tr.
func (a *app) songMenu(c *ui.Context, tr *spotify.Track, size float32, color ui.Color) ui.Element {
	b := iconButton(c, iconMore, size, color, "更多")
	b.Menu(a.songItems(tr))
	return b
}

// songItems builds the menu of what can be done with tr, behind its dots
// or a right click on its row: copying its title, its title with its
// artists, or the link to its page.
func (a *app) songItems(tr *spotify.Track) func(m *ui.Menu) {
	return func(m *ui.Menu) {
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
		if tr.Album.ID != "" || slices.ContainsFunc(tr.Artists, func(ar spotify.Artist) bool { return ar.ID != "" }) {
			m.Separator()
		}
		for _, ar := range tr.Artists {
			if ar.ID != "" && m.Item("查看歌手："+ar.Name).Chosen() {
				a.openArtist(ar.ID)
			}
		}
		if tr.Album.ID != "" && m.Item("查看专辑："+tr.Album.Name).Chosen() {
			a.goTo(route{page: pagePlaylist, playlist: tr.Album.ID})
		}
	}
}
