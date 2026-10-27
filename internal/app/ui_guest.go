package app

import (
	"vinyl/internal/spotify"
)

// Signed out, the pages are the visitor's pages: browsing, search and the
// public lists all work, and nothing plays. Spotify plays nothing to an
// anonymous identity, so a play button asks for a sign-in instead of
// failing at the CDN a moment later, out of sight.
//
// Every play goes through these, so that one rule holds everywhere.

// playQueue plays tracks from start, or asks for a sign-in first if nobody
// is signed in. It reports whether it played.
func (a *app) playQueue(tracks []spotify.Track, start int) bool {
	if !a.canPlay() {
		return false
	}
	a.pl.PlayQueue(tracks, start)
	return true
}

// playRadio plays the radio of name, from tracks, as playQueue does.
func (a *app) playRadio(name string, tracks []spotify.Track) bool {
	if !a.canPlay() {
		return false
	}
	a.pl.PlayRadio(name, tracks, a.radioFeed())
	return true
}

// askSignIn asks the listener to sign in, as the play buttons do signed
// out. Tests replace it, as the login window wants one; it is set in init,
// as the way in runs back through the pages that play.
var askSignIn func(*app)

func init() { askSignIn = signInForPlayback }

func signInForPlayback(a *app) {
	a.toast = "登录 Spotify 就能听歌，免费账号也能完整播放"
	a.openLogin()
}

// canPlay reports whether anyone is signed in to play for, asking for a
// sign-in if not.
func (a *app) canPlay() bool {
	if a.profile != nil {
		return true
	}
	askSignIn(a)
	return false
}
