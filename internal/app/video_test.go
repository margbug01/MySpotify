package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"vinyl/internal/lyrics"
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

// Filling Now Playing, the music video shows the line sung over it, above
// the controls and lower once they go, letting the pointer through to the
// video; the next line takes its place as it comes. The listener turns
// the subtitles off and on.
func TestMVShowsTheLyricsAsSubtitles(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.pl.Cue([]spotify.Track{{ID: "1", Name: "APT.", Duration: 170000}})
	a.pl.SetLyrics([]lyrics.Line{
		{At: 10 * time.Second, End: 14 * time.Second, Text: "Hold on, hold on", Translation: "等等，等等"},
		{At: 15 * time.Second, End: 18 * time.Second, Text: "I'm on my way"},
	})
	a.pl.ShowVideo(&mygo.WebView{})
	a.pl.Seek(11 * time.Second)
	a.mvMode = mvTheater
	tt := ui.NewTester(a.view, 1200, 800)
	a.nowPlaying = true
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	line := func(text string) (ui.Rect, bool) {
		t.Helper()
		tt.Advance(300 * time.Millisecond)
		tt.Frame()
		return tt.Find(text)
	}
	up, ok := line("Hold on, hold on")
	if !ok {
		t.Fatal("no subtitle over the video")
	}
	if _, ok := tt.Find("等等，等等"); !ok {
		t.Fatal("no translation under it")
	}
	// Moving over the subtitle, the pointer moves over the video: the
	// controls stay.
	tt.Move(600, 300)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	tt.Move(up.X+up.W/2, up.Y+3)
	tt.Frame()
	tt.Advance(mvIdle - time.Second/2)
	tt.Frame()
	if r, _ := line("Hold on, hold on"); r.Y != up.Y {
		t.Fatalf("moved over the subtitle, the controls went: it is at %v, was %v", r, up)
	}
	tt.Advance(mvIdle + time.Second)
	tt.Frame()
	down, ok := line("Hold on, hold on")
	if !ok || down.Y <= up.Y || tt.Cursor() != ui.CursorNone {
		t.Fatalf("with the controls gone, the subtitle is at %v, was %v; the pointer %v", down, up, tt.Cursor())
	}
	a.pl.Seek(15500 * time.Millisecond)
	if _, ok := line("I'm on my way"); !ok {
		t.Fatal("the next line did not come")
	}
	if _, ok := tt.Find("Hold on, hold on"); ok {
		t.Fatal("the line before stays")
	}
	tt.Move(600, 300)
	tt.Frame()
	if err := tt.Click("隐藏歌词"); err != nil {
		t.Fatal(err)
	}
	if _, ok := line("I'm on my way"); ok || a.settings.MVLyrics {
		t.Fatal("the subtitles stay, turned off")
	}
	if err := tt.Click("显示歌词"); err != nil {
		t.Fatal(err)
	}
	if _, ok := line("I'm on my way"); !ok {
		t.Fatal("the subtitles did not come back")
	}
	a.pl.Seek(20 * time.Second)
	if _, ok := line("I'm on my way"); ok {
		t.Fatal("the last line stays after it was sung")
	}
}

// Whether the music video fills Now Playing is kept, for the next time the
// app opens; full screen is not, the way before it is.
func TestMVModeIsKept(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	a.pl.Cue([]spotify.Track{{ID: "1", Name: "APT.", Duration: 170000}})
	a.pl.SetLyrics(nil)
	a.pl.ShowVideo(&mygo.WebView{})
	tt := ui.NewTester(a.view, 1200, 800)
	a.nowPlaying = true
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	saved := func() int {
		t.Helper()
		s := defaultSettings()
		b, err := os.ReadFile(a.settings.path)
		if err != nil {
			t.Fatal(err)
		}
		json.Unmarshal(b, &s)
		return (&app{settings: s}).mvModeChosen()
	}
	r, _ := tt.Find("MV 画面")
	tt.Move(r.X+r.W/2, r.Y+r.H/2)
	tt.Frame()
	if err := tt.Click("铺满窗口"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if m := saved(); m != mvTheater {
		t.Fatalf("kept %d, want filling Now Playing", m)
	}
	tt.Key(0, ui.KeyF)
	tt.Frame()
	if a.mvMode != mvFullScreen || saved() != mvTheater {
		t.Fatalf("full screen: mode %d, kept %d", a.mvMode, saved())
	}
	tt.Key(0, ui.KeyEscape)
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.mvMode != mvInCover || saved() != mvInCover {
		t.Fatalf("back by the lyrics: mode %d, kept %d", a.mvMode, saved())
	}
	if defaultSettings().MVLyrics != true {
		t.Fatal("subtitles are off at first")
	}
}

// The lyrics over a music video come later or earlier with ] and [, and
// so they stay for the song.
func TestMVLyricsCanBeNudged(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	a.pl.Cue([]spotify.Track{{ID: "1", Name: "APT.", Duration: 170000}})
	a.pl.SetLyrics([]lyrics.Line{
		{At: 10 * time.Second, End: 14 * time.Second, Text: "Hold on, hold on"},
		{At: 15 * time.Second, End: 18 * time.Second, Text: "I'm on my way"},
	})
	a.pl.ShowVideo(&mygo.WebView{})
	a.pl.Seek(15200 * time.Millisecond)
	a.mvMode = mvTheater
	tt := ui.NewTester(a.view, 1200, 800)
	a.nowPlaying = true
	frame := func() {
		tt.Advance(300 * time.Millisecond)
		tt.Frame()
	}
	frame()
	if _, ok := tt.Find("I'm on my way"); !ok {
		t.Fatal("no subtitle")
	}
	tt.Key(0, ui.KeyBracketRight)
	frame()
	if _, ok := tt.Find("Hold on, hold on"); !ok || a.pl.VideoLead() != 500*time.Millisecond {
		t.Fatalf("]: the lyrics did not come later, ahead by %v", a.pl.VideoLead())
	}
	tt.Move(600, 300)
	frame()
	if err := tt.Click("歌词延后 0.5 秒 (])"); err != nil {
		t.Fatal(err)
	}
	frame()
	b, err := os.ReadFile(a.settings.path)
	if err != nil {
		t.Fatal(err)
	}
	var kept settings
	json.Unmarshal(b, &kept)
	if kept.MVLeads["1"] != 1000 || a.pl.VideoLead() != time.Second {
		t.Fatalf("kept %v, ahead by %v", kept.MVLeads, a.pl.VideoLead())
	}
	for range 2 {
		tt.Key(0, ui.KeyBracketLeft)
	}
	frame()
	if _, ok := tt.Find("I'm on my way"); !ok || a.pl.VideoLead() != 0 {
		t.Fatalf("[: the lyrics did not come back, ahead by %v", a.pl.VideoLead())
	}
}
