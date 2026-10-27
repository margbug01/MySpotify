package spotify

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

const testFileID = "f6e2ff1804615a9cb45b63cbba993d3ba440eb4a"

// Mirrors names every CDN a file is on, and keeps them: the second ask does
// not go to Spotify.
func TestMirrorsKeepsWhatItAsks(t *testing.T) {
	old := Client
	SetIdentity(Identity{Token: "t", ClientToken: "c", Expires: time.Now().Add(time.Hour)})
	defer func() { Client = old; SetIdentity(Identity{}); forgetMirrors() }()
	forgetMirrors()

	var asked int
	Client = &http.Client{Transport: lyricsTransport(func(req *http.Request) (*http.Response, error) {
		asked++
		if !strings.Contains(req.URL.Path, "/storage-resolve/v2/files/audio/interactive/10/"+testFileID) {
			t.Errorf("asked %s", req.URL.Path)
		}
		if req.URL.Query().Get("alt") != "json" {
			t.Error("asked without alt=json, which answers with protobuf")
		}
		body := `{"result":"CDN","fileid":"` + testFileID + `","ttl":86400,
		 "cdnurl":["https://audio-ak.spotifycdn.com/audio/` + testFileID + `?a=1",
		           "https://audio-fa.scdn.co/audio/` + testFileID + `?a=2"]}`
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})}

	urls, err := Mirrors(testFileID)
	if err != nil {
		t.Fatal(err)
	}
	if len(urls) != 2 || !strings.Contains(urls[1], "audio-fa.scdn.co") {
		t.Fatalf("mirrors %q", urls)
	}
	// Upper case names the same file; the ask is not made twice.
	if _, err := Mirrors(strings.ToUpper(testFileID)); err != nil {
		t.Fatal(err)
	}
	if asked != 1 {
		t.Fatalf("asked %d times, want once", asked)
	}
}

// A file none of the CDNs has is an error, not an empty list: the caller
// keeps the one URL it has.
func TestMirrorsNoneIsAnError(t *testing.T) {
	old := Client
	SetIdentity(Identity{Token: "t", ClientToken: "c", Expires: time.Now().Add(time.Hour)})
	defer func() { Client = old; SetIdentity(Identity{}); forgetMirrors() }()
	forgetMirrors()

	Client = &http.Client{Transport: lyricsTransport(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"result":"STORAGE","cdnurl":[]}`)), Header: http.Header{}}, nil
	})}
	if urls, err := Mirrors(testFileID); err == nil {
		t.Fatalf("no mirrors, no error: %q", urls)
	}
}

// What storage-resolve refuses is passed on, rather than cached as nothing.
func TestMirrorsPassesOnARefusal(t *testing.T) {
	old := Client
	SetIdentity(Identity{Token: "t", ClientToken: "c", Expires: time.Now().Add(time.Hour)})
	defer func() { Client = old; SetIdentity(Identity{}); forgetMirrors() }()
	forgetMirrors()

	Client = &http.Client{Transport: lyricsTransport(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader(`{}`)), Header: http.Header{}}, nil
	})}
	if _, err := Mirrors(testFileID); err == nil {
		t.Fatal("a refusal came back as mirrors")
	}
	// Nothing was kept: the next ask goes out again.
	var asked int
	Client = &http.Client{Transport: lyricsTransport(func(req *http.Request) (*http.Response, error) {
		asked++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(
			`{"cdnurl":["https://audio-ak.spotifycdn.com/audio/` + testFileID + `"]}`)), Header: http.Header{}}, nil
	})}
	if _, err := Mirrors(testFileID); err != nil {
		t.Fatal(err)
	}
	if asked != 1 {
		t.Fatalf("asked %d times after a refusal, want once", asked)
	}
}

// A signature lasts a day; what is kept past that is asked for again.
func TestMirrorsExpire(t *testing.T) {
	old := Client
	SetIdentity(Identity{Token: "t", ClientToken: "c", Expires: time.Now().Add(time.Hour)})
	defer func() { Client = old; SetIdentity(Identity{}); forgetMirrors() }()
	forgetMirrors()

	var asked int
	Client = &http.Client{Transport: lyricsTransport(func(req *http.Request) (*http.Response, error) {
		asked++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(
			`{"cdnurl":["https://audio-ak.spotifycdn.com/audio/` + testFileID + `"]}`)), Header: http.Header{}}, nil
	})}
	if _, err := Mirrors(testFileID); err != nil {
		t.Fatal(err)
	}
	mirrorMu.Lock()
	e := mirrors[testFileID]
	e.at = time.Now().Add(-mirrorTTL - time.Minute)
	mirrors[testFileID] = e
	mirrorMu.Unlock()

	if _, err := Mirrors(testFileID); err != nil {
		t.Fatal(err)
	}
	if asked != 2 {
		t.Fatalf("asked %d times, want twice", asked)
	}
}

// FileIDOf reads the file's ID off a CDN URL, and nothing off one that does
// not name a file.
func TestFileIDOf(t *testing.T) {
	for _, tc := range []struct{ url, want string }{
		{"https://audio4-fa.scdn.co/audio/" + testFileID + "?1793198783_x=y", testFileID},
		{"https://audio-ak.spotifycdn.com/audio/" + testFileID, testFileID},
		{"https://audio-ak.spotifycdn.com/audio/" + strings.ToUpper(testFileID), testFileID},
		{"https://audio-ak.spotifycdn.com/audio/" + testFileID + "/", testFileID},
		{"https://audio-ak.spotifycdn.com/audio/short", ""},
		{"https://audio-ak.spotifycdn.com/audio/zzzzff1804615a9cb45b63cbba993d3ba440eb4a", ""},
		{"", ""},
	} {
		if got := FileIDOf(tc.url); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.url, got, tc.want)
		}
	}
}

func forgetMirrors() {
	mirrorMu.Lock()
	mirrors = map[string]mirrorEntry{}
	mirrorMu.Unlock()
}
