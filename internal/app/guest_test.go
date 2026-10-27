package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"vinyl/internal/spotify"
)

// Signed out, playing asks for a sign-in rather than calling on Spotify: it
// would refuse, and the refusal would come out of sight.
func TestPlaySignedOutAsksForASignIn(t *testing.T) {
	var asked int
	askSignIn = func(a *app) { asked++; a.toast = "登录" }
	defer func() {
		askSignIn = func(a *app) { a.toast = "登录 Spotify 就能听歌，免费账号也能完整播放"; a.openLogin() }
	}()

	a := newTestApp()
	a.settings = defaultSettings()
	a.profile = nil
	songs := []spotify.Track{{ID: "1", Name: "晴天", Duration: 200000}}

	if a.playQueue(songs, 0) {
		t.Error("played with nobody signed in")
	}
	if a.pl.Playing() || a.pl.Queue() != nil {
		t.Error("the queue took songs anyway")
	}
	if asked != 1 {
		t.Errorf("asked %d times to sign in, want once", asked)
	}

	// Signed in, the same call plays.
	a.profile = &spotify.Profile{UserID: "1"}
	if !a.playQueue(songs, 0) {
		t.Error("a signed-in listener could not play")
	}
	if asked != 1 {
		t.Errorf("asked to sign in while signed in")
	}
}

// Signed out, the pages everyone's are still open: the way in does not
// stand in front of the home page unless even the visitor's failed.
func TestSignedOutBrowsingStays(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.profile = nil
	a.homeSections = testSections(2)
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if !tt.HasText("分区 0") {
		t.Fatal("the home page is gone signed out; the sign-in page stands in")
	}
	if err := tt.Click("搜索"); err == nil {
		tt.Frame()
		if !tt.HasText("搜索") {
			t.Error("search does not open signed out")
		}
	}
}
