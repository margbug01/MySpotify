package app

import (
	"bytes"
	"image/png"
	"testing"
)

// The tray icon is a template, which the taskbar and the menu bar draw in
// their own ink: a pixel of colour in it would show as it is, black on a
// dark taskbar.
func TestTrayIconIsATemplate(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(trayIcon))
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	if b.Dx() != 32 || b.Dy() != 32 {
		t.Errorf("tray icon is %dx%d, not 32x32", b.Dx(), b.Dy())
	}
	shown := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := img.At(x, y).RGBA()
			if a > 0 {
				shown++
			}
			if a > 0 && r|g|bl != 0 {
				t.Fatalf("pixel %d,%d is not black: %d %d %d", x, y, r, g, bl)
			}
		}
	}
	if shown < 32*32/3 {
		t.Errorf("only %d pixels show", shown)
	}
}
