package player

import (
	"errors"
	"testing"
	"time"

	"vinyl/internal/lyrics"
	"vinyl/internal/spotify"
)

// An episode's lyrics are its transcript, asked again when it could not
// be had, as the lyrics sites are, but not when Spotify asked to wait.
func TestEpisodesLyricsAreItsTranscript(t *testing.T) {
	oldWaits, oldOf := lyricsWaits, transcriptOf
	lyricsWaits = []time.Duration{0, 0}
	defer func() { lyricsWaits, transcriptOf = oldWaits, oldOf }()
	said := []lyrics.Line{{At: time.Second, End: 2 * time.Second, Text: "Hello everyone."}}
	var asked []string
	fails := 1
	transcriptOf = func(id string) ([]lyrics.Line, error) {
		asked = append(asked, id)
		if fails > 0 {
			fails--
			return nil, errors.New("connection reset")
		}
		return said, nil
	}
	got := trackLyrics(spotify.Track{ID: "ep", Episode: true})
	if len(got) != 1 || got[0].Text != "Hello everyone." || len(asked) != 2 || asked[0] != "ep" {
		t.Fatalf("lyrics %+v, asked %q", got, asked)
	}

	asked = nil
	transcriptOf = func(id string) ([]lyrics.Line, error) {
		asked = append(asked, id)
		return nil, &spotify.Throttled{Wait: time.Minute}
	}
	if got := trackLyrics(spotify.Track{ID: "ep", Episode: true}); got != nil || len(asked) != 1 {
		t.Fatalf("throttled: lyrics %+v, asked %d times", got, len(asked))
	}
}
