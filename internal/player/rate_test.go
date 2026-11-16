package player

import (
	"slices"
	"strings"
	"testing"
	"time"

	"vinyl/internal/spotify"
)

// An episode plays at the rate set, the page told as it loads and as the
// rate changes; a song after it plays at its own pace.
func TestEpisodesPlayAtTheRateSet(t *testing.T) {
	old := resolve
	defer func() { resolve = old }()
	resolve = func(uri string) (*spotify.Media, error) {
		if strings.HasPrefix(uri, "spotify:episode:") {
			return &spotify.Media{File: "https://podcast.test/ep.mp3", Duration: time.Hour}, nil
		}
		return &spotify.Media{Manifest: "#EXTM3U\nhttps://cdn.test/a.mp4\n", Duration: time.Minute}, nil
	}
	ch := make(chan func(), 16)
	p := New(func(fn func()) { ch <- fn })
	p.Web = &Web{addr: "127.0.0.1:1", cmds: make(chan string, 256), vol: make(chan float64, 1), outs: map[int]*webOut{}}
	p.SetRate(1.5)
	p.PlayQueue([]spotify.Track{{ID: "e", Episode: true}, {ID: "s"}}, 0)
	until(t, ch, func() bool { return p.out != nil })
	if got := sent(p.Web); !slices.Contains(got, "vinyl.rate(1,1.5)") || p.Rate() != 1.5 {
		t.Fatalf("episode: sent %q, rate %v", got, p.Rate())
	}
	p.SetRate(1)
	if got := sent(p.Web); !slices.Contains(got, "vinyl.rate(1,1)") {
		t.Fatalf("back to 1: sent %q", got)
	}

	p.SetRate(2)
	sent(p.Web)
	p.Next()
	until(t, ch, func() bool { return p.out != nil && p.track.ID == "s" })
	for _, cmd := range sent(p.Web) {
		if strings.HasPrefix(cmd, "vinyl.rate(") {
			t.Fatalf("song: sent %q", cmd)
		}
	}
	if p.Rate() != 1 {
		t.Fatalf("song: rate %v", p.Rate())
	}
}

// Between the page's reports, where a track is moves on at its rate.
func TestPositionMovesAtTheRate(t *testing.T) {
	at := time.Unix(1000, 0)
	o := &webOut{dur: time.Hour, pos: 10 * time.Second, at: at, running: true, rate: 2}
	if got := o.position(at.Add(3 * time.Second)); got != 16*time.Second {
		t.Fatalf("at 2×: %v", got)
	}
	o.rate = 0 // as made without one: its own pace
	if got := o.position(at.Add(3 * time.Second)); got != 13*time.Second {
		t.Fatalf("unset: %v", got)
	}
}
