package app

import (
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
	a.pl.SetVolume(0.5)
	return a
}
