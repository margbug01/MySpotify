package app

import (
	"fmt"
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

// With NM_FRAMES set, saves Now Playing on its way out of the cover.
func TestGenieFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	genieOnCPU = true
	defer func() { genieOnCPU = false }()
	a, tt := genieApp(t, 1180, 780)
	tt.Frame()
	a.nowPlaying = true
	tt.Frame()
	for i := range 6 {
		f, err := os.Create(filepath.Join(dir, fmt.Sprintf("genie-%d.png", i)))
		if err != nil {
			t.Fatal(err)
		}
		png.Encode(f, tt.Image())
		f.Close()
		tt.Advance(genieTime / 5)
		tt.Frame()
	}
}
