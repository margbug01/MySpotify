package player

import (
	"os"
	"sync"
	"testing"

	"vinyl/internal/lyrics"
	"vinyl/internal/spotify"
)

// lyricsFor gives the tracks' lyrics as the test running asks, nil for none.
var (
	lyricsMu  sync.Mutex
	lyricsFor func(spotify.Track) []lyrics.Line
)

// No test looks lyrics up on the internet. A track's are looked for on a
// goroutine, which can outlive the test that played it: the real lookup
// went on to LRCLIB, reading lyricsWaits as a later test set it, and a
// test that put its own lyricsOf in place wrote it as such a goroutine read
// it, each a race that failed whatever test ran then. lyricsOf is set once,
// before any test; a test gives lyrics with useLyrics. Otherwise a lookup
// never answers, as a site that takes its time: the tests wait on what the
// player posts in order, which lyrics answering at once would cut into.
func TestMain(m *testing.M) {
	lyricsOf = func(t spotify.Track) []lyrics.Line {
		lyricsMu.Lock()
		f := lyricsFor
		lyricsMu.Unlock()
		if f == nil {
			select {}
		}
		return f(t)
	}
	os.Exit(m.Run())
}

// useLyrics has the tracks' lyrics be what f gives, until the test ends.
func useLyrics(t *testing.T, f func(spotify.Track) []lyrics.Line) {
	lyricsMu.Lock()
	lyricsFor = f
	lyricsMu.Unlock()
	t.Cleanup(func() {
		lyricsMu.Lock()
		lyricsFor = nil
		lyricsMu.Unlock()
	})
}
