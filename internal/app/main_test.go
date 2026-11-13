package app

import (
	"os"
	"testing"

	"vinyl/internal/lyrics"
	"vinyl/internal/player"
	"vinyl/internal/spotify"
)

func TestMain(m *testing.M) {
	player.SetLyricsFetcherForTest(func(spotify.Track) []lyrics.Line { return nil })
	os.Exit(m.Run())
}
