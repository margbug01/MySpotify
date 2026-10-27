package spotify

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type lyricsTransport func(*http.Request) (*http.Response, error)

func (fn lyricsTransport) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }

// Exact fallback requests carry song metadata but never Spotify credentials, and reject other versions.
func TestLRCLyrics(t *testing.T) {
	old := Client
	defer func() { Client = old }()
	track := Track{Name: "Song & 雨", Artists: []Artist{{Name: "Singer"}}, Album: Album{Name: "Album"}, Duration: 200000}
	for _, tc := range []struct {
		name, body string
		status     int
		want, fail bool
	}{
		{"synced", `{"trackName":"Song & 雨","artistName":"Singer","duration":201,"syncedLyrics":"[00:01.00]hello","plainLyrics":"hello"}`, 200, true, false},
		{"plain", `{"trackName":"Song & 雨","artistName":"Singer","duration":200,"plainLyrics":"hello"}`, 200, true, false},
		{"wrong-version", `{"trackName":"Song & 雨","artistName":"Singer","duration":210,"plainLyrics":"wrong"}`, 200, false, false},
		{"wrong-artist", `{"trackName":"Song & 雨","artistName":"Other","duration":200,"plainLyrics":"wrong"}`, 200, false, false},
		{"wrong-title", `{"trackName":"Other","artistName":"Singer","duration":200,"plainLyrics":"wrong"}`, 200, false, false},
		{"instrumental", `{"trackName":"Song & 雨","artistName":"Singer","duration":200,"instrumental":true,"plainLyrics":"wrong"}`, 200, false, false},
		{"missing", `{}`, 404, false, false},
		{"limited", `{}`, 429, false, true},
		{"invalid", `{`, 200, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			Client = &http.Client{Transport: lyricsTransport(func(req *http.Request) (*http.Response, error) {
				if req.URL.Host != "lrclib.net" || req.URL.Path != "/api/get" || req.URL.Query().Get("track_name") != track.Name || req.URL.Query().Get("artist_name") != "Singer" || req.URL.Query().Get("album_name") != "Album" || req.URL.Query().Get("duration") != "200.000" {
					t.Fatal("incorrect exact lookup")
				}
				if req.Header.Get("Authorization") != "" || req.Header.Get("client-token") != "" || req.Header.Get("Cookie") != "" {
					t.Fatal("credentials leaked")
				}
				if _, ok := req.Context().Deadline(); !ok {
					t.Fatal("no deadline")
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}
			synced, plain, err := LRCLyrics(track)
			if (err != nil) != tc.fail || (synced != "" || plain != "") != tc.want {
				t.Fatalf("unexpected result: synced=%t plain=%t err=%v", synced != "", plain != "", err)
			}
		})
	}
}

// A track Connect named no artist of is not looked up: LRCLIB answers 400.
func TestLRCLyricsNeedsAnArtist(t *testing.T) {
	old := Client
	defer func() { Client = old }()
	Client = &http.Client{Transport: lyricsTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("asked LRCLIB without an artist")
		return nil, nil
	})}
	synced, plain, err := LRCLyrics(Track{ID: "a", Name: "Song", Artists: []Artist{{ID: "spotify:artist:x"}}, Duration: 200000})
	if synced != "" || plain != "" || err != nil {
		t.Fatal(synced, plain, err)
	}
}

// LRCLIB keeps many records of a song, some with plain text only: the
// search takes the timed one of the same title and artist nearest in
// length, and asks by name and artist only.
func TestLRCSearch(t *testing.T) {
	old := Client
	defer func() { Client = old }()
	track := Track{Name: "梁山伯與茱麗葉", Artists: []Artist{{Name: "卓文萱"}}, Album: Album{Name: "習慣"}, Duration: 232896}
	results := `[{"trackName":"梁山伯與茱麗葉","artistName":"卓文萱","duration":233,"syncedLyrics":null,"plainLyrics":"plain"},
{"trackName":"梁山伯與茱麗葉","artistName":"別人","duration":232.9,"syncedLyrics":"[00:01.00]other artist"},
{"trackName":"梁山伯與茱麗葉 (Live)","artistName":"卓文萱","duration":232.9,"syncedLyrics":"[00:01.00]live"},
{"trackName":"梁山伯與茱麗葉","artistName":"卓文萱","duration":240.95,"syncedLyrics":"[00:01.00]too long"},
{"trackName":"梁山伯與茱麗葉","artistName":"卓文萱","duration":232.9,"instrumental":true,"syncedLyrics":"[00:01.00]instrumental"},
{"trackName":"梁山伯與茱麗葉","artistName":"卓文萱、曹格","duration":232.0,"syncedLyrics":"[00:01.00]near"},
{"trackName":"梁山伯與茱麗葉","artistName":"卓文萱 曹格","duration":233.0,"syncedLyrics":"[00:01.00]nearest"}]`
	for _, c := range []struct {
		name, body string
		status     int
		want       string
		fail       bool
	}{
		{"found", results, 200, "[00:01.00]nearest", false},
		{"missing", `{}`, 404, "", false},
		{"busy", `{}`, 503, "", true},
	} {
		Client = &http.Client{Transport: lyricsTransport(func(req *http.Request) (*http.Response, error) {
			q := req.URL.Query()
			if req.URL.Host != "lrclib.net" || req.URL.Path != "/api/search" || q.Get("track_name") != track.Name || q.Get("artist_name") != "卓文萱" || q.Has("duration") || q.Has("album_name") {
				t.Fatal("incorrect search", req.URL)
			}
			return &http.Response{StatusCode: c.status, Body: io.NopCloser(strings.NewReader(c.body))}, nil
		})}
		synced, plain, err := LRCSearch(track)
		if synced != c.want || plain != "" || (err != nil) != c.fail {
			t.Errorf("%s: %q %q %v", c.name, synced, plain, err)
		}
	}
}
