package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"vinyl/internal/spotify"
)

// A song Spotify plays another of shows the heart of the one liked, and
// unliking it takes that one out of the library: the heart was empty.
func TestRelinkedSongKeepsItsHeart(t *testing.T) {
	defer func(old func(string, bool) error) { likeSong = old }(likeSong)
	a := newTestApp()
	a.liked = map[string]bool{"asked": true}
	tr := spotify.Track{ID: "played", LinkedFrom: "asked", Name: "Wrong World"}
	tt := ui.NewTester(func(c *ui.Context) { a.likeButton(c, &tr, 22, ui.RGB(0, 0, 0)) }, 100, 100)
	tt.Frame()
	if _, ok := tt.Find("取消喜欢"); !ok {
		t.Fatal("heart shows the song as not liked")
	}
	sent := make(chan string, 1)
	likeSong = func(id string, like bool) error {
		if like {
			t.Error("asked to like it")
		}
		sent <- id
		return nil
	}
	a.toggleLike(&tr)
	if a.liked["asked"] {
		t.Error("still liked")
	}
	select {
	case id := <-sent:
		if id != "asked" {
			t.Errorf("unliked %q", id)
		}
	case <-time.After(time.Second):
		t.Fatal("not unliked with Spotify")
	}
}
