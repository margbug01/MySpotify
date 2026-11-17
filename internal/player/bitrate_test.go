package player

import (
	"testing"
	"time"

	"vinyl/internal/spotify"
)

// Bitrate is the bits per second of the file the track plays at, once its
// media is resolved, and 0 before.
func TestBitrateIsTheFilePlaying(t *testing.T) {
	old := resolve
	defer func() { resolve = old }()
	resolve = func(uri string) (*spotify.Media, error) {
		return &spotify.Media{Manifest: "#EXTM3U\nhttps://cdn.test/a.mp4\n", Duration: time.Minute, Bitrate: 256000}, nil
	}
	ch := make(chan func(), 16)
	p := New(func(fn func()) { ch <- fn })
	p.Web = &Web{addr: "127.0.0.1:1", cmds: make(chan string, 256), vol: make(chan float64, 1), outs: map[int]*webOut{}}
	if p.Bitrate() != 0 {
		t.Fatalf("before a track: %d", p.Bitrate())
	}
	p.PlayQueue([]spotify.Track{{ID: "a"}}, 0)
	until(t, ch, func() bool { return p.out != nil })
	if p.Bitrate() != 256000 {
		t.Fatalf("playing: %d", p.Bitrate())
	}
}
