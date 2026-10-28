package player

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"vinyl/internal/spotify"
)

// An episode where its podcast keeps it plays as a file: the page loads
// it as such, and its bytes pass through a range at a time, as asked.
func TestFilePlaysAsItIs(t *testing.T) {
	w := &Web{addr: "127.0.0.1:1", cmds: make(chan string, 16), vol: make(chan float64, 1), outs: map[int]*webOut{}}
	o := w.Open(&spotify.Media{File: "https://podcast.test/ep.mp3", Duration: 10 * time.Minute}).(*webOut)
	if got := sent(w); len(got) != 1 || got[0] != "vinyl.load(1,'file')" {
		t.Fatalf("sent %q", got)
	}
	old := cdnDo
	defer func() { cdnDo = old }()
	var asked []string
	fail := true
	cdnDo = func(req *http.Request) (*http.Response, error) {
		asked = append(asked, req.URL.String()+" "+req.Header.Get("Range"))
		if fail {
			return nil, errors.New("reset")
		}
		return &http.Response{StatusCode: http.StatusPartialContent, Body: io.NopCloser(strings.NewReader("ID3")),
			Header: http.Header{"Content-Range": {"bytes 0-2/30000000"}, "Content-Type": {"audio/mpeg"}}}, nil
	}
	get := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://"+w.addr+"/m/1/0", nil)
		req.Header.Set("Range", "bytes=0-2")
		w.ServeHTTP(rec, req)
		return rec
	}
	// A failure is the page's to try again: asked once, not again as the
	// last of a CDN's mirrors, and not passed over for a while as a CDN is.
	if rec := get(); rec.Code != http.StatusBadGateway || len(asked) != 1 {
		t.Fatalf("failed: %d, asked %q", rec.Code, asked)
	}
	fail = false
	rec := get()
	if rec.Code != http.StatusPartialContent || rec.Body.String() != "ID3" || rec.Header().Get("Content-Range") != "bytes 0-2/30000000" {
		t.Fatalf("passed through %d %q %v", rec.Code, rec.Body.String(), rec.Header())
	}
	if len(asked) != 2 || asked[1] != "https://podcast.test/ep.mp3 bytes=0-2" {
		t.Fatalf("asked %q", asked)
	}

	// It says how long it is once loaded; a song's manifest does already.
	ask(w, http.MethodPost, "/event", `{"gen": 1, "type": "duration", "t": 754.5}`)
	if o.Duration() != 754500*time.Millisecond {
		t.Fatalf("duration %v", o.Duration())
	}
	song := w.Open(&spotify.Media{Manifest: "#EXTM3U\nhttps://cdn.test/a.mp4\n", Duration: 3 * time.Minute}).(*webOut)
	if got := sent(w); len(got) != 1 || got[0] != "vinyl.load(2)" {
		t.Fatalf("song: sent %q", got)
	}
	ask(w, http.MethodPost, "/event", `{"gen": 2, "type": "duration", "t": 1}`)
	if song.Duration() != 3*time.Minute {
		t.Fatalf("song's duration %v", song.Duration())
	}
}

// A video podcast's episode and its video keep the same time: no lead is
// guessed, as for a song's music video, longer than the song.
func TestEpisodesVideoHasNoLead(t *testing.T) {
	v := &spotify.Video{Duration: 1174 * time.Second}
	if lead := leadGuess(&spotify.Track{Duration: 1170000}, v); lead == 0 {
		t.Fatal("a song's video: no lead")
	}
	if lead := leadGuess(&spotify.Track{Duration: 1170000, Episode: true}, v); lead != 0 {
		t.Fatalf("an episode's video: lead %v", lead)
	}
}

// An episode that plays nowhere here, as one only in MP4s for Widevine on
// macOS, says so and stops, rather than ask for each of its podcast's other
// episodes in turn, which are kept as it is: in a queue of songs, the song
// after it plays.
func TestEpisodeThatDoesNotPlayHereStops(t *testing.T) {
	old := resolve
	defer func() { resolve = old }()
	var mu sync.Mutex
	var asked []string
	resolve = func(uri string) (*spotify.Media, error) {
		mu.Lock()
		asked = append(asked, uri)
		mu.Unlock()
		if strings.HasPrefix(uri, "spotify:episode:") {
			return nil, fmt.Errorf("%s: %w", uri, spotify.ErrNoAudioHere)
		}
		return &spotify.Media{Manifest: "#EXTM3U\nhttps://cdn.test/a.mp4\n", Duration: time.Minute}, nil
	}
	ch := make(chan func(), 16)
	p := New(func(fn func()) { ch <- fn })
	p.Web = &Web{addr: "127.0.0.1:1", cmds: make(chan string, 256), vol: make(chan float64, 1), outs: map[int]*webOut{}}
	pod := spotify.Album{ID: "spotify:show:pod"}
	p.PlayQueue([]spotify.Track{{ID: "a", Episode: true, Album: pod}, {ID: "b", Episode: true, Album: pod}, {ID: "c", Episode: true, Album: pod}}, 0)
	next(t, ch)
	mu.Lock()
	n := len(asked)
	mu.Unlock()
	if n != 1 || p.Playing() || p.Err() != "这一集在这台电脑上放不了" {
		t.Fatalf("asked %d, playing %v, err %q", n, p.Playing(), p.Err())
	}

	mu.Lock()
	asked = nil
	mu.Unlock()
	p.PlayQueue([]spotify.Track{{ID: "a", Episode: true, Album: pod}, {ID: "s"}}, 0)
	until(t, ch, func() bool { return p.out != nil })
	mu.Lock()
	defer mu.Unlock()
	if len(asked) != 2 || asked[1] != "spotify:track:s" {
		t.Fatalf("asked %q", asked)
	}
}

// A file's address leads, through redirects seconds each, to where it is:
// the ranges after the first are asked for there. Where it led no longer
// answering, as a signed address that ran out, the address is asked again.
func TestFileGoesWhereItLed(t *testing.T) {
	w := &Web{addr: "127.0.0.1:1", cmds: make(chan string, 16), vol: make(chan float64, 1), outs: map[int]*webOut{}}
	w.Open(&spotify.Media{File: "https://podcast.test/ep.mp3", Duration: 10 * time.Minute})
	old := cdnDo
	defer func() { cdnDo = old }()
	var asked []string
	gone := false
	cdnDo = func(req *http.Request) (*http.Response, error) {
		asked = append(asked, req.URL.Host)
		res := &http.Response{StatusCode: http.StatusPartialContent, Body: io.NopCloser(strings.NewReader("ID3")), Header: http.Header{}}
		switch req.URL.Host {
		case "podcast.test":
			// What the client answers once it followed the redirects.
			res.Request = httptest.NewRequest(http.MethodGet, "https://media.test/ep.mp3?sig=1", nil)
		case "media.test":
			res.Request = req
			if gone {
				res.StatusCode = http.StatusForbidden
			}
		}
		return res, nil
	}
	get := func() int {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://"+w.addr+"/m/1/0", nil)
		req.Header.Set("Range", "bytes=0-2")
		w.ServeHTTP(rec, req)
		return rec.Code
	}
	get()
	get()
	if strings.Join(asked, " ") != "podcast.test media.test" {
		t.Fatalf("asked %q", asked)
	}
	gone = true
	if code := get(); code != http.StatusPartialContent || strings.Join(asked, " ") != "podcast.test media.test media.test podcast.test" {
		t.Fatalf("where it led gone: %d, asked %q", code, asked)
	}
}
