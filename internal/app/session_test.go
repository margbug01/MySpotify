package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/audio"

	"vinyl/internal/player"
	"vinyl/internal/spotify"
)

// The session is kept a while after playback changes and every while as
// it plays, and paused nothing more is due. It was kept by a ticker that
// woke the app, and redrew its window, every half minute even at rest.
func TestSessionRestsWhilePaused(t *testing.T) {
	var due []func()
	defer func(after func(time.Duration, func())) { sessionAfter = after }(sessionAfter)
	sessionAfter = func(_ time.Duration, fn func()) { due = append(due, fn) }

	a := newTestApp()
	a.pl.Changed = a.keepSession
	a.pl.Cue([]spotify.Track{{ID: "7", Name: "宝船", Duration: 200000}})
	out := audio.NewPlayer(&silence{})
	out.SetVolume(0)
	out.Play()
	a.pl.Attach(out)
	defer out.Close()

	a.pl.TogglePlay()
	a.pl.TogglePlay()
	if len(due) != 1 {
		t.Fatalf("%d saves due after pausing and playing", len(due))
	}
	due[0]()
	if len(due) != 2 {
		t.Fatalf("playing, %d saves due after one, want another", len(due)-1)
	}

	a.pl.TogglePlay()
	due[1]()
	if len(due) != 2 {
		t.Errorf("paused, %d more saves due", len(due)-2)
	}
}

// The track playing is kept as soon as it changes, not when the next save
// comes due: killed within half a minute of a skip, the app came back to
// the song before.
func TestSessionKeepsTrackAsItChanges(t *testing.T) {
	defer func(after func(time.Duration, func())) { sessionAfter = after }(sessionAfter)
	sessionAfter = func(time.Duration, func()) {}
	path := filepath.Join(t.TempDir(), "session.json")
	defer func(p func() (string, error)) { sessionPath = p }(sessionPath)
	sessionPath = func() (string, error) { return path, nil }

	a := newTestApp()
	for _, tr := range []spotify.Track{{ID: "7", Name: "宝船", Duration: 200000}, {ID: "8", Name: "富士山下", Duration: 200000}} {
		a.pl.Cue([]spotify.Track{tr})
		a.playbackChanged()
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var s player.Session
		if err := json.Unmarshal(b, &s); err != nil {
			t.Fatal(err)
		}
		if len(s.Queue) != 1 || s.Queue[0].ID != tr.ID {
			t.Errorf("the session kept %s, want %s as it began to play", b, tr.Name)
		}
	}
}
