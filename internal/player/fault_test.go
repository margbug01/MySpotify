package player

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"vinyl/internal/spotify"
)

// licensed sets up a player that plays in a Web with no page, the licence
// service the handler, as the web player signed in as "old": the track at
// the head of the queue loaded, as track 1. It returns what the player
// posts, the player and the waits it asked for, whose functions the test
// runs.
func licensed(t *testing.T, licence http.HandlerFunc) (chan func(), *Player, *[]func()) {
	t.Helper()
	srv := httptest.NewServer(licence)
	host, res := licenseHost, resolve
	licenseHost = srv.URL
	resolve = func(string) (*spotify.Media, error) {
		return &spotify.Media{Manifest: "#EXTM3U\nhttps://cdn.test/a.mp4\n", Duration: time.Minute}, nil
	}
	spotify.SetIdentity(spotify.Identity{Token: "old", Expires: time.Now().Add(time.Hour)})
	var waits []func()
	after := afterFunc
	afterFunc = func(d time.Duration, fn func()) { waits = append(waits, fn) }
	t.Cleanup(func() {
		srv.Close()
		licenseHost, resolve, afterFunc = host, res, after
		spotify.SetIdentity(spotify.Identity{})
		spotify.Renew = nil
	})

	ch := make(chan func(), 16)
	p := New(func(fn func()) { ch <- fn })
	p.Web = &Web{addr: "127.0.0.1:1", cmds: make(chan string, 256), vol: make(chan float64, 1), outs: map[int]*webOut{}}
	p.PlayQueue([]spotify.Track{{ID: "1"}, {ID: "2"}, {ID: "3"}}, 0)
	next(t, ch)
	if p.out == nil {
		t.Fatal("did not load")
	}
	return ch, p, &waits
}

// page asks the server what the page in the web view would, and returns
// the status it answers.
func page(p *Player, method, path, body string) int {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, "http://"+p.Web.addr+path, strings.NewReader(body))
	p.Web.ServeHTTP(rec, req)
	return rec.Code
}

// fails asks for the licence of track gen as the page would, which the page
// then reports it could not play.
func fails(t *testing.T, ch chan func(), p *Player, gen string) {
	t.Helper()
	page(p, http.MethodPost, "/license?gen="+gen, "challenge")
	page(p, http.MethodPost, "/event", `{"gen":`+gen+`,"type":"error","msg":"keySystemError keySystemLicenseRequestFailed"}`)
	next(t, ch)
}

// A licence turned away for asking too often waits as long as Spotify asks,
// saying so, then loads the track again; it doesn't give up on it.
func TestThrottledLicenceWaits(t *testing.T) {
	var asked int
	ch, p, waits := licensed(t, func(w http.ResponseWriter, r *http.Request) {
		asked++
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	fails(t, ch, p, "1")
	// The page reports it twice, as the licence and the media element both
	// fail: the second didn't pass for the track ending, on to the next.
	page(p, http.MethodPost, "/event", `{"gen":1,"type":"error","msg":"media 3"}`)
	for len(ch) > 0 {
		next(t, ch)
	}
	if p.Err() != "Spotify 暂时限流，稍后重试" || !p.Playing() || p.Current().ID != "1" {
		t.Fatalf("throttled: err %q, playing %v, track %s", p.Err(), p.Playing(), p.Current().ID)
	}
	if len(*waits) != 1 || asked != 1 {
		t.Fatalf("%d waits, licence asked %d times", len(*waits), asked)
	}
	(*waits)[0]()
	next(t, ch)
	if !p.Loading() || p.Current().ID != "1" || p.Err() != "" {
		t.Fatalf("after the wait: loading %v, track %s, err %q", p.Loading(), p.Current().ID, p.Err())
	}
	next(t, ch)
	if p.out == nil {
		t.Fatal("did not load again")
	}
}

// Turned away for asking too often where the audio is, the track waits too,
// rather than the queue skipping on through tracks that would be as well.
func TestThrottledMediaWaits(t *testing.T) {
	ch, p, waits := licensed(t, func(w http.ResponseWriter, r *http.Request) {})
	resolve = func(string) (*spotify.Media, error) { return nil, &spotify.Throttled{Wait: 3 * time.Second} }
	p.Next()
	next(t, ch)
	if p.Current().ID != "2" || p.Err() != "Spotify 暂时限流，稍后重试" || len(*waits) != 1 {
		t.Fatalf("track %s, err %q, %d waits", p.Current().ID, p.Err(), len(*waits))
	}
}

// Paused while it waits, the track doesn't load once the wait is over.
func TestThrottledPausedStays(t *testing.T) {
	ch, p, waits := licensed(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	})
	fails(t, ch, p, "1")
	p.TogglePlay()
	(*waits)[0]()
	next(t, ch)
	if p.Loading() || p.Playing() {
		t.Fatalf("loading %v, playing %v after the wait, paused", p.Loading(), p.Playing())
	}
}

// A licence refused stops at the track: the rest of the queue would be
// refused too.
func TestRefusedLicenceStops(t *testing.T) {
	ch, p, waits := licensed(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	fails(t, ch, p, "1")
	if p.Playing() || p.Current().ID != "1" || p.Err() != "这首歌暂时无法播放" || len(*waits) != 0 {
		t.Fatalf("refused: playing %v, track %s, err %q, %d waits", p.Playing(), p.Current().ID, p.Err(), len(*waits))
	}
}

// A track the page gave up on, its licence granted, goes on to the next.
func TestFailedTrackSkips(t *testing.T) {
	ch, p, _ := licensed(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("licence"))
	})
	fails(t, ch, p, "1")
	if !p.Loading() || p.Current().ID != "2" {
		t.Fatalf("loading %v, track %s; want track 2 loading", p.Loading(), p.Current().ID)
	}
}

// A licence asked for with an identity run out is asked for again with a
// fresh one.
func TestLicenceRenewsIdentity(t *testing.T) {
	_, p, _ := licensed(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer new" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte("licence"))
	})
	renewed := 0
	spotify.Renew = func() (spotify.Identity, error) {
		renewed++
		return spotify.Identity{Token: "new", Expires: time.Now().Add(time.Hour)}, nil
	}
	if code := page(p, http.MethodPost, "/license?gen=1", "challenge"); code != http.StatusOK || renewed != 1 {
		t.Fatalf("answered %d, renewed %d times", code, renewed)
	}
}
