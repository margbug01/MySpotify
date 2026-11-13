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

	"vinyl/internal/player"
	"vinyl/internal/spotify"
)

// The drift eases in as the music plays and glides to a stop as it
// pauses; the bass swells and settles.
func TestBackdropDrifts(t *testing.T) {
	var bd backdropCache
	now := time.Unix(0, 0)
	var quiet [player.Bands]float32
	for range 120 {
		now = now.Add(backdropFrame)
		bd.step(now, true, quiet, true)
	}
	if bd.speed != 1 || bd.drift <= 0 {
		t.Fatalf("playing: speed %v, drift %v", bd.speed, bd.drift)
	}
	var loud [player.Bands]float32
	for i := range loud {
		loud[i] = 1
	}
	now = now.Add(backdropFrame)
	bd.step(now, true, loud, true)
	if bd.level.group(0, 6) <= 0.2 {
		t.Errorf("bass %v on a beat", bd.level.group(0, 6))
	}
	moving := true
	for range 120 {
		now = now.Add(backdropFrame)
		moving = bd.step(now, false, quiet, false)
	}
	if moving || bd.speed != 0 {
		t.Errorf("paused: still moving, speed %v", bd.speed)
	}
	at := bd.drift
	now = now.Add(time.Second)
	bd.step(now, false, quiet, false)
	if bd.drift != at {
		t.Error("drifted while paused")
	}
}

// paintCover returns a 120×120 cover that fill colors pixel by pixel.
func paintCover(fill func(x, y int) color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 120, 120))
	for y := range 120 {
		for x := range 120 {
			img.SetRGBA(x, y, fill(x, y))
		}
	}
	return img
}

// A portrait in black on white stays gray. It used to turn the backdrop
// brown: HSV's saturation runs high in the dark, so the warm black of a
// suit and a face or two outweighed the white wall.
func TestPaletteGrayPortrait(t *testing.T) {
	img := paintCover(func(x, y int) color.RGBA {
		switch {
		case x >= 40 && x < 60 && y >= 30 && y < 50:
			return color.RGBA{200, 150, 120, 255} // a face
		case y < 60:
			return color.RGBA{245, 244, 241, 255}
		}
		return color.RGBA{44, 24, 19, 255}
	})
	pal := paletteOf(img)
	for _, c := range []ui.Color{pal.c1, pal.c2, pal.c3} {
		deep, fog := fogColors(c, pal)
		for _, d := range []ui.Color{deep, fog} {
			// The old backdrop was 0.6 at least; a gray cover's, 0.15 or so.
			if _, s, _ := hsv(float64(d.R)/255, float64(d.G)/255, float64(d.B)/255); s > 0.2 {
				t.Errorf("palette %+v: backdrop %v, saturation %.2f", pal, d, s)
			}
		}
	}
}

// A cover of two colors keeps both, each where it sits on the cover.
func TestPaletteTwoColors(t *testing.T) {
	img := paintCover(func(x, y int) color.RGBA {
		if x < 70 {
			return color.RGBA{200, 30, 140, 255}
		}
		return color.RGBA{30, 60, 200, 255}
	})
	pal := paletteOf(img)
	hue := func(c ui.Color) float64 {
		h, _, _ := hsv(float64(c.R)/255, float64(c.G)/255, float64(c.B)/255)
		return h
	}
	if h := hue(pal.c1); h < 300 || h > 340 || pal.p1[0] > 0.5 {
		t.Errorf("first color %v (hue %.0f) at %v, want the magenta at the left", pal.c1, h, pal.p1)
	}
	if h := hue(pal.c2); h < 210 || h > 240 || pal.p2[0] < 0.5 {
		t.Errorf("second color %v (hue %.0f) at %v, want the blue at the right", pal.c2, h, pal.p2)
	}
}

// With NM_FRAMES set, saves the fog of a few covers as it drifts,
// and on a beat, one row a cover.
func TestBackdropDriftFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	const w, h = 300, 420
	var rows []*image.RGBA
	for _, q := range []string{"春日影 MyGO", "晴天 周杰伦", "夜に駆ける"} {
		hits, err := spotify.Search(q, 1)
		if err != nil || len(hits) == 0 {
			t.Fatal(q, err)
		}
		img, err := fetchImage(spotify.Sized(hits[0].Album.PicURL, 600))
		if err != nil {
			t.Fatal(err)
		}
		pal := paletteOf(img)
		shots := []struct{ drift, bass float32 }{{0, 0}, {2, 0}, {4, 0}, {4, 0.9}}
		row := image.NewRGBA(image.Rect(0, 0, w*len(shots), h))
		for i, s := range shots {
			tt := ui.NewTester(func(c *ui.Context) {
				ui.Box(c).Fill().Draw(func(p *ui.Painter, r ui.Rect) { drawFog(p, r, pal, s.drift, s.bass, false) })
			}, w, h)
			tt.Frame()
			shot := tt.Image()
			draw.Draw(row, image.Rect(i*w, 0, (i+1)*w, h), shot, shot.Bounds().Min, draw.Src)
		}
		rows = append(rows, row)
	}
	out := image.NewRGBA(image.Rect(0, 0, rows[0].Bounds().Dx(), h*len(rows)))
	for i, r := range rows {
		draw.Draw(out, image.Rect(0, i*h, r.Bounds().Dx(), (i+1)*h), r, image.Point{}, draw.Src)
	}
	f, _ := os.Create(filepath.Join(dir, "drift.png"))
	defer f.Close()
	png.Encode(f, out)
}
