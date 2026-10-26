package app

import (
	"vinyl/internal/netease"
	"vinyl/internal/player"
)

func newTestApp() *app {
	a := &app{playlists: map[int64]*netease.Playlist{}, loadingPl: map[int64]bool{}, plErr: map[int64]string{}, lyricAt: -1, queueAt: -1}
	a.art = newArtCache(func(func()) {})
	a.pl = player.New(func(func()) {})
	a.pl.SetVolume(0.5)
	return a
}
