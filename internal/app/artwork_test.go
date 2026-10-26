package app

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"testing"
	"time"
)

// An animated WebP cover, as some playlists have, shows its first frame,
// in the bitmap too, which decodes its data again once it drops its pixels.
func TestAnimatedWebPCover(t *testing.T) {
	b, err := os.ReadFile("testdata/animated.webp")
	if err != nil {
		t.Fatal(err)
	}
	bmp, img, err := decodeArt(b, 0)
	if err != nil {
		t.Fatal(err)
	}
	if r := img.Bounds(); r.Dx() != 24 || r.Dy() != 24 {
		t.Errorf("first frame is %v, want 24×24", r)
	}
	if w, h := bmp.Size(); w != 24 || h != 24 {
		t.Errorf("bitmap is %d×%d, want 24×24", w, h)
	}
}

// A cover larger than the size asked for is shrunk to it, keeping its
// shape, and so is the bitmap; one with transparency stays as it is.
func TestCoverShrunkToSizeAsked(t *testing.T) {
	encode := func(img image.Image, enc func(io.Writer, image.Image) error) []byte {
		var buf bytes.Buffer
		if err := enc(&buf, img); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}
	photo := image.NewRGBA(image.Rect(0, 0, 640, 480))
	draw.Draw(photo, photo.Bounds(), image.NewUniform(color.RGBA{200, 40, 60, 255}), image.Point{}, draw.Src)
	b := encode(photo, func(w io.Writer, m image.Image) error { return jpeg.Encode(w, m, nil) })
	bmp, img, err := decodeArt(b, 400)
	if err != nil {
		t.Fatal(err)
	}
	if w, h := bmp.Size(); w != 400 || h != 300 || img.Bounds().Dx() != 400 {
		t.Errorf("shrunk to %d×%d (image %v), want 400×300", w, h, img.Bounds())
	}
	if bmp, _, _ := decodeArt(b, 800); bmp == nil {
		t.Fatal("no bitmap")
	} else if w, _ := bmp.Size(); w != 640 {
		t.Errorf("enlarged to %d, want it left at 640", w)
	}
	clear := image.NewNRGBA(image.Rect(0, 0, 640, 640))
	bmp, _, err = decodeArt(encode(clear, png.Encode), 400)
	if err != nil {
		t.Fatal(err)
	}
	if w, _ := bmp.Size(); w != 640 {
		t.Errorf("transparent cover shrunk to %d", w)
	}
}

// Covers not asked for in a while go, so that a long session does not
// keep every cover it showed; the ones still showing stay.
func TestArtCacheDropsUnusedCovers(t *testing.T) {
	now := time.Unix(1e9, 0)
	c := newArtCache(func(func()) {})
	c.now, c.swept = func() time.Time { return now }, now
	c.art["shown"] = &artwork{used: now}
	c.art["gone"] = &artwork{used: now}
	for range 6 {
		now = now.Add(time.Minute)
		c.get("shown", false)
	}
	if _, ok := c.art["gone"]; ok {
		t.Error("kept a cover not asked for in 6 minutes")
	}
	if _, ok := c.art["shown"]; !ok {
		t.Error("dropped a cover asked for every minute")
	}
}
