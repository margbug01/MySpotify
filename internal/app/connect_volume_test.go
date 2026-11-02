package app

import (
	"encoding/json"
	"testing"
	"time"

	"vinyl/internal/spotify"
)

func connectTestApp() *app {
	a := newTestApp()
	a.connect = &connectState{client: spotify.NewConnect("local", "Vinyl", nil), observer: true, receiver: true, jobs: make(chan func() error, 32), done: make(chan struct{})}
	a.pl.Control = a.connectControl
	return a
}

// A drag over another device's volume asks for where it ended, not for
// every percent on the way: each was sent in turn, walking the device there
// a step at a time long after the drag, and filling the queue.
func TestConnectVolumeSendsTheLatest(t *testing.T) {
	defer func(old func(*spotify.Connect, string, float64) error) { connectVolume = old }(connectVolume)
	var sent []float64
	connectVolume = func(_ *spotify.Connect, target string, v float64) error {
		if target != "phone" {
			t.Errorf("sent to %q", target)
		}
		sent = append(sent, v)
		return nil
	}
	a := connectTestApp()
	a.connect.cluster.Active = "phone"
	for i := range 50 {
		a.pl.SetVolume(float64(i) / 100)
	}
	if len(a.connect.jobs) != 1 {
		t.Fatalf("%d requests queued for one drag", len(a.connect.jobs))
	}
	(<-a.connect.jobs)()
	if len(sent) != 1 || sent[0] != 0.49 {
		t.Fatalf("sent %v", sent)
	}
	a.pl.SetVolume(0.2)
	if len(a.connect.jobs) != 1 {
		t.Fatal("a later turn was not sent")
	}
	(<-a.connect.jobs)()
	if sent[1] != 0.2 {
		t.Fatalf("sent %v", sent)
	}
}

// Playing here, the volume Spotify sends back as the listener turns it is
// a step behind, and does not set the slider back.
func TestConnectVolumeHereIgnoresItsEcho(t *testing.T) {
	a := connectTestApp()
	a.volume = 0.6
	a.pl.SetVolume(0.6)
	a.connectEvent(spotify.ConnectEvent{Kind: "command", Data: json.RawMessage(`{"type":"set_volume","volume":6553}`)})
	if a.volume != 0.6 {
		t.Fatalf("echo set the volume to %v", a.volume)
	}
	a.connect.volLocal = time.Now().Add(-time.Minute)
	a.connectEvent(spotify.ConnectEvent{Kind: "command", Data: json.RawMessage(`{"type":"set_volume","volume":6553}`)})
	if a.volume > 0.11 {
		t.Fatalf("another device's volume was ignored: %v", a.volume)
	}
}

// Playing here, clicking a line of the lyrics jumps there at once: it went
// to Spotify's servers and waited for them to say so.
func TestConnectSeekHereIsAtOnce(t *testing.T) {
	a := connectTestApp()
	a.pl.Cue([]spotify.Track{{ID: "1", Name: "x", Duration: 200000}})
	a.pl.Seek(42 * time.Second)
	if a.pl.Position() != 42*time.Second {
		t.Fatalf("position %v", a.pl.Position())
	}
	if len(a.connect.jobs) != 0 {
		t.Fatal("asked Spotify to seek")
	}
}
