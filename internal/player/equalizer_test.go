package player

import (
	"slices"
	"testing"
	"time"

	"vinyl/internal/spotify"
)

// The equalizer's gains reach the page as they are set, once off it hears
// null, and a track loaded after it is turned on has them too.
func TestEqualizerReachesThePage(t *testing.T) {
	old := resolve
	defer func() { resolve = old }()
	resolve = func(uri string) (*spotify.Media, error) {
		return &spotify.Media{Manifest: "#EXTM3U\nhttps://cdn.test/a.mp4\n", Duration: time.Minute}, nil
	}
	ch := make(chan func(), 16)
	p := New(func(fn func()) { ch <- fn })
	p.Web = &Web{addr: "127.0.0.1:1", cmds: make(chan string, 256), vol: make(chan float64, 1), outs: map[int]*webOut{}}
	p.PlayQueue([]spotify.Track{{ID: "a"}, {ID: "b"}}, 0)
	until(t, ch, func() bool { return p.out != nil })
	sent(p.Web)

	p.SetEqualizer(true, [6]float64{6, 5, 3, 0, 0, -2})
	if got := sent(p.Web); !slices.Contains(got, "vinyl.eq([6,5,3,0,0,-2])") {
		t.Fatalf("on: sent %q", got)
	}
	p.SetEqualizer(false, [6]float64{6, 5, 3, 0, 0, -2})
	if got := sent(p.Web); !slices.Contains(got, "vinyl.eq(null)") {
		t.Fatalf("off: sent %q", got)
	}

	p.SetEqualizer(true, [6]float64{1, 1, 1, 1, 1, 1})
	sent(p.Web)
	p.Next()
	until(t, ch, func() bool { return p.out != nil && p.track.ID == "b" })
	if got := sent(p.Web); !slices.Contains(got, "vinyl.eq([1,1,1,1,1,1])") {
		t.Fatalf("next track: sent %q", got)
	}
}
