package app

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"vinyl/internal/spotify"
)

// flatGenie has the genie run in a Tester, drawing its content unwarped, as the
// CPU draws the shader slowly.
func flatGenie(t *testing.T) {
	draw := genieDraw
	genieOnCPU = true
	genieDraw = func(p *ui.Painter, box ui.Rect, content *ui.Bitmap, _ ui.Point, _ float32) {
		p.ImageRegion(content, ui.Rect{}, box, ui.ImageOptions{})
	}
	t.Cleanup(func() { genieOnCPU, genieDraw = false, draw })
}

// genieApp is an app playing a song, its page showing.
func genieApp(t *testing.T, w, h int) (*app, *ui.Tester) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.homeSections = testSections(4)
	a.pl.Cue([]spotify.Track{{ID: "1", Name: "Herman's Habit", Artists: []spotify.Artist{{Name: "Justin Hurwitz"}}, Duration: 200000}})
	a.pl.Hold(0)
	tt := ui.NewTester(a.view, w, h)
	tt.Frame()
	return a, tt
}

// Now Playing funnels into where the player bar's cover really is.
func TestGenieTargetIsTheCover(t *testing.T) {
	_, tt := genieApp(t, 1180, 780)
	r, ok := tt.Find("打开正在播放")
	if !ok {
		t.Fatal("no player bar")
	}
	if r.X != genieCoverLeft {
		t.Errorf("the cover starts at %v, the genie goes to %v", r.X, float32(genieCoverLeft))
	}
	if y := 780 - float32(genieCoverRise); math.Abs(float64(r.Y+r.H/2-(y+genieCoverSize/2))) > 1 {
		t.Errorf("the cover's middle is at %v, the genie's at %v", r.Y+r.H/2, y+genieCoverSize/2)
	}
}

// Clicking the cover pours Now Playing out and closing pours it back, with
// frames asked for only on the way once it is closed. Open, its lyrics ask for
// their own.
func TestGenieRunsAndRests(t *testing.T) {
	flatGenie(t)
	a, tt := genieApp(t, 1180, 780)
	if err := tt.Click("打开正在播放"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.FramePending() {
		t.Error("opening asks for no frames")
	}
	tt.Advance(npSettle + 50*time.Millisecond)
	tt.Frame()
	if got := a.genieAmount(tt.Now()); got != 0 {
		t.Errorf("open, amount = %v", got)
	}
	a.back()
	tt.Frame()
	if !tt.FramePending() {
		t.Error("closing asks for no frames")
	}
	tt.Advance(npSettle + 50*time.Millisecond)
	tt.Frame()
	if tt.FramePending() {
		t.Error("closed and at rest, it still asks for frames")
	}
	if got := a.genieAmount(tt.Now()); got != 1 {
		t.Errorf("closed, amount = %v", got)
	}
}

// Closed halfway through opening, it goes back from where it got to, without
// a jump.
func TestGenieTurnsBackFromWhereItIs(t *testing.T) {
	a, tt := genieApp(t, 1180, 780)
	a.nowPlaying = true
	tt.Frame()
	tt.Advance(genieTime / 2)
	before := a.genieAmount(tt.Now())
	if before <= 0 || before >= 1 {
		t.Fatalf("halfway, amount = %v", before)
	}
	a.nowPlaying = false
	tt.Frame()
	if after := a.genieAmount(tt.Now()); math.Abs(float64(after-before)) > 0.001 {
		t.Errorf("turning back jumped from %v to %v", before, after)
	}
}

// fixedGenie draws an element funneled by a set amount.
type fixedGenie float32

func (g fixedGenie) EffectMargin() ui.Point { return ui.Point{} }

func (g fixedGenie) PaintEffect(p *ui.Painter, box ui.Rect, content *ui.Bitmap, inset ui.Point) {
	genieDraw(p, box, content, inset, float32(g))
}

// Nearly closed, the cover shows where the page goes in. It used to show the
// page squeezed into a dark square, which flashed as it turned into the cover.
func TestGenieEndsOnTheCover(t *testing.T) {
	const w, h = 400, 300
	coverAt := func(amount float32) uint32 {
		tt := ui.NewTester(func(c *ui.Context) {
			ui.Box(c).Fill().Background(ui.RGB(255, 255, 255)).Children(func() {
				ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Background(ui.RGB(0, 0, 0)).Effect(fixedGenie(amount))
			})
		}, w, h)
		tt.Frame()
		r, _, _, _ := tt.Image().At(genieCoverLeft+genieCoverSize/2, h-genieCoverRise+genieCoverSize/2).RGBA()
		return r >> 8
	}
	if v := coverAt(0); v > 16 {
		t.Errorf("open, the page does not cover the cover: %d", v)
	}
	if v := coverAt(0.96); v < 240 {
		t.Errorf("nearly closed, the page still covers the cover: %d", v)
	}
}

// With NM_FRAMES set, saves Now Playing on its way out of a colored cover and
// back into it.
func TestGenieFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	genieOnCPU = true
	defer func() { genieOnCPU = false }()
	a, tt := genieApp(t, 1180, 780)
	pix := image.NewRGBA(image.Rect(0, 0, 8, 8))
	draw.Draw(pix, pix.Bounds(), image.NewUniform(color.RGBA{230, 60, 90, 255}), image.Point{}, draw.Src)
	for _, key := range []string{spotify.Sized("c", 120), spotify.Sized("c", 600), spotify.Sized("c", 600) + "#backdrop"} {
		a.art.art[key] = &artwork{bmp: ui.NewBitmap(pix), tint: ui.RGB(230, 60, 90)}
	}
	a.pl.Queue()[0].Album.PicURL = "c"
	save := func(name string) {
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		png.Encode(f, tt.Image())
	}
	tt.Frame()
	a.nowPlaying = true
	tt.Frame()
	for i := range 6 {
		save(fmt.Sprintf("genie-%d.png", i))
		tt.Advance(genieTime / 5)
		tt.Frame()
	}
	tt.Advance(npSettle)
	tt.Frame()
	a.nowPlaying = false
	tt.Frame()
	for i := range 11 {
		if i >= 4 {
			save(fmt.Sprintf("close-%02d.png", i))
		}
		tt.Advance(genieTime / 10)
		tt.Frame()
	}
}
