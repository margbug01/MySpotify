package player

import (
	"testing"
	"time"
	"vinyl/internal/spotify"
)

type connectTestOutput struct {
	closed chan struct{}
	paused bool
}

func (o *connectTestOutput) Play()                   {}
func (o *connectTestOutput) Pause()                  { o.paused = true }
func (o *connectTestOutput) Playing() bool           { return !o.paused }
func (o *connectTestOutput) Seek(time.Duration)      {}
func (o *connectTestOutput) Position() time.Duration { return time.Second }
func (o *connectTestOutput) Duration() time.Duration { return time.Minute }
func (o *connectTestOutput) OnEnd(func()) func()     { return func() {} }
func (o *connectTestOutput) SetVolume(float64)       {}
func (o *connectTestOutput) Close()                  { close(o.closed) }

// Mirroring another device closes real audio and never asks for a local media source.
func TestConnectMirrorStopsLocalAudio(t *testing.T) {
	p := New(func(func()) {})
	track := spotify.Track{ID: "song", Name: "Song", Duration: 60000}
	p.Cue([]spotify.Track{track})
	output := &connectTestOutput{closed: make(chan struct{})}
	p.out = output
	p.fade.attach(output, 1)
	p.playing = true
	p.MirrorConnect(track, 12*time.Second, false)
	if !output.paused || !p.IsRemote() || p.Playing() || p.Position() != 12*time.Second {
		t.Fatal("mirror did not release local playback")
	}
	select {
	case <-output.closed:
	case <-time.After(time.Second):
		t.Fatal("old audio was not released")
	}
	first := p.out
	p.MirrorConnect(track, 15*time.Second, false)
	if p.out != first || p.Position() != 15*time.Second {
		t.Fatal("same track was unnecessarily reloaded")
	}
	p.StopConnect()
	if p.out != nil || p.Playing() {
		t.Fatal("stop retained remote output")
	}
}

// Handled Connect actions must not mutate the local queue or restart local audio.
func TestConnectInterceptsControls(t *testing.T) {
	p := New(func(func()) {})
	p.Cue([]spotify.Track{{ID: "original"}})
	var actions []string
	p.Control = func(action string, value any) bool { actions = append(actions, action); return true }
	p.PlayQueue([]spotify.Track{{ID: "other"}}, 0)
	p.TogglePlay()
	p.Next()
	p.Prev()
	p.PrevTrack()
	p.Seek(time.Second)
	p.SetVolume(.4)
	p.ToggleShuffle()
	p.CycleRepeat()
	p.ended()
	if len(actions) != 10 || p.Current().ID != "original" || p.out != nil {
		t.Fatalf("controls escaped routing: %v", actions)
	}
}

// A Connect queue naming the artist of what plays tells the system and the
// app, once: the same names again do not.
func TestConnectQueueAnnouncesANamedArtist(t *testing.T) {
	p := New(func(func()) {})
	changed := 0
	p.Cue([]spotify.Track{{ID: "song", Name: "Song", Artists: []spotify.Artist{{ID: "spotify:artist:s"}}}})
	p.Changed = func() { changed++ }
	named := []spotify.Track{{ID: "song", Name: "Song", Artists: []spotify.Artist{{Name: "Singer"}}}}
	p.ConnectQueue(named)
	p.ConnectQueue(named)
	if p.Current().ArtistNames() != "Singer" || changed != 1 {
		t.Fatalf("artist %q, %d changes", p.Current().ArtistNames(), changed)
	}
}
