package app

import (
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
	bmp, img, err := decodeArt(b)
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
