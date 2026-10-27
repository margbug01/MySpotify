package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"vinyl/internal/spotify"
)

// Now Playing offers the music video of a song that has one, not of one
// that hasn't, and its button turns music videos on and off.
func TestMVButtonTurnsVideosOnAndOff(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.pl.Cue([]spotify.Track{{ID: "1", Name: "APT.", Duration: 170000}})
	a.pl.SetLyrics(nil)
	tt := ui.NewTester(a.view, 1200, 800)
	a.nowPlaying = true
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	if _, ok := tt.Find("播放 MV"); ok {
		t.Fatal("MV offered for a song without one")
	}
	a.pl.SetMedia(&spotify.Media{Video: "160a9d67"})
	tt.Frame()
	if err := tt.Click("播放 MV"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.pl.VideoOn() {
		t.Fatal("music videos did not turn on")
	}
	if err := tt.Click("关闭 MV"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.pl.VideoOn() {
		t.Fatal("music videos did not turn off")
	}
}

// The music video fills the window from its place by the lyrics, and the
// screen with F; Esc steps back the way it came, the last time closing Now
// Playing. Filling the window, it hides the pointer once still for a few
// seconds, which moving brings back.
func TestMVFillsTheWindowAndTheScreen(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.pl.Cue([]spotify.Track{{ID: "1", Name: "APT.", Duration: 170000}})
	a.pl.SetLyrics(nil)
	a.pl.ShowVideo(&mygo.WebView{})
	tt := ui.NewTester(a.view, 1200, 800)
	a.nowPlaying = true
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	video := func() ui.Rect {
		t.Helper()
		r, ok := tt.Find("MV 画面")
		if !ok {
			t.Fatal("no video")
		}
		return r
	}
	r := video()
	if r.W >= 1200 {
		t.Fatalf("the video fills the window before asked to: %v", r)
	}
	tt.Move(r.X+r.W/2, r.Y+r.H/2)
	tt.Frame()
	if err := tt.Click("铺满窗口"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if r := video(); r.W != 1200 || r.H != 800 {
		t.Fatalf("filling the window, the video is %v", r)
	}
	tt.Move(600, 400)
	tt.Frame()
	if tt.Cursor() == ui.CursorNone {
		t.Fatal("the pointer went as it moved")
	}
	tt.Advance(mvIdle + time.Second)
	tt.Frame()
	if tt.Cursor() != ui.CursorNone {
		t.Fatal("the pointer stays over the video, still")
	}
	tt.Move(620, 410)
	tt.Frame()
	if tt.Cursor() == ui.CursorNone {
		t.Fatal("the pointer did not come back as it moved")
	}
	tt.Key(0, ui.KeyF)
	tt.Frame()
	if a.mvMode != mvFullScreen {
		t.Fatalf("F: mode %d, want full screen", a.mvMode)
	}
	for _, want := range []int{mvTheater, mvInCover} {
		tt.Key(0, ui.KeyEscape)
		tt.Frame()
		if a.mvMode != want || !a.nowPlaying {
			t.Fatalf("Esc: mode %d, want %d; Now Playing open %v", a.mvMode, want, a.nowPlaying)
		}
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.nowPlaying {
		t.Fatal("Esc did not close Now Playing")
	}
}
