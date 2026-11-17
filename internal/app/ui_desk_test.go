package app

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"vinyl/internal/lyrics"
)

// The line sung is held through a short pause but not a long break, and is
// none before the first line or once the last has ended, for the desktop
// lyrics and the subtitles of music videos alike.
func TestSungLine(t *testing.T) {
	s := time.Second
	lines := []lyrics.Line{
		{At: 2 * s, End: 5 * s, Text: "一"},
		{At: 6 * s, End: 9 * s, Text: "二"},
		{At: 20 * s, End: 23 * s, Text: "三"},
	}
	for _, c := range []struct {
		at   time.Duration
		want int
	}{{1 * s, -1}, {3 * s, 0}, {5500 * time.Millisecond, 0}, {9500 * time.Millisecond, 1}, {12 * s, -1}, {21 * s, 2}, {25 * s, -1}} {
		if got := sungLine(lines, c.at); got != c.want {
			t.Errorf("at %v: line %d, want %d", c.at, got, c.want)
		}
	}
}

// Under the line shows its translation, or the next line without one, as
// the listener chose; or always the next line; or nothing.
func TestDeskUnder(t *testing.T) {
	lines := []lyrics.Line{{Text: "一", Translation: "one"}, {Text: "二"}}
	for _, c := range []struct {
		mode string
		cur  int
		want string
	}{{"", 0, "one"}, {"", 1, ""}, {"next", 0, "二"}, {"none", 0, ""}} {
		if got := deskUnder(lines, c.cur, c.mode); got != c.want {
			t.Errorf("%q under line %d: %q, want %q", c.mode, c.cur, got, c.want)
		}
	}
	lines[0].Translation = ""
	if got := deskUnder(lines, 0, ""); got != "二" {
		t.Errorf("no translation: %q, want the next line", got)
	}
}

// The settings choose the lyrics' size, the color of words sung and what
// shows under the line, and the lyrics take the color at once.
func TestDeskSettings(t *testing.T) {
	if !deskHere {
		t.Skip("no desktop lyrics here")
	}
	a := newTestApp()
	a.settings = defaultSettings()
	a.open(route{page: pageSettings})
	tt := ui.NewTester(a.view, 1200, 2000)
	tt.Frame()
	for _, label := range []string{"大", "蓝色", "下一句"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		tt.Advance(300 * time.Millisecond)
		tt.Frame()
	}
	s := a.settings
	if s.DeskSize != "large" || s.DeskColor != "blue" || s.DeskUnder != "next" {
		t.Fatalf("size %q, color %q, under %q", s.DeskSize, s.DeskColor, s.DeskUnder)
	}
	if got := (&desk{a: a}).tint(); got != deskColors[5].color {
		t.Errorf("words sung in %v, not the blue chosen", got)
	}
	if err := tt.Click("跟随封面"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.settings.DeskColor != "" {
		t.Errorf("color %q, not the cover's", a.settings.DeskColor)
	}
}

// deskApp is an app playing a song with lyrics and its desktop lyrics, the
// pointer over them.
func deskApp(t *testing.T) (*app, *desk, *ui.Tester) {
	if !deskHere {
		t.Skip("no desktop lyrics here")
	}
	a, _, _ := playingApp()
	a.settings.DeskLyrics = true
	d := &desk{a: a, hover: true}
	a.desk = d
	tt := ui.NewTester(d.view, deskW, deskH)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	return a, d, tt
}

// The pointer over the lyrics brings up the lock, which leaves only the key
// that unlocks them, and the button that closes them.
func TestDeskLockAndClose(t *testing.T) {
	a, _, tt := deskApp(t)
	if err := tt.Click("锁定桌面歌词"); err != nil {
		t.Fatal(err)
	}
	if !a.settings.DeskLocked {
		t.Fatal("not locked")
	}
	tt.Advance(time.Second)
	tt.Frame()
	if err := tt.Click("关闭桌面歌词"); err == nil {
		t.Fatal("locked, the controls still take clicks")
	}
	if err := tt.Click("解锁桌面歌词"); err != nil {
		t.Fatal(err)
	}
	if a.settings.DeskLocked {
		t.Fatal("not unlocked")
	}
	tt.Advance(time.Second)
	tt.Frame()
	if err := tt.Click("关闭桌面歌词"); err != nil {
		t.Fatal(err)
	}
	if a.settings.DeskLyrics {
		t.Fatal("not closed")
	}
}

// The player bar's button shows the desktop lyrics and hides them.
func TestDeskBarButton(t *testing.T) {
	if !deskHere {
		t.Skip("no desktop lyrics here")
	}
	a, tt, _ := playingApp()
	tt.Frame()
	for _, want := range []bool{true, false} {
		if err := tt.Click("桌面歌词"); err != nil {
			t.Fatal(err)
		}
		if a.settings.DeskLyrics != want {
			t.Fatalf("shown %v, want %v", a.settings.DeskLyrics, want)
		}
		tt.Frame()
	}
}

// The desktop lyrics ask for frames at the display's rate only as a line
// rises in: not once it has, its words lighting up on timers, nor between
// lines. They float over every app all day.
func TestDeskFrameBudget(t *testing.T) {
	a, d, _ := deskApp(t)
	d.hover = false
	tt := ui.NewTester(d.view, deskW, deskH)
	if n := framesAt(a, tt, 12*time.Second); n > 1 {
		t.Errorf("a line sung: %d frames a second", n)
	}
	if n := framesAt(a, tt, 18*time.Second); n > 1 {
		t.Errorf("between lines: %d frames a second", n)
	}
}

// With NM_FRAMES set, saves the desktop lyrics over a light and a dark
// desktop: a line half sung, under the pointer, locked under it, and the
// song's name between lines.
func TestDeskFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	a, d, _ := deskApp(t)
	lines, _ := a.pl.Lyrics()
	lines[1].Translation = "A translation under the line"
	save := func(name string, pos time.Duration) {
		a.pl.Seek(pos)
		tt := ui.NewTester(d.view, deskW, deskH)
		tt.Frame()
		tt.Advance(time.Second)
		tt.Frame()
		img := image.NewRGBA(image.Rect(0, 0, deskW, deskH*2))
		draw.Draw(img, image.Rect(0, 0, deskW, deskH), image.NewUniform(color.RGBA{236, 232, 224, 255}), image.Point{}, draw.Src)
		draw.Draw(img, image.Rect(0, deskH, deskW, deskH*2), image.NewUniform(color.RGBA{28, 34, 48, 255}), image.Point{}, draw.Src)
		for _, top := range []int{0, deskH} {
			draw.Draw(img, image.Rect(0, top, deskW, top+deskH), tt.Image(), image.Point{}, draw.Over)
		}
		f, err := os.Create(filepath.Join(dir, "desk-"+name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		png.Encode(f, img)
		f.Close()
	}
	d.hover = false
	save("sung", 12500*time.Millisecond)
	d.hover = true
	save("hover", 12500*time.Millisecond)
	a.settings.DeskLocked = true
	save("locked", 12500*time.Millisecond)
	d.hover = false
	save("break", 18*time.Second)
	a.settings.DeskLocked = false
	a.settings.DeskSize, a.settings.DeskColor = "large", "white"
	lines[1].Translation = "A translation long enough to have to shrink to fit on its one line, rather than end in dots"
	save("large-white-long", 12500*time.Millisecond)
	a.settings.DeskSize, a.settings.DeskColor, a.settings.DeskUnder = "small", "pink", "next"
	save("small-pink-next", 12500*time.Millisecond)

	a.open(route{page: pageSettings})
	tt := ui.NewTester(a.view, 1000, 1000)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	f, err := os.Create(filepath.Join(dir, "desk-settings.png"))
	if err != nil {
		t.Fatal(err)
	}
	png.Encode(f, tt.Image())
	f.Close()
}
