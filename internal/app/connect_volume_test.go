package app

import (
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"vinyl/internal/l10n"
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

// Switching between local and remote devices syncs the volume slider and restores local player volume on return.
func TestConnectDeviceSwitchVolumeSync(t *testing.T) {
	a := connectTestApp()
	a.volume = 1.0
	a.pl.SetVolume(1.0)

	// Switch to a remote speaker that has volume at 50% (32768/65535).
	speakerCluster := `{"cluster":{"active_device_id":"speaker","server_timestamp_ms":"2000","devices":{"local":{"name":"Vinyl","volume":"65535"},"speaker":{"name":"Speaker","volume":"32768"}},"player_state":{"track":{"uri":"spotify:track:abc","metadata":{"title":"Song","artist_name":"Artist"}}}}}`
	a.connectEvent(spotify.ConnectEvent{Kind: "cluster", Data: json.RawMessage(speakerCluster)})

	if math.Abs(a.volume-0.5) > 0.01 {
		t.Errorf("switching to speaker: expected volume ~0.5, got %v", a.volume)
	}
	if math.Abs(a.pl.Volume()-1.0) > 0.01 {
		t.Errorf("switching to speaker changed local player volume to %v", a.pl.Volume())
	}

	// While connected to speaker, turn volume to 70%.
	a.volume = 0.7
	a.pl.SetVolume(0.7)
	// Drift local player volume to verify restoreLocalVolume reapplies it after exiting remote mode.
	a.here(func() { a.pl.SetVolume(0.25) })

	// Switch back to local computer which had volume at 100%.
	localCluster := `{"cluster":{"active_device_id":"local","server_timestamp_ms":"3000","devices":{"local":{"name":"Vinyl","volume":"65535"},"speaker":{"name":"Speaker","volume":"45874"}},"player_state":{"track":{"uri":"spotify:track:abc","metadata":{"title":"Song","artist_name":"Artist"}}}}}`
	a.connectEvent(spotify.ConnectEvent{Kind: "cluster", Data: json.RawMessage(localCluster)})

	if a.pl.IsRemote() || math.Abs(a.volume-1.0) > 0.01 || math.Abs(a.pl.Volume()-1.0) > 0.01 {
		t.Errorf("switching back to local: expected volume 1.0 and local mode, got remote=%v volume=%v pl=%v", a.pl.IsRemote(), a.volume, a.pl.Volume())
	}
}

// Playing here, clicking a line of the lyrics jumps there at once rather than waiting on Spotify's servers.
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

// Switching back to local restores the local computer's volume even when replace_state arrives before the cluster event.
func TestConnectSwitchBackWithReplaceState(t *testing.T) {
	a := connectTestApp()
	a.volume = 1.0
	a.localVolume = 1.0
	a.pl.SetVolume(1.0)

	// Switch to a remote speaker that has volume at 50%.
	speakerCluster := `{"cluster":{"active_device_id":"speaker","server_timestamp_ms":"2000","devices":{"local":{"name":"Vinyl","volume":"65535"},"speaker":{"name":"Speaker","volume":"32768"}},"player_state":{"track":{"uri":"spotify:track:abc","metadata":{"title":"Song","artist_name":"Artist"}}}}}`
	a.connectEvent(spotify.ConnectEvent{Kind: "cluster", Data: json.RawMessage(speakerCluster)})

	if math.Abs(a.volume-0.5) > 0.01 {
		t.Fatalf("switching to speaker: expected volume ~0.5, got %v", a.volume)
	}
	a.here(func() { a.pl.SetVolume(0.25) })

	// Spotify sends replace_state command to start playing locally.
	replaceCmd := `{"type":"replace_state","state_ref":{"state_index":0,"paused":false},"state_machine":{"state_machine_id":"m1","tracks":[{"metadata":{"uri":"spotify:track:abc","name":"Song","duration":200000,"authors":[{"name":"Artist"}]}}],"states":[{"state_id":"s1","track":0,"transitions":{}}]}}`
	a.connectEvent(spotify.ConnectEvent{Kind: "command", Data: json.RawMessage(replaceCmd)})
	if a.pl.IsRemote() || math.Abs(a.volume-1.0) > 0.01 || math.Abs(a.pl.Volume()-1.0) > 0.01 {
		t.Fatalf("replace_state back to local: expected volume 1.0 and local mode, got remote=%v volume=%v pl=%v", a.pl.IsRemote(), a.volume, a.pl.Volume())
	}

	// Then cluster event arrives.
	localCluster := `{"cluster":{"active_device_id":"local","server_timestamp_ms":"3000","devices":{"local":{"name":"Vinyl","volume":"65535"},"speaker":{"name":"Speaker","volume":"32768"}},"player_state":{"track":{"uri":"spotify:track:abc","metadata":{"title":"Song","artist_name":"Artist"}}}}}`
	a.connectEvent(spotify.ConnectEvent{Kind: "cluster", Data: json.RawMessage(localCluster)})

	if a.pl.IsRemote() || math.Abs(a.volume-1.0) > 0.01 || math.Abs(a.pl.Volume()-1.0) > 0.01 {
		t.Fatalf("switching back to local with replace_state: expected volume 1.0, got remote=%v volume=%v pl=%v", a.pl.IsRemote(), a.volume, a.pl.Volume())
	}
}

// Explicit local volume 0 restores to 0 rather than settings default or remote volume across cluster switches, replace_state, and stopConnect.
func TestConnectZeroVolumeRestore(t *testing.T) {
	speakerCluster := `{"cluster":{"active_device_id":"speaker","server_timestamp_ms":"2000","devices":{"local":{"name":"Vinyl","volume":"0"},"speaker":{"name":"Speaker","volume":"32768"}},"player_state":{"track":{"uri":"spotify:track:abc","metadata":{"title":"Song","artist_name":"Artist"}}}}}`
	localCluster := `{"cluster":{"active_device_id":"local","server_timestamp_ms":"3000","devices":{"local":{"name":"Vinyl","volume":"0"},"speaker":{"name":"Speaker","volume":"32768"}},"player_state":{"track":{"uri":"spotify:track:abc","metadata":{"title":"Song","artist_name":"Artist"}}}}}`

	// Case 1: cluster switch back to local.
	a := connectTestApp()
	a.settings = defaultSettings()
	a.volume = 0.2
	a.scrollVolume(ui.InputEvent{Kind: ui.InputScroll, DY: 400})
	if a.volume != 0 || !a.hasLocalVolume || a.pl.Volume() != 0 {
		t.Fatalf("scrollVolume to 0 failed: volume=%v hasLocalVolume=%v pl=%v", a.volume, a.hasLocalVolume, a.pl.Volume())
	}
	a.connectEvent(spotify.ConnectEvent{Kind: "cluster", Data: json.RawMessage(speakerCluster)})
	if math.Abs(a.volume-0.5) > 0.01 {
		t.Fatalf("switching to speaker: expected ~0.5, got %v", a.volume)
	}
	a.here(func() { a.pl.SetVolume(0.4) })
	a.connectEvent(spotify.ConnectEvent{Kind: "cluster", Data: json.RawMessage(localCluster)})
	if a.pl.IsRemote() || a.volume != 0 || a.localVolume != 0 || a.pl.Volume() != 0 {
		t.Fatalf("switching back via cluster: expected 0, got remote=%v volume=%v localVolume=%v pl=%v", a.pl.IsRemote(), a.volume, a.localVolume, a.pl.Volume())
	}

	// Case 2: replace_state (applyConnect) before local cluster arrives.
	b := connectTestApp()
	b.settings = defaultSettings()
	b.volume = 0.2
	b.scrollVolume(ui.InputEvent{Kind: ui.InputScroll, DY: 400})
	b.connectEvent(spotify.ConnectEvent{Kind: "cluster", Data: json.RawMessage(speakerCluster)})
	if math.Abs(b.volume-0.5) > 0.01 {
		t.Fatalf("switching to speaker: expected ~0.5, got %v", b.volume)
	}
	b.here(func() { b.pl.SetVolume(0.4) })
	replaceCmd := `{"type":"replace_state","state_ref":{"state_index":0,"paused":false},"state_machine":{"state_machine_id":"m1","tracks":[{"metadata":{"uri":"spotify:track:abc","name":"Song","duration":200000,"authors":[{"name":"Artist"}]}}],"states":[{"state_id":"s1","track":0,"transitions":{}}]}}`
	b.connectEvent(spotify.ConnectEvent{Kind: "command", Data: json.RawMessage(replaceCmd)})
	if b.pl.IsRemote() || b.volume != 0 || b.pl.Volume() != 0 {
		t.Fatalf("replace_state/applyConnect: expected 0, got remote=%v volume=%v pl=%v", b.pl.IsRemote(), b.volume, b.pl.Volume())
	}
	b.connectEvent(spotify.ConnectEvent{Kind: "cluster", Data: json.RawMessage(localCluster)})
	if b.pl.IsRemote() || b.volume != 0 || b.pl.Volume() != 0 {
		t.Fatalf("cluster after replace_state: expected 0, got remote=%v volume=%v pl=%v", b.pl.IsRemote(), b.volume, b.pl.Volume())
	}

	// Case 3: stopConnect while playing remotely.
	c := connectTestApp()
	c.settings = defaultSettings()
	c.volume = 0.2
	c.scrollVolume(ui.InputEvent{Kind: ui.InputScroll, DY: 400})
	c.connectEvent(spotify.ConnectEvent{Kind: "cluster", Data: json.RawMessage(speakerCluster)})
	if !c.isRemote() || math.Abs(c.volume-0.5) > 0.01 {
		t.Fatalf("expected remote at ~0.5, got remote=%v volume=%v", c.isRemote(), c.volume)
	}
	c.here(func() { c.pl.SetVolume(0.4) })
	c.stopConnect()
	if c.volume != 0 || c.pl.IsRemote() || c.pl.Volume() != 0 {
		t.Fatalf("stopConnect: expected volume=0 local, got volume=%v remote=%v pl=%v", c.volume, c.pl.IsRemote(), c.pl.Volume())
	}

	// Case 4: unrecorded localVolume falls back to settings.Volume.
	d := connectTestApp()
	d.settings = defaultSettings()
	d.volume = 0.5
	d.restoreLocalVolume()
	if d.volume != 0.8 || d.pl.Volume() != 0.8 {
		t.Fatalf("unrecorded localVolume fallback: expected 0.8, got volume=%v pl=%v", d.volume, d.pl.Volume())
	}
}

// Quitting while playing remotely preserves an explicit local volume of 0 in settings.
func TestConnectZeroVolumeQuitSave(t *testing.T) {
	speakerCluster := `{"cluster":{"active_device_id":"speaker","server_timestamp_ms":"2000","devices":{"local":{"name":"Vinyl","volume":"0"},"speaker":{"name":"Speaker","volume":"32768"}},"player_state":{"track":{"uri":"spotify:track:abc","metadata":{"title":"Song","artist_name":"Artist"}}}}}`

	a := connectTestApp()
	a.settings = defaultSettings()
	a.volume = 0.2
	a.scrollVolume(ui.InputEvent{Kind: ui.InputScroll, DY: 400})
	a.connectEvent(spotify.ConnectEvent{Kind: "cluster", Data: json.RawMessage(speakerCluster)})
	if !a.isRemote() || math.Abs(a.volume-0.5) > 0.01 {
		t.Fatalf("expected remote at ~0.5, got remote=%v volume=%v", a.isRemote(), a.volume)
	}
	a.settings.Volume = 0.8
	a.syncVolumeBeforeQuit()
	if a.settings.Volume != 0 {
		t.Fatalf("quit sync while remote: expected settings.Volume=0, got %v", a.settings.Volume)
	}

	// Full OnBeforeQuit sequence (stopConnect then syncVolumeBeforeQuit).
	b := connectTestApp()
	b.settings = defaultSettings()
	b.volume = 0.2
	b.scrollVolume(ui.InputEvent{Kind: ui.InputScroll, DY: 400})
	b.connectEvent(spotify.ConnectEvent{Kind: "cluster", Data: json.RawMessage(speakerCluster)})
	b.settings.Volume = 0.8
	b.stopConnect()
	b.syncVolumeBeforeQuit()
	if b.settings.Volume != 0 || b.volume != 0 {
		t.Fatalf("stopConnect + quit sync: expected 0, got settings=%v volume=%v", b.settings.Volume, b.volume)
	}

	// Unrecorded localVolume while remote must not clobber settings.Volume with 0.
	c := connectTestApp()
	c.settings = defaultSettings()
	c.connect.cluster.Active = "speaker"
	c.volume = 0.5
	c.syncVolumeBeforeQuit()
	if c.settings.Volume != 0.8 {
		t.Fatalf("unrecorded localVolume quit sync: expected 0.8, got %v", c.settings.Volume)
	}
}

// External set_volume commands are ignored during remote playback and applied during local playback past the echo window.
func TestConnectRemoteSetVolumeIsolated(t *testing.T) {
	a := connectTestApp()
	a.settings = defaultSettings()
	a.localVolume = 0.3
	a.hasLocalVolume = true
	a.settings.Volume = 0.3
	a.volume = 0.5
	a.connect.cluster.Active = "speaker"
	a.connect.volLocal = time.Now().Add(-time.Minute)

	a.connectEvent(spotify.ConnectEvent{Kind: "command", Data: json.RawMessage(`{"type":"set_volume","volume":65535}`)})
	if a.localVolume != 0.3 || a.settings.Volume != 0.3 || a.volume != 0.5 || a.pl.Volume() != 0.5 {
		t.Fatalf("remote cluster set_volume mutated local state: local=%v settings=%v vol=%v pl=%v", a.localVolume, a.settings.Volume, a.volume, a.pl.Volume())
	}

	// Also when MirrorConnect has marked pl.IsRemote() true.
	speakerCluster := `{"cluster":{"active_device_id":"speaker","server_timestamp_ms":"2000","devices":{"local":{"name":"Vinyl","volume":"19660"},"speaker":{"name":"Speaker","volume":"32768"}},"player_state":{"track":{"uri":"spotify:track:abc","metadata":{"title":"Song","artist_name":"Artist"}}}}}`
	a.connectEvent(spotify.ConnectEvent{Kind: "cluster", Data: json.RawMessage(speakerCluster)})
	a.connect.volLocal = time.Now().Add(-time.Minute)
	a.connectEvent(spotify.ConnectEvent{Kind: "command", Data: json.RawMessage(`{"type":"set_volume","volume":65535}`)})
	if a.localVolume != 0.3 || a.settings.Volume != 0.3 || math.Abs(a.volume-0.5) > 0.01 || a.pl.Volume() != 0.5 {
		t.Fatalf("MirrorConnect remote set_volume mutated local state: local=%v settings=%v vol=%v pl=%v", a.localVolume, a.settings.Volume, a.volume, a.pl.Volume())
	}

	// Local playback past echo window updates localVolume, settings.Volume, volume, and player volume.
	a.pl.StopConnect()
	a.connect.cluster.Active = "local"
	a.connect.volLocal = time.Now().Add(-time.Minute)
	a.connectEvent(spotify.ConnectEvent{Kind: "command", Data: json.RawMessage(`{"type":"set_volume","volume":0}`)})
	if a.localVolume != 0 || !a.hasLocalVolume || a.settings.Volume != 0 || a.volume != 0 || a.pl.Volume() != 0 {
		t.Fatalf("local set_volume 0 failed: local=%v has=%v settings=%v vol=%v pl=%v", a.localVolume, a.hasLocalVolume, a.settings.Volume, a.volume, a.pl.Volume())
	}
	a.connectEvent(spotify.ConnectEvent{Kind: "command", Data: json.RawMessage(`{"type":"set_volume","volume":32768}`)})
	if math.Abs(a.localVolume-0.5) > 0.01 || math.Abs(a.settings.Volume-0.5) > 0.01 || math.Abs(a.volume-0.5) > 0.01 || math.Abs(a.pl.Volume()-0.5) > 0.01 {
		t.Fatalf("local set_volume ~0.5 failed: local=%v settings=%v vol=%v pl=%v", a.localVolume, a.settings.Volume, a.volume, a.pl.Volume())
	}
}

// Local Player.SetVolume routed through connectControl marks hasLocalVolume so explicit 0 survives remote switches.
func TestConnectControlVolumeSetsHasLocalVolume(t *testing.T) {
	defer func(old func(*spotify.Connect, string, float64) error) { connectVolume = old }(connectVolume)
	connectVolume = func(*spotify.Connect, string, float64) error { return nil }

	a := connectTestApp()
	a.settings = defaultSettings()
	a.connect.cluster.Active = "local"
	a.pl.SetVolume(0)
	if !a.hasLocalVolume || a.localVolume != 0 || a.settings.Volume != 0 || a.volume != 0 || a.pl.Volume() != 0 {
		t.Fatalf("connectControl volume 0 failed: has=%v local=%v settings=%v vol=%v pl=%v", a.hasLocalVolume, a.localVolume, a.settings.Volume, a.volume, a.pl.Volume())
	}

	speakerCluster := `{"cluster":{"active_device_id":"speaker","server_timestamp_ms":"2000","devices":{"local":{"name":"Vinyl","volume":"0"},"speaker":{"name":"Speaker","volume":"32768"}},"player_state":{"track":{"uri":"spotify:track:abc","metadata":{"title":"Song","artist_name":"Artist"}}}}}`
	localCluster := `{"cluster":{"active_device_id":"local","server_timestamp_ms":"3000","devices":{"local":{"name":"Vinyl","volume":"0"},"speaker":{"name":"Speaker","volume":"32768"}},"player_state":{"track":{"uri":"spotify:track:abc","metadata":{"title":"Song","artist_name":"Artist"}}}}}`
	a.connectEvent(spotify.ConnectEvent{Kind: "cluster", Data: json.RawMessage(speakerCluster)})
	a.connectEvent(spotify.ConnectEvent{Kind: "cluster", Data: json.RawMessage(localCluster)})
	if a.volume != 0 || a.pl.Volume() != 0 {
		t.Fatalf("restoring after connectControl volume 0: expected 0, got vol=%v pl=%v", a.volume, a.pl.Volume())
	}
}

// Clicking the mute button syncs localVolume and settings.Volume during local playback and leaves them untouched during remote playback.
func TestMuteButtonSyncsLocalAndRemoteVolume(t *testing.T) {
	defer func(old func(*spotify.Connect, string, float64) error) { connectVolume = old }(connectVolume)
	connectVolume = func(*spotify.Connect, string, float64) error { return nil }

	tr := spotify.Track{ID: "1", Name: "Song", Artists: []spotify.Artist{{Name: "Artist"}}, Duration: 200000}
	muteLabel := l10n.T("静音")

	// Case 1: local playback without Connect.
	a := newTestApp()
	a.settings = defaultSettings()
	a.volume, a.localVolume, a.hasLocalVolume, a.settings.Volume = 0.7, 0.7, true, 0.7
	a.pl.SetVolume(0.7)
	a.pl.Cue([]spotify.Track{tr})
	tt := ui.NewTester(func(c *ui.Context) { a.volumeSlider(c, white, white.Alpha(0.2), 180) }, 260, 48)
	tt.Frame()
	if err := tt.Click(muteLabel); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.volume != 0 || a.localVolume != 0 || !a.hasLocalVolume || a.settings.Volume != 0 || a.pl.Volume() != 0 {
		t.Fatalf("mute without Connect: volume=%v local=%v has=%v settings=%v pl=%v", a.volume, a.localVolume, a.hasLocalVolume, a.settings.Volume, a.pl.Volume())
	}
	if err := tt.Click(muteLabel); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.volume != 0.7 || a.localVolume != 0.7 || a.settings.Volume != 0.7 || a.pl.Volume() != 0.7 {
		t.Fatalf("unmute without Connect: volume=%v local=%v settings=%v pl=%v", a.volume, a.localVolume, a.settings.Volume, a.pl.Volume())
	}

	// Case 2: local playback with Connect, then switch to remote and back.
	a.connect = &connectState{client: spotify.NewConnect("local", "Vinyl", nil), observer: true, receiver: true, jobs: make(chan func() error, 32), done: make(chan struct{})}
	a.pl.Control = a.connectControl
	a.connect.cluster.Active = "local"
	a.volume, a.localVolume, a.hasLocalVolume, a.settings.Volume = 0.6, 0.6, true, 0.6
	tt.Frame()
	if err := tt.Click(muteLabel); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.volume != 0 || a.localVolume != 0 || a.settings.Volume != 0 || a.pl.Volume() != 0 {
		t.Fatalf("mute with Connect: volume=%v local=%v settings=%v pl=%v", a.volume, a.localVolume, a.settings.Volume, a.pl.Volume())
	}
	speakerCluster := `{"cluster":{"active_device_id":"speaker","server_timestamp_ms":"2000","devices":{"local":{"name":"Vinyl","volume":"0"},"speaker":{"name":"Speaker","volume":"32768"}},"player_state":{"track":{"uri":"spotify:track:abc","metadata":{"title":"Song","artist_name":"Artist"}}}}}`
	localCluster := `{"cluster":{"active_device_id":"local","server_timestamp_ms":"3000","devices":{"local":{"name":"Vinyl","volume":"0"},"speaker":{"name":"Speaker","volume":"32768"}},"player_state":{"track":{"uri":"spotify:track:abc","metadata":{"title":"Song","artist_name":"Artist"}}}}}`
	a.connectEvent(spotify.ConnectEvent{Kind: "cluster", Data: json.RawMessage(speakerCluster)})
	a.pl.SetLyrics(nil)
	if math.Abs(a.volume-0.5) > 0.01 {
		t.Fatalf("switching to speaker: expected ~0.5, got %v", a.volume)
	}
	a.connectEvent(spotify.ConnectEvent{Kind: "cluster", Data: json.RawMessage(localCluster)})
	if a.volume != 0 || a.pl.Volume() != 0 {
		t.Fatalf("switching back after mute: expected 0, got vol=%v pl=%v", a.volume, a.pl.Volume())
	}

	// Case 3: remote playback must not overwrite localVolume or settings.Volume.
	a.localVolume, a.hasLocalVolume, a.settings.Volume = 0.25, true, 0.25
	a.volume = 0.8
	a.connect.cluster.Active = "speaker"
	tt.Frame()
	if err := tt.Click(muteLabel); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.volume != 0 || a.localVolume != 0.25 || a.settings.Volume != 0.25 || a.pl.Volume() != 0 {
		t.Fatalf("remote mute altered local state: volume=%v local=%v settings=%v pl=%v", a.volume, a.localVolume, a.settings.Volume, a.pl.Volume())
	}
	if err := tt.Click(muteLabel); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.volume != 0.8 || a.localVolume != 0.25 || a.settings.Volume != 0.25 || a.pl.Volume() != 0 {
		t.Fatalf("remote unmute altered local state: volume=%v local=%v settings=%v pl=%v", a.volume, a.localVolume, a.settings.Volume, a.pl.Volume())
	}
}

// Clicking the local device row in the Connect menu does not restore local volume or clear volLocal until remote mode actually exits.
func TestConnectLocalRowClickDefersVolumeRestore(t *testing.T) {
	oldTransfer := connectTransfer
	defer func() { connectTransfer = oldTransfer }()
	connectTransfer = func(_ *spotify.Connect, target string) error {
		if target != "local" {
			t.Fatalf("unexpected transfer target %q", target)
		}
		return errors.New("transfer failed")
	}

	speakerCluster := `{"cluster":{"active_device_id":"speaker","server_timestamp_ms":"2000","devices":{"local":{"name":"Vinyl","volume":"13107"},"speaker":{"name":"Speaker","volume":"39321"}},"player_state":{"track":{"uri":"spotify:track:abc","metadata":{"title":"Song","artist_name":"Artist"}}}}}`
	localCluster := `{"cluster":{"active_device_id":"local","server_timestamp_ms":"3000","devices":{"local":{"name":"Vinyl","volume":"0"},"speaker":{"name":"Speaker","volume":"39321"}},"player_state":{"track":{"uri":"spotify:track:abc","metadata":{"title":"Song","artist_name":"Artist"}}}}}`
	replaceCmd := `{"type":"replace_state","state_ref":{"state_index":0,"paused":false},"state_machine":{"state_machine_id":"m1","tracks":[{"metadata":{"uri":"spotify:track:abc","name":"Song","duration":200000,"authors":[{"name":"Artist"}]}}],"states":[{"state_id":"s1","track":0,"transitions":{}}]}}`

	// Case 1: prior local volume 0.2, transfer fails, then applyConnect exits remote mode.
	a := testConnectApp(t)
	a.settings = defaultSettings()
	a.volume, a.localVolume, a.hasLocalVolume = 0.2, 0.2, true
	a.here(func() { a.pl.SetVolume(0.2) })
	a.connectEvent(spotify.ConnectEvent{Kind: "cluster", Data: json.RawMessage(speakerCluster)})
	if !a.isRemote() || math.Abs(a.volume-0.6) > 0.01 {
		t.Fatalf("expected remote at ~0.6, got remote=%v volume=%v", a.isRemote(), a.volume)
	}
	a.connect.volLocal = time.Now()
	a.here(func() { a.pl.SetVolume(0.9) })

	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click(l10n.T("播放设备")); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click(l10n.T("此电脑 · ") + appName); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(a.connect.jobs) != 1 {
		t.Fatalf("expected 1 transfer job, got %d", len(a.connect.jobs))
	}
	if err := (<-a.connect.jobs)(); err == nil {
		t.Fatal("expected connectTransfer error")
	}
	if !a.isRemote() || math.Abs(a.volume-0.6) > 0.01 || a.connect.volLocal.IsZero() || math.Abs(a.pl.Volume()-0.9) > 0.01 {
		t.Fatalf("clicking local row while transfer failed mutated state prematurely: remote=%v vol=%v volLocalZero=%v pl=%v", a.isRemote(), a.volume, a.connect.volLocal.IsZero(), a.pl.Volume())
	}

	a.connectEvent(spotify.ConnectEvent{Kind: "command", Data: json.RawMessage(replaceCmd)})
	if a.isRemote() || math.Abs(a.volume-0.2) > 0.01 || math.Abs(a.pl.Volume()-0.2) > 0.01 || !a.connect.volLocal.IsZero() {
		t.Fatalf("applyConnect did not restore local volume: remote=%v vol=%v pl=%v volLocalZero=%v", a.isRemote(), a.volume, a.pl.Volume(), a.connect.volLocal.IsZero())
	}

	// Case 2: prior local volume 0, transfer fails, then cluster event exits remote mode.
	b := testConnectApp(t)
	b.settings = defaultSettings()
	b.volume, b.localVolume, b.hasLocalVolume = 0, 0, true
	b.here(func() { b.pl.SetVolume(0) })
	b.connectEvent(spotify.ConnectEvent{Kind: "cluster", Data: json.RawMessage(speakerCluster)})
	if !b.isRemote() || math.Abs(b.volume-0.6) > 0.01 {
		t.Fatalf("expected remote at ~0.6, got remote=%v volume=%v", b.isRemote(), b.volume)
	}
	b.connect.volLocal = time.Now()
	b.here(func() { b.pl.SetVolume(0.75) })

	tb := ui.NewTester(b.view, 1200, 800)
	tb.Frame()
	if err := tb.Click(l10n.T("播放设备")); err != nil {
		t.Fatal(err)
	}
	tb.Frame()
	if err := tb.Click(l10n.T("此电脑 · ") + appName); err != nil {
		t.Fatal(err)
	}
	tb.Frame()
	if len(b.connect.jobs) != 1 {
		t.Fatalf("expected 1 transfer job, got %d", len(b.connect.jobs))
	}
	if err := (<-b.connect.jobs)(); err == nil {
		t.Fatal("expected connectTransfer error")
	}
	if !b.isRemote() || math.Abs(b.volume-0.6) > 0.01 || b.connect.volLocal.IsZero() || math.Abs(b.pl.Volume()-0.75) > 0.01 {
		t.Fatalf("clicking local row while transfer failed mutated state prematurely: remote=%v vol=%v volLocalZero=%v pl=%v", b.isRemote(), b.volume, b.connect.volLocal.IsZero(), b.pl.Volume())
	}

	b.connectEvent(spotify.ConnectEvent{Kind: "cluster", Data: json.RawMessage(localCluster)})
	if b.isRemote() || b.volume != 0 || b.pl.Volume() != 0 || !b.connect.volLocal.IsZero() {
		t.Fatalf("cluster exit did not restore local volume 0: remote=%v vol=%v pl=%v volLocalZero=%v", b.isRemote(), b.volume, b.pl.Volume(), b.connect.volLocal.IsZero())
	}
}
