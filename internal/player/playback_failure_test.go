package player

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"vinyl/internal/spotify"
)

// A dead Chrome backend must stop on the same song and restart from its position on retry.
func TestBrowserExitStopsAndRestartsPlayback(t *testing.T) {
	ch, p, _ := licensed(t, func(w http.ResponseWriter, r *http.Request) {})
	closed, starts := false, 0
	p.Web.stopBrowser = func() { closed = true }
	p.NewWeb = func() (*Web, error) {
		starts++
		web, err := listen()
		if err == nil {
			t.Cleanup(web.Close)
		}
		return web, err
	}
	p.out.Seek(12 * time.Second)
	p.Web.browserFailed()
	next(t, ch)
	if !closed || p.playing || p.loading || p.at != 0 || p.out != nil || p.Web != nil || p.err == "" {
		t.Fatalf("backend failure did not stop and release playback: %+v", p)
	}
	p.TogglePlay()
	next(t, ch)
	if starts != 1 || !p.playing || p.loading || p.at != 0 || p.out == nil || p.Position() != 12*time.Second || p.err != "" {
		t.Fatalf("retry did not restart the same song at its position: starts=%d player=%+v", starts, p)
	}
}

// Unsupported key systems used to skip every song, including after MV fallback.
func TestUnsupportedKeySystemStopsQueue(t *testing.T) {
	for _, video := range []bool{false, true} {
		for _, msg := range []string{"keySystemError keySystemNoAccess", "keySystemError keySystemNoEME"} {
			ch, p, _ := licensed(t, func(w http.ResponseWriter, r *http.Request) {})
			p.showsVideo, p.video = video, video
			page(p, http.MethodPost, "/event", `{"gen":1,"type":"error","msg":"`+msg+`"}`)
			next(t, ch)
			if p.playing || p.at != 0 || p.loading || p.err == "" || p.showsVideo != video || p.gen != 1 {
				t.Fatalf("video=%v error=%s: advanced, retried, or lost error: %+v", video, msg, p)
			}
		}
	}
}

// Repeated playback errors must stop after three failures rather than drain a long queue.
func TestConsecutivePlaybackFailuresStop(t *testing.T) {
	ch, p, _ := licensed(t, func(w http.ResponseWriter, r *http.Request) {})
	p.queue = append(p.queue, spotify.Track{ID: "4"}, spotify.Track{ID: "5"})
	p.order = append(p.order, 3, 4)
	for gen := 1; gen <= 3; gen++ {
		page(p, http.MethodPost, "/event", `{"gen":`+strconv.Itoa(gen)+`,"type":"error","msg":"media 3"}`)
		next(t, ch)
		next(t, ch) // the network is up
		if gen < 3 {
			if !p.Loading() || p.at != gen {
				t.Fatalf("failure %d did not advance once", gen)
			}
			next(t, ch)
		}
	}
	if p.playing || p.at != 2 || p.loading || p.gen != 3 || p.err == "" {
		t.Fatalf("advanced past consecutive failure limit: %+v", p)
	}
}
