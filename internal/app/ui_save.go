package app

import (
	"slices"
	"strings"

	"vinyl/internal/spotify"
)

// followPlaylist keeps a playlist in the listener's library or not; tests
// replace it. Albums and podcasts go through saveToLibrary.
var followPlaylist = spotify.FollowPlaylist

// isSaved reports whether the listener keeps the album, playlist or
// podcast id in their library, as the sidebar lists it.
func (a *app) isSaved(id string) bool {
	return slices.ContainsFunc(a.saved, func(p spotify.Playlist) bool { return p.ID == id })
}

// toggleSaved keeps pl in the listener's library, or takes it out: at once
// in the sidebar, then with Spotify, which may refuse: it goes back then.
func (a *app) toggleSaved(pl *spotify.Playlist) {
	if a.profile == nil {
		a.showMain()
		a.openLogin()
		return
	}
	save := !a.isSaved(pl.ID)
	uri, user := pl.ID, a.profile.UserID
	kept := *pl
	kept.Tracks = nil
	if save {
		a.saved = append([]spotify.Playlist{kept}, a.saved...)
	} else {
		a.saved = slices.DeleteFunc(slices.Clone(a.saved), func(p spotify.Playlist) bool { return p.ID == uri })
	}
	go func() {
		var err error
		if strings.HasPrefix(uri, "spotify:playlist:") {
			err = followPlaylist(user, uri, save)
		} else {
			err = saveToLibrary(uri, save)
		}
		a.post(func() {
			if err != nil {
				if save {
					a.saved = slices.DeleteFunc(slices.Clone(a.saved), func(p spotify.Playlist) bool { return p.ID == uri })
				} else if !a.isSaved(uri) {
					a.saved = append([]spotify.Playlist{kept}, a.saved...)
				}
				a.toast = "操作失败，请稍后再试"
				return
			}
			if save {
				a.toast = "已收藏"
			} else {
				a.toast = "已取消收藏"
			}
		})
	}()
}
