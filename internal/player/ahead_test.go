package player

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"vinyl/internal/spotify"
)

// The start of a file fetched ahead answers the page's first requests for
// it without the CDN, as the parts they ask for; later parts still come from
// the CDN.
func TestPrefetchedStartAnswersThePage(t *testing.T) {
	file := make([]byte, 1000)
	for i := range file {
		file[i] = byte(i)
	}
	var hits atomic.Int32
	cdn := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.ServeContent(rw, r, "a.mp4", time.Time{}, bytes.NewReader(file))
	}))
	defer cdn.Close()
	src := cdn.URL + "/audio/a"
	m := &spotify.Media{Manifest: "#EXTM3U\n#EXT-X-MAP:URI=\"" + src + "\",BYTERANGE=\"100@0\"\n" +
		"#EXT-X-BYTERANGE:300@100\n" + src + "\n#EXT-X-BYTERANGE:300@400\n" + src + "\n#EXT-X-BYTERANGE:300@700\n" + src + "\n#EXT-X-ENDLIST\n"}
	w := &Web{addr: "127.0.0.1:1", cmds: make(chan string, 8), vol: make(chan float64, 1), outs: map[int]*webOut{}}
	w.Prefetch(m)
	// The start comes in parts asked for at once, so that a slow link does
	// not hold the first one behind a whole minute of audio.
	if n := hits.Load(); n < 2 {
		t.Fatalf("prefetch asked the CDN %d times, want the start in parts", n)
	}
	w.Open(m)
	get := func(rg string) (*httptest.ResponseRecorder, int32) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://"+w.addr+"/m/1/0", nil)
		req.Header.Set("Range", rg)
		w.ServeHTTP(rec, req)
		return rec, hits.Load()
	}
	// What the prefetch fetched is answered from memory; the rest, past
	// it, still comes from the CDN.
	prefetched := hits.Load()
	for _, c := range []struct {
		rg       string
		from, to int
		cdn      int32
	}{{"bytes=0-399", 0, 399, 0}, {"bytes=400-699", 400, 699, 0}, {"bytes=300-399", 300, 399, 0}} {
		rec, n := get(c.rg)
		if rec.Code != http.StatusPartialContent || !bytes.Equal(rec.Body.Bytes(), file[c.from:c.to+1]) || n != prefetched+c.cdn {
			t.Fatalf("%s: %d, %d bytes, CDN asked %d times", c.rg, rec.Code, rec.Body.Len(), n-prefetched)
		}
		if rec.Header().Get("Content-Range") != "bytes "+c.rg[6:]+"/1000" {
			t.Fatalf("%s: Content-Range %q", c.rg, rec.Header().Get("Content-Range"))
		}
	}
}

// aheadPlayer plays in a Web that serves no page but counts as one,
// resolving at once, each URI asked for on asked. The tests queue episodes,
// whose lyrics are never looked for.
func aheadPlayer(t *testing.T) (*Player, chan string, func(string, func() bool)) {
	t.Helper()
	asked := make(chan string, 16)
	realResolve, delay := resolve, aheadDelay
	resolve = func(uri string) (*spotify.Media, error) {
		asked <- uri
		return &spotify.Media{Manifest: "#EXTM3U\n", Duration: time.Minute}, nil
	}
	aheadDelay = 0
	t.Cleanup(func() { resolve, aheadDelay = realResolve, delay })
	ch := make(chan func(), 32)
	p := New(func(fn func()) { ch <- fn })
	p.Web = &Web{addr: "127.0.0.1:1", cmds: make(chan string, 256), vol: make(chan float64, 1), outs: map[int]*webOut{}, server: &http.Server{}}
	until := func(what string, done func() bool) {
		t.Helper()
		deadline := time.After(3 * time.Second)
		for !done() {
			select {
			case fn := <-ch:
				fn()
			case <-deadline:
				t.Fatal("waited for", what)
			}
		}
	}
	return p, asked, until
}

// The track after the one playing is resolved as that one starts, and plays
// from what was resolved without asking the service again.
func TestNextPlaysWhatWasResolvedAhead(t *testing.T) {
	p, asked, until := aheadPlayer(t)
	p.PlayQueue([]spotify.Track{{ID: "1", Episode: true}, {ID: "2", Episode: true}, {ID: "3", Episode: true}}, 0)
	until("track 2 resolved ahead", func() bool { return p.upNext.id == "2" && p.upNext.m != nil })
	p.Next()
	until("track 3 resolved ahead", func() bool { return p.upNext.id == "3" && p.upNext.m != nil })
	close(asked)
	var got []string
	for uri := range asked {
		got = append(got, uri)
	}
	if strings.Join(got, " ") != "spotify:episode:1 spotify:episode:2 spotify:episode:3" {
		t.Fatalf("asked for %v", got)
	}
}

// At the end of the queue, the track resolved ahead is the first, where Next
// goes: the last one played used to start without it.
func TestLastTrackResolvesTheFirstAhead(t *testing.T) {
	p, _, until := aheadPlayer(t)
	p.PlayQueue([]spotify.Track{{ID: "1", Episode: true}, {ID: "2", Episode: true}}, 1)
	until("track 1 resolved ahead", func() bool { return p.upNext.id == "1" && p.upNext.m != nil })
}
