package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"vinyl/internal/l10n"
	"vinyl/internal/spotify"
)

// Concurrent cluster updates during Cast wakeup match the speaker without races and stopConnect cancels polling promptly.
func TestWakeAndTransferCastConcurrentClusterAndStop(t *testing.T) {
	oldTransfer := connectTransfer
	oldPost := connectPost
	oldTimeout := castWakeTimeout
	oldInterval := castPollInterval
	defer func() {
		connectTransfer = oldTransfer
		connectPost = oldPost
		castWakeTimeout = oldTimeout
		castPollInterval = oldInterval
	}()

	t.Run("matches device amid concurrent cluster updates", func(t *testing.T) {
		var uiMu sync.Mutex
		connectPost = func(_ *app, fn func()) {
			uiMu.Lock()
			defer uiMu.Unlock()
			fn()
		}
		castWakeTimeout = time.Second
		castPollInterval = 2 * time.Millisecond
		transferred := make(chan string, 1)
		connectTransfer = func(_ *spotify.Connect, target string) error {
			transferred <- target
			return nil
		}

		a := testConnectApp(t)
		a.castConnect = func(context.Context, *spotify.CastSpeaker) (string, error) {
			return "", nil
		}
		state := a.connect
		spk := &spotify.CastSpeaker{ID: "cast-1", Name: "Living Room"}

		stopUpdates := make(chan struct{})
		updatesDone := make(chan struct{})
		go func() {
			defer close(updatesDone)
			for i := 1; i <= 40; i++ {
				select {
				case <-stopUpdates:
					return
				default:
				}
				var raw string
				if i < 10 {
					raw = fmt.Sprintf(`{"active_device_id":"local","server_timestamp_ms":"%d","devices":{"local":{"name":"Vinyl"}}}`, 1000+i)
				} else {
					raw = fmt.Sprintf(`{"active_device_id":"local","server_timestamp_ms":"%d","devices":{"local":{"name":"Vinyl"},"cast-spotify-1":{"name":"Living Room"}}}`, 1000+i)
				}
				uiMu.Lock()
				a.connectEvent(spotify.ConnectEvent{Kind: "cluster", Data: json.RawMessage(raw)})
				uiMu.Unlock()
				time.Sleep(time.Millisecond)
			}
		}()

		err := a.wakeAndTransferCast(state, spk)
		close(stopUpdates)
		<-updatesDone
		if err != nil {
			t.Fatalf("wakeAndTransferCast failed: %v", err)
		}
		select {
		case got := <-transferred:
			if got != "cast-spotify-1" {
				t.Fatalf("transferred to %q, want %q", got, "cast-spotify-1")
			}
		default:
			t.Fatal("Transfer was not called")
		}
		uiMu.Lock()
		gotErr := state.err
		uiMu.Unlock()
		if gotErr != "" {
			t.Fatalf("state.err = %q, want empty after transfer", gotErr)
		}
	})

	t.Run("stopConnect cancels polling immediately", func(t *testing.T) {
		var uiMu sync.Mutex
		connectPost = func(_ *app, fn func()) {
			uiMu.Lock()
			defer uiMu.Unlock()
			fn()
		}
		castWakeTimeout = 3 * time.Second
		castPollInterval = 2 * time.Millisecond
		waking := make(chan struct{})
		transferCalled := false
		connectTransfer = func(*spotify.Connect, string) error {
			transferCalled = true
			return nil
		}

		a := testConnectApp(t)
		a.castConnect = func(context.Context, *spotify.CastSpeaker) (string, error) {
			close(waking)
			return "", nil
		}
		state := a.connect
		spk := &spotify.CastSpeaker{ID: "cast-2", Name: "Bedroom"}

		done := make(chan error, 1)
		go func() {
			done <- a.wakeAndTransferCast(state, spk)
		}()

		<-waking
		for i := 1; i <= 10; i++ {
			raw := fmt.Sprintf(`{"active_device_id":"local","server_timestamp_ms":"%d","devices":{"local":{"name":"Vinyl"}}}`, 2000+i)
			uiMu.Lock()
			a.connectEvent(spotify.ConnectEvent{Kind: "cluster", Data: json.RawMessage(raw)})
			uiMu.Unlock()
			time.Sleep(time.Millisecond)
		}

		uiMu.Lock()
		a.stopConnect()
		uiMu.Unlock()

		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("wakeAndTransferCast after stopConnect returned %v, want nil", err)
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatal("wakeAndTransferCast did not cancel promptly on stopConnect")
		}
		if transferCalled {
			t.Fatal("Transfer should not be called after stopConnect")
		}
	})
}

// A speaker that wakes but never shows up among Spotify's devices ends in an
// error rather than a transfer, and a failed wake keeps what made it fail.
func TestWakeAndTransferCastFailures(t *testing.T) {
	oldTransfer, oldPost := connectTransfer, connectPost
	oldTimeout, oldInterval := castWakeTimeout, castPollInterval
	defer func() {
		connectTransfer, connectPost = oldTransfer, oldPost
		castWakeTimeout, castPollInterval = oldTimeout, oldInterval
	}()
	connectPost = func(_ *app, fn func()) { fn() }
	castWakeTimeout = 15 * time.Millisecond
	castPollInterval = 3 * time.Millisecond
	transferred := false
	connectTransfer = func(*spotify.Connect, string) error {
		transferred = true
		return nil
	}
	spk := &spotify.CastSpeaker{ID: "cast-1", Name: "Kitchen"}

	a := testConnectApp(t)
	cause := errors.New("refused")
	a.castConnect = func(context.Context, *spotify.CastSpeaker) (string, error) { return "", cause }
	if err := a.wakeAndTransferCast(a.connect, spk); !errors.Is(err, cause) {
		t.Fatalf("wake error = %v, want it to wrap %v", err, cause)
	}

	b := testConnectApp(t)
	b.castConnect = func(context.Context, *spotify.CastSpeaker) (string, error) {
		return "cloud-cast-id-never-in-cluster", nil
	}
	if err := b.wakeAndTransferCast(b.connect, spk); err == nil {
		t.Fatal("a speaker that never joined the cluster reported no error")
	}
	if transferred {
		t.Fatal("transferred to a speaker that never joined the cluster")
	}
}

// isRemote reads cluster.Active under lock without racing, and stopConnect restores local volume before clearing a.connect and tolerates repeated calls.
func TestConnectCastIsRemoteLockingAndStopConnectOrder(t *testing.T) {
	a := testConnectApp(t)
	a.settings = defaultSettings()
	a.localVolume = 0.35
	a.hasLocalVolume = true
	a.volume = 0.8
	a.connect.volLocal = time.Now()
	a.connect.cluster.Active = "speaker-1"

	stopUpdates := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			raw := fmt.Sprintf(`{"active_device_id":"speaker-%d","server_timestamp_ms":"%d","devices":{"local":{"name":"Vinyl"}}}`, i%2, 3000+i)
			a.connectEvent(spotify.ConnectEvent{Kind: "cluster", Data: json.RawMessage(raw)})
		}
		close(stopUpdates)
	}()
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stopUpdates:
				return
			default:
				_ = a.isRemote()
			}
		}
	}()
	wg.Wait()

	state := a.connect
	state.mu.Lock()
	state.cluster.Active = "speaker-1"
	state.mu.Unlock()
	state.volLocal = time.Now()
	a.volume = 0.8

	a.stopConnect()
	if a.connect != nil {
		t.Fatal("stopConnect did not clear a.connect")
	}
	if !state.volLocal.IsZero() {
		t.Fatal("restoreLocalVolume ran after a.connect was cleared, leaving state.volLocal non-zero")
	}
	if a.volume != 0.35 || a.pl.IsRemote() {
		t.Fatalf("stopConnect did not restore local volume and local mode: volume=%v remote=%v", a.volume, a.pl.IsRemote())
	}

	// Double-stop and nil timer must not panic.
	a.stopConnect()
	a.connect = &connectState{done: make(chan struct{})}
	a.stopConnect()
}

// Switching devices successfully clears stale state.err across local, clustered Cast, non-Cast, applyConnect, and cluster active transitions.
func TestConnectCastClearsStaleErrorOnSwitch(t *testing.T) {
	oldTransfer := connectTransfer
	oldPost := connectPost
	defer func() {
		connectTransfer = oldTransfer
		connectPost = oldPost
	}()
	connectPost = func(_ *app, fn func()) { fn() }
	connectTransfer = func(*spotify.Connect, string) error { return nil }

	// 1. UI transfer paths: local playback, already-clustered Cast speaker, and non-Cast device.
	a := testConnectApp(t)
	a.speakersRegistry().Seed(
		&spotify.CastSpeaker{ID: "cast-1", SpotifyDeviceID: "cast-spotify-1", Name: "Living Room", Model: "Nest Audio", IP: "127.0.0.1", Port: 8009, ExpiresAt: time.Now().Add(time.Minute)},
	)
	a.connect.cluster = spotify.ConnectCluster{
		Active: "phone-1",
		Devices: map[string]spotify.ConnectDevice{
			"local":          {Name: "Vinyl"},
			"cast-spotify-1": {Name: "Living Room"},
			"phone-1":        {Name: "Phone"},
		},
	}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click(l10n.T("播放设备")); err != nil {
		t.Fatal(err)
	}
	tt.Frame()

	for _, label := range []string{l10n.T("此电脑 · ") + appName, "Living Room · Nest Audio", "✓ Phone"} {
		a.connect.err = "stale error"
		a.connect.open = true
		tt.Frame()
		if err := tt.Click(label); err != nil {
			t.Fatalf("clicking %q: %v", label, err)
		}
		tt.Frame()
		if len(a.connect.jobs) != 1 {
			t.Fatalf("%q queued %d jobs, want 1", label, len(a.connect.jobs))
		}
		if err := (<-a.connect.jobs)(); err != nil {
			t.Fatalf("%q job failed: %v", label, err)
		}
		if a.connect.err != "" {
			t.Fatalf("%q left stale state.err = %q", label, a.connect.err)
		}
	}

	// 2. applyConnect clears stale state.err on success.
	b := testConnectApp(t)
	b.connect.err = "stale error"
	replaceCmd := `{"type":"replace_state","state_ref":{"state_index":0,"paused":false},"state_machine":{"state_machine_id":"m1","tracks":[{"metadata":{"uri":"spotify:track:abc","name":"Song","duration":200000,"authors":[{"name":"Artist"}]}}],"states":[{"state_id":"s1","track":0,"transitions":{}}]}}`
	b.connectEvent(spotify.ConnectEvent{Kind: "command", Data: json.RawMessage(replaceCmd)})
	if b.connect.err != "" {
		t.Fatalf("applyConnect left stale state.err = %q", b.connect.err)
	}

	// 3. Cluster event clears stale state.err when active device changes, but preserves it when active device is unchanged.
	c := testConnectApp(t)
	c.connect.err = "stale error"
	c.connect.cluster.Active = "local"
	c.connectEvent(spotify.ConnectEvent{Kind: "cluster", Data: json.RawMessage(`{"active_device_id":"local","server_timestamp_ms":"100","devices":{"local":{"name":"Vinyl"}}}`)})
	if c.connect.err != "stale error" {
		t.Fatalf("unchanged active device cleared state.err = %q", c.connect.err)
	}
	c.connectEvent(spotify.ConnectEvent{Kind: "cluster", Data: json.RawMessage(`{"active_device_id":"speaker-2","server_timestamp_ms":"200","devices":{"local":{"name":"Vinyl"},"speaker-2":{"name":"Speaker"}}}`)})
	if c.connect.err != "" {
		t.Fatalf("active device switch left stale state.err = %q", c.connect.err)
	}
}

// Cast-to-Cluster matching prioritizes ID over case-insensitive name and filters hidden or non-playable devices in the device menu.
func TestConnectCastClusterDeviceMatching(t *testing.T) {
	falseVal := false
	trueVal := true
	devices := map[string]spotify.ConnectDevice{
		"local": {Name: "Living Room"},
		"name-dup": {Name: "living room", Capabilities: struct {
			Hidden  bool  `json:"hidden"`
			CanPlay *bool `json:"can_be_player"`
		}{CanPlay: &trueVal}},
		"id-match": {Name: "Renamed Speaker"},
		"hidden-dev": {Name: "Kitchen", Capabilities: struct {
			Hidden  bool  `json:"hidden"`
			CanPlay *bool `json:"can_be_player"`
		}{Hidden: true}},
		"no-play-dev": {Name: "Bedroom", Capabilities: struct {
			Hidden  bool  `json:"hidden"`
			CanPlay *bool `json:"can_be_player"`
		}{CanPlay: &falseVal}},
	}

	if got := matchCastClusterDevice(devices, "id-match", "local", "Living Room", true); got != "id-match" {
		t.Fatalf("ID match = %q, want id-match", got)
	}
	if got := matchCastClusterDevice(devices, "", "local", "LIVING ROOM", true); got != "name-dup" {
		t.Fatalf("case-insensitive name match = %q, want name-dup", got)
	}
	if got := matchCastClusterDevice(devices, "", "local", "Kitchen", true); got != "" {
		t.Fatalf("playableOnly matched hidden device %q", got)
	}
	if got := matchCastClusterDevice(devices, "", "local", "Bedroom", true); got != "" {
		t.Fatalf("playableOnly matched non-playable device %q", got)
	}

	// Verify menu display omits Hidden and CanPlay=false devices while showing matched Cast speaker as active.
	a := testConnectApp(t)
	a.speakersRegistry().Seed(
		&spotify.CastSpeaker{ID: "cast-1", SpotifyDeviceID: "id-match", Name: "Living Room", Model: "Nest Audio", IP: "127.0.0.1", Port: 8009, ExpiresAt: time.Now().Add(time.Minute)},
	)
	a.connect.cluster = spotify.ConnectCluster{
		Active:  "id-match",
		Devices: devices,
	}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click(l10n.T("播放设备")); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("✓ Living Room · Nest Audio") {
		t.Fatalf("expected active ID-matched Cast speaker in menu, got %q", tt.Texts())
	}
	if tt.HasText("Kitchen") || tt.HasText("Bedroom") {
		t.Fatalf("menu displayed hidden or non-playable device: %q", tt.Texts())
	}
}

type stubCastPacketConn struct {
	closed    atomic.Bool
	closeCh   chan struct{}
	closeOnce sync.Once
	reading   chan struct{}
	readOnce  sync.Once
}

func newStubCastPacketConn() *stubCastPacketConn {
	return &stubCastPacketConn{
		closeCh: make(chan struct{}),
		reading: make(chan struct{}),
	}
}

func (c *stubCastPacketConn) ReadFrom([]byte) (int, net.Addr, error) {
	c.readOnce.Do(func() { close(c.reading) })
	<-c.closeCh
	return 0, nil, net.ErrClosed
}

func (c *stubCastPacketConn) WriteTo(p []byte, _ net.Addr) (int, error) {
	if c.closed.Load() {
		return 0, net.ErrClosed
	}
	return len(p), nil
}

func (c *stubCastPacketConn) Close() error {
	c.closed.Store(true)
	c.closeOnce.Do(func() { close(c.closeCh) })
	return nil
}

func (c *stubCastPacketConn) LocalAddr() net.Addr              { return &net.UDPAddr{} }
func (c *stubCastPacketConn) SetDeadline(time.Time) error      { return nil }
func (c *stubCastPacketConn) SetReadDeadline(time.Time) error  { return nil }
func (c *stubCastPacketConn) SetWriteDeadline(time.Time) error { return nil }

// stopConnect unsubscribes the Cast speaker listener so subsequent Upsert calls do not invoke stale callbacks, cancels in-flight Cast wakeups, and closes Cast resources cleanly under -race.
func TestConnectStopUnsubscribesCastAndClosesResources(t *testing.T) {
	oldPost := connectPost
	defer func() { connectPost = oldPost }()
	connectPost = func(_ *app, fn func()) { fn() }

	pktConn := newStubCastPacketConn()
	a := testConnectApp(t)
	a.castRegistry = spotify.NewSpeakerRegistry(spotify.WithListenPacket(func(*net.ListenConfig, context.Context, string, string) (net.PacketConn, error) {
		return pktConn, nil
	}))
	a.speakersRegistry().Start()
	<-pktConn.reading

	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()
	go io.Copy(io.Discard, serverConn)

	dialed := make(chan struct{})
	a.castWakeup = spotify.NewCastWakeupClient(spotify.WithCastDialer(func(string) (net.Conn, error) {
		close(dialed)
		return clientConn, nil
	}))

	var subCalls atomic.Int32
	a.connect.castUnsub = a.speakersRegistry().Subscribe(func() {
		subCalls.Add(1)
	})
	a.speakersRegistry().Upsert(&spotify.CastSpeaker{ID: "spk-1", Name: "Living Room"})
	if got := subCalls.Load(); got != 1 {
		t.Fatalf("subscriber calls before stopConnect = %d, want 1", got)
	}

	wakeDone := make(chan error, 1)
	go func() {
		wakeDone <- a.wakeAndTransferCast(a.connect, &spotify.CastSpeaker{ID: "spk-1", Name: "Living Room", IP: "127.0.0.1", Port: 8009})
	}()
	<-dialed

	a.stopConnect()

	select {
	case err := <-wakeDone:
		if err != nil {
			t.Fatalf("wakeAndTransferCast after stopConnect returned %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("wakeAndTransferCast did not cancel promptly on stopConnect")
	}

	if !pktConn.closed.Load() {
		t.Fatal("stopConnect did not stop Cast SpeakerRegistry discovery")
	}

	a.speakersRegistry().Upsert(&spotify.CastSpeaker{ID: "spk-2", Name: "Kitchen"})
	if got := subCalls.Load(); got != 1 {
		t.Fatalf("subscriber invoked after stopConnect: got %d calls, want 1", got)
	}

	a.stopConnect()
}
