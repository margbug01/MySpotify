package app

import (
	"image"
	"testing"

	"github.com/egoist/mygo/ui"
)

// A cover that loads while its place shows fades in; one ready as its
// place first shows is there at once.
func TestArtFadesIn(t *testing.T) {
	var bmp *ui.Bitmap
	var in float32
	tt := ui.NewTester(func(c *ui.Context) { in = artIn(ui.Box(c).Size(40, 40), bmp) }, 100, 100)
	tt.Frame()
	if in != 0 {
		t.Fatalf("shown %v while loading", in)
	}
	bmp = ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 4, 4)))
	tt.Frame()
	tt.Advance(artFade.Duration / 3)
	if in <= 0 || in >= 1 {
		t.Errorf("shown %v a third of the way", in)
	}
	tt.Advance(artFade.Duration)
	if in != 1 {
		t.Errorf("shown %v once faded in", in)
	}

	in = 0
	ready := ui.NewTester(func(c *ui.Context) { in = artIn(ui.Box(c).Size(40, 40), bmp) }, 100, 100)
	ready.Frame()
	if in != 1 {
		t.Errorf("a cover ready at once shown %v", in)
	}
}
