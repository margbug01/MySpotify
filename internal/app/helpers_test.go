package app

import (
	"fmt"

	"vinyl/internal/player"
	"vinyl/internal/spotify"
)

func newTestApp() *app {
	a := &app{playlists: map[string]*spotify.Playlist{}, loadingPl: map[string]bool{}, plErr: map[string]string{}, lyricAt: -1, queueAt: -1,
		artists: map[string]*spotify.ArtistPage{}, loadingAr: map[string]bool{}, arErr: map[string]string{}, loadingRel: map[string]bool{}}
	// Signed in: signed out, every page asks to sign in.
	a.profile = &spotify.Profile{UserID: "listener", Nickname: "听众"}
	a.art = newArtCache(func(func()) {})
	a.pl = player.New(func(func()) {})
	a.pl.Leads = a.mvLead
	a.pl.SetVolume(0.5)
	return a
}

// testSections returns n sections of the home, six lists each.
func testSections(n int) []spotify.HomeSection {
	var out []spotify.HomeSection
	for i := range n {
		s := spotify.HomeSection{URI: fmt.Sprintf("spotify:section:%d", i), Title: fmt.Sprintf("分区 %d", i), Total: 6}
		for j := range 6 {
			pl := spotify.Playlist{ID: fmt.Sprintf("spotify:playlist:%d-%d", i, j), Name: "歌单"}
			s.Items = append(s.Items, spotify.HomeItem{List: &pl})
		}
		out = append(out, s)
	}
	return out
}
