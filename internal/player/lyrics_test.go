package player

import (
	"errors"
	"slices"
	"testing"
	"time"
	"vinyl/internal/lyrics"
	"vinyl/internal/spotify"
)

// Missing or failed Spotify lyrics fall back, while existing lyrics and podcasts never query LRCLIB.
func TestLyricsFallback(t *testing.T) {
	for _, mode := range []string{"primary", "missing", "error", "invalid", "plain", "empty", "podcast"} {
		t.Run(mode, func(t *testing.T) {
			primaryCalls, fallbackCalls := 0, 0
			track := spotify.Track{ID: "track", Episode: mode == "podcast"}
			lines := findLyrics(track, func(string) (string, error) {
				primaryCalls++
				switch mode {
				case "primary":
					return "[00:01.00]original", nil
				case "error":
					return "", errors.New("offline")
				case "invalid":
					return "bad lrc", nil
				}
				return "", nil
			}, func(spotify.Track) (string, string, error) {
				fallbackCalls++
				if mode == "empty" {
					return "", "", nil
				}
				if mode == "plain" {
					return "bad lrc", "first\n\nsecond", nil
				}
				return "[00:02.00]fallback", "plain", nil
			})
			switch mode {
			case "primary":
				if fallbackCalls != 0 || len(lines) != 1 || lines[0].Text != "original" {
					t.Fatal("primary replaced")
				}
			case "podcast":
				if primaryCalls != 0 || fallbackCalls != 0 || len(lines) != 0 {
					t.Fatal("podcast queried")
				}
			case "empty":
				if len(lines) != 0 {
					t.Fatal("invented lyrics")
				}
			case "plain":
				if len(lines) != 2 || !lines[0].Unsynced || lyrics.Index(lines, time.Minute) != -1 || len(lyrics.Quiets(lines, time.Minute, time.Second)) != 0 {
					t.Fatal("plain text treated as synchronized")
				}
			default:
				if len(lines) != 1 || lines[0].Text != "fallback" || lines[0].At != 2*time.Second || fallbackCalls != 1 {
					t.Fatal("fallback not used")
				}
			}
		})
	}
}

// LRCLIB busy or out of reach is asked again; a lookup it turns down is not.
func TestLyricsRetriesWhatLRCLIBCouldNotAnswer(t *testing.T) {
	old := lyricsWaits
	lyricsWaits = []time.Duration{0, 0}
	defer func() { lyricsWaits = old }()
	for _, c := range []struct {
		name  string
		errs  []error
		calls int
		found bool
	}{
		{"busy", []error{&spotify.LyricsError{Site: "lrclib", Status: 503}, &spotify.LyricsError{Site: "lrclib", Status: 503}}, 3, true},
		{"offline", []error{errors.New("timeout"), errors.New("timeout"), errors.New("timeout")}, 3, false},
		{"refused", []error{&spotify.LyricsError{Site: "lrclib", Status: 400}}, 1, false},
	} {
		calls := 0
		lines := findLyrics(spotify.Track{ID: "track"}, func(string) (string, error) { return "", nil }, func(spotify.Track) (string, string, error) {
			calls++
			if calls <= len(c.errs) {
				return "", "", c.errs[calls-1]
			}
			return "[00:02.00]found", "", nil
		})
		if calls != c.calls || (len(lines) > 0) != c.found {
			t.Errorf("%s: %d calls, lyrics %v", c.name, calls, len(lines) > 0)
		}
	}
}

// The lyrics sites know 周旋 by 王以太, as Spotify names him to Vinyl, not
// by the Yitai Wang Connect can give: what finds nothing in time is looked
// up again by Spotify's names, once, only when they differ, and plain text
// found first is kept if that finds nothing better.
func TestLookupBySpotifysNames(t *testing.T) {
	artist := func(name string) spotify.Track {
		return spotify.Track{ID: "song", Name: "周旋", Artists: []spotify.Artist{{Name: name}}}
	}
	for _, c := range []struct {
		name, given, spotify string
		plain                []string // the artists the sites have plain text by
		lookups, decorates   int
		want                 string
	}{
		{"connect", "Yitai Wang", "王以太", nil, 2, 1, "synced"},
		{"vinyl", "王以太", "王以太", nil, 1, 0, "synced"},
		{"unnamed", "", "王以太", nil, 1, 1, "synced"},
		{"missing", "Someone", "Someone", nil, 1, 1, ""},
		{"plain-then-timed", "Yitai Wang", "王以太", []string{"Yitai Wang"}, 2, 1, "synced"},
		{"plain-kept", "Yitai Wang", "Wang Yitai", []string{"Yitai Wang"}, 2, 1, "plain"},
	} {
		lookups, decorates := 0, 0
		synced, plain, err := lookupByNames(artist(c.given), func(list []spotify.Track) ([]spotify.Track, error) {
			decorates++
			return []spotify.Track{artist(c.spotify)}, nil
		}, func(tr spotify.Track) (string, string, error) {
			lookups++
			switch {
			case tr.Artists[0].Name == "王以太":
				return "[00:01.00]found", "", nil
			case slices.Contains(c.plain, tr.Artists[0].Name):
				return "", "found", nil
			}
			return "", "", nil
		})
		got := ""
		if synced != "" {
			got = "synced"
		} else if plain != "" {
			got = "plain"
		}
		if err != nil || got != c.want || lookups != c.lookups || decorates != c.decorates {
			t.Errorf("%s: got %q, %d lookups, %d decorates", c.name, got, lookups, decorates)
		}
	}
}

// The sources are asked in turn until one has the lyrics timed; plain text
// waits for the others; the error kept is the one worth asking again for,
// and only when nothing was found.
func TestLyricsSourcesInTurn(t *testing.T) {
	type answer struct {
		synced, plain string
		err           error
	}
	busy, refused := &spotify.LyricsError{Site: "lrclib", Status: 503}, &spotify.LyricsError{Site: "netease", Status: -460}
	for _, c := range []struct {
		name    string
		answers []answer
		asked   int
		want    answer
	}{
		{"netease", []answer{{synced: "[00:01.00]a"}, {synced: "[00:01.00]b"}, {}}, 1, answer{synced: "[00:01.00]a"}},
		{"search", []answer{{}, {plain: "plain"}, {synced: "[00:01.00]c"}}, 3, answer{synced: "[00:01.00]c"}},
		{"plain", []answer{{plain: "first"}, {plain: "second"}, {}}, 3, answer{plain: "first"}},
		{"plain-despite-errors", []answer{{err: refused}, {plain: "plain"}, {err: busy}}, 3, answer{plain: "plain"}},
		{"busy-kept", []answer{{err: refused}, {err: busy}, {}}, 3, answer{err: busy}},
		{"none", []answer{{}, {}, {}}, 3, answer{}},
	} {
		asked := 0
		var sources []func(spotify.Track) (string, string, error)
		for _, a := range c.answers {
			sources = append(sources, func(spotify.Track) (string, string, error) {
				asked++
				return a.synced, a.plain, a.err
			})
		}
		synced, plain, err := firstTimed(spotify.Track{ID: "song"}, sources...)
		if asked != c.asked || synced != c.want.synced || plain != c.want.plain || err != c.want.err {
			t.Errorf("%s: asked %d, got %q %q %v", c.name, asked, synced, plain, err)
		}
	}
}
