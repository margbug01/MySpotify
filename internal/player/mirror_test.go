package player

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const mirrorFileID = "f6e2ff1804615a9cb45b63cbba993d3ba440eb4a"

// A CDN that does not answer costs the part, not the song: the same bytes
// come from another CDN that serves the file, for the same range.
func TestAPartComesFromAnotherCDN(t *testing.T) {
	var ak, fa atomic.Int32
	restore := mirrorTest(t, []string{
		"https://audio-ak.spotifycdn.com/audio/" + mirrorFileID + "?sig=ak",
		"https://audio-fa.scdn.co/audio/" + mirrorFileID + "?sig=fa",
	}, func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Host, "audio4-fa"), strings.Contains(req.URL.Host, "audio-ak"):
			ak.Add(1)
			return nil, io.ErrUnexpectedEOF // the CDN having a bad day
		case strings.Contains(req.URL.Host, "audio-fa"):
			fa.Add(1)
			if got := req.Header.Get("Range"); got != "bytes=1338-164217" {
				t.Errorf("mirror asked for %q, want the same range", got)
			}
			return partial("from-fa", "bytes 1338-164217/3307037"), nil
		}
		t.Errorf("asked %s", req.URL.Host)
		return nil, io.ErrUnexpectedEOF
	})
	defer restore()

	// The manifest names audio4-fa, which is not among the mirrors: it fails
	// too, and the parts come from audio-fa.
	source := "https://audio4-fa.scdn.co/audio/" + mirrorFileID + "?sig=manifest"
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/m/1/0", nil)
	req.Header.Set("Range", "bytes=1338-164217")
	res, err := fetchPart(req, source, req.Header.Get("Range"))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if string(body) != "from-fa" {
		t.Fatalf("body %q, want the mirror's", body)
	}
	if fa.Load() != 1 {
		t.Errorf("asked the mirror %d times", fa.Load())
	}
}

// A CDN that failed is passed over for a while, so that the rest of the
// track does not wait on it again.
func TestAFailedCDNIsPassedOver(t *testing.T) {
	var ak atomic.Int32
	restore := mirrorTest(t, []string{
		"https://audio-ak.spotifycdn.com/audio/" + mirrorFileID + "?sig=ak",
		"https://audio-fa.scdn.co/audio/" + mirrorFileID + "?sig=fa",
	}, func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Host, "audio-ak") {
			ak.Add(1)
			return nil, io.ErrUnexpectedEOF
		}
		return partial("ok", "bytes 0-1/3307037"), nil
	})
	defer restore()

	source := "https://audio-ak.spotifycdn.com/audio/" + mirrorFileID + "?sig=ak"
	for range 3 {
		req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/m/1/1", nil)
		res, err := fetchPart(req, source, "bytes=0-1")
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
	}
	// The first try, then never again: the two after go straight to the
	// mirror.
	if ak.Load() != 1 {
		t.Fatalf("asked the failed CDN %d times, want once", ak.Load())
	}
}

// When every CDN fails, the caller is told, rather than handed a part of
// nothing.
func TestNoCDNAnsweringIsAnError(t *testing.T) {
	restore := mirrorTest(t, []string{
		"https://audio-ak.spotifycdn.com/audio/" + mirrorFileID + "?sig=ak",
	}, func(req *http.Request) (*http.Response, error) {
		return nil, io.ErrUnexpectedEOF
	})
	defer restore()

	source := "https://audio-ak.spotifycdn.com/audio/" + mirrorFileID + "?sig=ak"
	if res, err := fetchPart(httptest.NewRequest(http.MethodGet, "http://127.0.0.1/m/1/2", nil), source, "bytes=0-1"); err == nil {
		res.Body.Close()
		t.Fatal("no CDN answered, and no error came back")
	}
}

// A file whose mirrors cannot be had still plays from the one the manifest
// names: nothing else is tried.
func TestWithoutMirrorsTheManifestPlays(t *testing.T) {
	var asked atomic.Int32
	restore := mirrorTest(t, nil, func(req *http.Request) (*http.Response, error) {
		asked.Add(1)
		return partial("manifest", "bytes 0-1/3307037"), nil
	})
	defer restore()

	source := "https://audio4-fa.scdn.co/audio/" + mirrorFileID + "?sig=manifest"
	res, err := fetchPart(httptest.NewRequest(http.MethodGet, "http://127.0.0.1/m/1/3", nil), source, "bytes=0-1")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if asked.Load() != 1 {
		t.Fatalf("asked %d times, want once", asked.Load())
	}
}

// A file without an ID at the end of its URL — a video's, say — gets no
// mirrors, and plays from where it is.
func TestAURLWithoutAFileIDHasNoMirrors(t *testing.T) {
	var asked atomic.Int32
	restore := mirrorTest(t, nil, func(req *http.Request) (*http.Response, error) {
		asked.Add(1)
		return partial("x", "bytes 0-1/10"), nil
	})
	defer restore()

	if got := otherMirrors("https://audio4-fa.scdn.co/audio/not-a-file-id"); len(got) != 0 {
		t.Fatalf("mirrors %q for a URL without a file ID", got)
	}
	source := "https://audio4-fa.scdn.co/audio/not-a-file-id"
	res, err := fetchPart(httptest.NewRequest(http.MethodGet, "http://127.0.0.1/m/1/4", nil), source, "bytes=0-1")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if asked.Load() != 1 {
		t.Fatalf("asked %d times, want once", asked.Load())
	}
}

// mirrorTest answers storage-resolve with urls and every CDN with cdn, then
// puts the app's own back. It swaps the player's hooks, not the client
// Spotify's own requests go through: other tests' background work is still
// using that one.
func mirrorTest(t *testing.T, urls []string, cdn func(*http.Request) (*http.Response, error)) func() {
	t.Helper()
	oldDo, oldMirrors := cdnDo, mirrorsOf
	cdnDo = cdn
	mirrorsOf = func(string) ([]string, error) {
		if urls == nil {
			return nil, io.ErrUnexpectedEOF
		}
		return urls, nil
	}
	mirrors.mu.Lock()
	mirrors.dead = nil
	mirrors.mu.Unlock()
	return func() {
		cdnDo, mirrorsOf = oldDo, oldMirrors
		mirrors.mu.Lock()
		mirrors.dead = nil
		mirrors.mu.Unlock()
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func partial(body, contentRange string) *http.Response {
	h := http.Header{}
	if contentRange != "" {
		h.Set("Content-Range", contentRange)
	}
	return &http.Response{StatusCode: 206, Body: io.NopCloser(strings.NewReader(body)), Header: h}
}
