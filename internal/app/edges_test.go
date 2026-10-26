package app

import (
	"bytes"
	"image"
	"image/draw"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"vinyl/internal/netease"
)

func TestScrollEdgesFollowContent(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	for i := range 40 {
		a.featured = append(a.featured, netease.Playlist{ID: int64(i + 1), Name: "歌单"})
	}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if a.underTop {
		t.Error("top edge shows before scrolling")
	}
	if !a.underBottom {
		t.Error("no bottom edge over a long page")
	}
	tt.Scroll(800, 400, 0, 100000)
	tt.Advance(time.Second)
	tt.Frame()
	tt.Frame()
	if !a.underTop {
		t.Error("no top edge after scrolling")
	}
	if a.underBottom {
		t.Error("bottom edge shows at the end of the page")
	}
}

func TestShortPageHasNoEdges(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.featured = []netease.Playlist{{ID: 1, Name: "歌单"}}
	tt := ui.NewTester(a.view, 1200, 1400)
	tt.Frame()
	if a.underTop || a.underBottom {
		t.Errorf("edges on a page that fits: top %v bottom %v", a.underTop, a.underBottom)
	}
}

// Opening or closing the account menu leaves the player's edge as it was:
// the content's box had no key, so the menu coming or going made it anew,
// and the edge faded in again from nothing, showing the page bare under
// the player until something asked for another frame.
func TestAccountMenuKeepsPlayerEdge(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.profile = &netease.Profile{UserID: 1, Nickname: "小明"}
	for i := range 40 {
		a.featured = append(a.featured, netease.Playlist{ID: int64(i + 1), Name: "歌单"})
	}
	tt := ui.NewTester(a.view, 1000, 700)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	// The player's foot, right of where the menu's shadow reaches.
	bar := func() *image.RGBA {
		img := tt.Image()
		s := float32(img.Bounds().Dx()) / 1000
		r := image.Rect(int((sidebarWidth+60)*s), int((700-playerBarHeight)*s), img.Bounds().Dx(), img.Bounds().Dy())
		out := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
		draw.Draw(out, out.Bounds(), img, r.Min, draw.Src)
		return out
	}
	want := bar()
	for _, step := range []string{"open", "close"} {
		if err := tt.Click("小明"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if !bytes.Equal(bar().Pix, want.Pix) {
			t.Fatalf("the player's edge changed on %s", step)
		}
	}
}
