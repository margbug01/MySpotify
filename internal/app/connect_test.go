package app

import (
	"encoding/json"
	"github.com/egoist/mygo/ui"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"vinyl/internal/l10n"
	"vinyl/internal/player"
	"vinyl/internal/spotify"
)

func testConnectApp(t *testing.T) *app {
	t.Helper()
	a := newTestApp()
	a.castRegistry = spotify.NewSpeakerRegistry(spotify.WithNoopDiscovery())
	a.connect = &connectState{client: spotify.NewConnect("local", "Vinyl", nil), observer: true, receiver: true, jobs: make(chan func() error, 32), done: make(chan struct{})}
	client := a.connect.client
	t.Cleanup(func() {
		a.closeCastResources()
		client.Close()
	})
	a.pl.Control = a.connectControl
	a.pl.Cue([]spotify.Track{{ID: "one", Name: "One", Duration: 200000}})
	return a
}

// Remote playback is mirrored without starting local audio, and old cluster snapshots cannot win.
func TestConnectRemoteOwnership(t *testing.T) {
	a := testConnectApp(t)
	a.pl.Hold(time.Second)
	a.connectEvent(spotify.ConnectEvent{Kind: "cluster", Data: json.RawMessage(`{"active_device_id":"phone","server_timestamp_ms":"2000","player_state":{"is_playing":true,"is_paused":false,"position_as_of_timestamp":"12000","timestamp":"2000","duration":"200000","track":{"uri":"spotify:track:two","metadata":{"title":"Two"}}}}`)})
	if !a.pl.IsRemote() || a.pl.Current().ID != "two" || !a.pl.Playing() {
		t.Fatal("remote playback was not mirrored")
	}
	a.connectEvent(spotify.ConnectEvent{Kind: "cluster", Data: json.RawMessage(`{"active_device_id":"old","server_timestamp_ms":"1000"}`)})
	if a.connect.cluster.Active != "phone" {
		t.Fatal("stale cluster replaced current device")
	}
	a.pl.TogglePlay()
	a.pl.Seek(3 * time.Second)
	a.pl.Next()
	if len(a.connect.jobs) != 3 || !a.pl.IsRemote() {
		t.Fatal("remote controls did not route to Connect")
	}
	a.connectEvent(spotify.ConnectEvent{Kind: "command", Data: json.RawMessage(`{"type":"replace_state","state_ref":null}`)})
	if !a.pl.IsRemote() || !a.pl.Playing() {
		t.Fatal("late local revocation erased remote playback")
	}
	<-a.connect.jobs
	a.connect.observer = false
	a.pl.TogglePlay()
	if len(a.connect.jobs) != 3 || a.toast == "" {
		t.Fatal("offline remote controls should wait, not start locally")
	}
}

// A revoked state must immediately stop local playback.
func TestConnectRevokesOwnership(t *testing.T) {
	a := testConnectApp(t)
	a.pl.Hold(time.Second)
	a.connect.ref = &spotify.ConnectRef{Index: 0}
	a.connectEvent(spotify.ConnectEvent{Kind: "command", Data: json.RawMessage(`{"type":"replace_state","state_ref":null}`)})
	if a.pl.Playing() || a.connect.ref != nil {
		t.Fatal("a revoked state did not stop playback")
	}
}

// The dealer dropping leaves the song playing here, its controls here until
// the receiver is back: it stopped the song where it was, nothing in the log,
// each time a link or a proxy dropped the dealer (2026-11-10, and a listener's).
func TestConnectDealerDropPlaysOn(t *testing.T) {
	a := testConnectApp(t)
	a.pl.Hold(time.Second)
	a.connect.ref = &spotify.ConnectRef{Index: 0}
	a.connectEvent(spotify.ConnectEvent{Kind: "receiver_offline"})
	if !a.pl.Playing() || a.connect.ref != nil {
		t.Fatalf("dealer dropped: playing %v, ref %v; want playing on, the ref dropped", a.pl.Playing(), a.connect.ref)
	}
	a.pl.TogglePlay()
	if len(a.connect.jobs) != 0 || a.toast != "" {
		t.Fatalf("toggled offline: %d Connect jobs, toast %q; want it done here", len(a.connect.jobs), a.toast)
	}
}

// A saved local song needs a play context, not resume against an empty server state.
func TestConnectResumeAndPlayRouting(t *testing.T) {
	a := testConnectApp(t)
	a.pl.TogglePlay()
	if len(a.connect.jobs) != 1 || a.pl.Playing() {
		t.Fatal("saved session was played locally before the server accepted it")
	}
	request := player.QueueRequest{Tracks: []spotify.Track{{ID: "a"}, {ID: "b"}}, Start: 1}
	body := connectPlayBody(request)
	tracks := body["context"].(map[string]any)["pages"].([]any)[0].(map[string]any)["tracks"].([]map[string]string)
	if len(tracks) != 2 || tracks[1]["uri"] != "spotify:track:b" || body["options"].(map[string]any)["skip_to"].(map[string]any)["track_index"] != 1 {
		t.Fatal("play context lost tracks or starting index")
	}
	a.pl.PlayQueue(request.Tracks, 1)
	if len(a.connect.jobs) != 2 {
		t.Fatal("new queue was not routed")
	}
}

// Extended state machines update the visible queue without restarting the current audio.
func TestConnectAdoptsExtendedQueue(t *testing.T) {
	a := testConnectApp(t)
	json.Unmarshal([]byte(`{"state_machine_id":"old","states":[{"state_id":"old-state","track":0}],"tracks":[{"metadata":{"uri":"spotify:track:one"}}]}`), &a.connect.machine)
	a.connect.ref = &spotify.ConnectRef{Index: 0}
	a.connectEvent(spotify.ConnectEvent{Kind: "report", Data: json.RawMessage(`{"updated_state_ref":{"state_index":0},"state_machine":{"state_machine_id":"new","states":[{"state_id":"new-state","track":0,"transitions":{"advance":{"state_index":1}}},{"state_id":"next","track":1,"transitions":{"advance":{"state_index":0}}}],"tracks":[{"metadata":{"uri":"spotify:track:one"}},{"metadata":{"uri":"spotify:track:two"}}]}}`)})
	if a.connect.machine.ID != "new" || len(a.pl.Queue()) != 2 || a.pl.Current().ID != "one" || a.pl.Loading() {
		t.Fatal("extended queue not adopted without a reload")
	}
}

// Choosing an online device queues a transfer; hidden observer devices are not selectable.
func TestConnectDeviceMenu(t *testing.T) {
	a := testConnectApp(t)
	cluster, err := spotify.ParseConnectCluster([]byte(`{"active_device_id":"phone","devices":{"phone":{"name":"My Phone"},"hidden":{"name":"Observer","capabilities":{"hidden":true}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	a.connect.cluster = cluster
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click(l10n.T("播放设备")); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if dir := os.Getenv("NM_FRAMES"); dir != "" {
		file, err := os.Create(filepath.Join(dir, "connect-menu.png"))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(file, tt.Image()); err != nil {
			file.Close()
			t.Fatal(err)
		}
		file.Close()
	}
	if err := tt.Click(l10n.T("此电脑 · ") + appName); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(a.connect.jobs) != 1 || a.connect.open {
		t.Fatal("device selection did not queue transfer and close menu")
	}
}

type connectTransport func(*http.Request) (*http.Response, error)

func (f connectTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Spotify's machine holds only the track playing and the next: the queue
// shown is the cluster's, as phones show it, without its hidden repeats; a
// report does not cut it back to two; and a track of it is skipped to in
// its place, not played as a queue of its own.
func TestConnectQueueIsTheClusters(t *testing.T) {
	a := testConnectApp(t)
	machine := `{"state_machine_id":"m","states":[{"state_id":"s1","track":0,"transitions":{"advance":{"state_index":1}}},{"state_id":"s2","track":1}],"tracks":[{"metadata":{"uri":"spotify:track:one","name":"One"}},{"metadata":{"uri":"spotify:track:two","name":"Two"}}]}`
	json.Unmarshal([]byte(machine), &a.connect.machine)
	a.connect.ref = &spotify.ConnectRef{Index: 0}
	a.connect.known = map[string]spotify.Track{"spotify:track:three": {ID: "three", Name: "Three"}}
	a.connectEvent(spotify.ConnectEvent{Kind: "cluster", Data: json.RawMessage(`{"active_device_id":"local","player_state":{"duration":"200000","track":{"uri":"spotify:track:one","uid":"u1","metadata":{"title":"One","artist_name":"Singer"}},"next_tracks":[{"uri":"spotify:delimiter","uid":"delimiter0","metadata":{"hidden":"true"}},{"uri":"spotify:track:two","uid":"u2","metadata":{"title":"Two","artist_name":"Singer"}},{"uri":"spotify:track:three","uid":"u3"},{"uri":"spotify:track:one","uid":"u1","metadata":{"hidden":"true","title":"One"}}]}}`)})
	a.connectEvent(spotify.ConnectEvent{Kind: "report", Data: json.RawMessage(`{"updated_state_ref":{"state_index":0},"state_machine":` + machine + `}`)})
	var got []string
	for _, tr := range a.pl.Queue() {
		got = append(got, tr.Name+"/"+tr.UID)
	}
	if strings.Join(got, " ") != "One/u1 Two/u2 Three/u3" {
		t.Fatalf("queue %v, not the cluster's", got)
	}

	oldClient := spotify.Client
	oldIdentity, _ := spotify.CurrentIdentity()
	defer func() { spotify.Client = oldClient; spotify.SetIdentity(oldIdentity) }()
	spotify.SetIdentity(spotify.Identity{Token: "test", Expires: time.Now().Add(time.Hour)})
	var sent struct {
		Command struct {
			Endpoint string            `json:"endpoint"`
			Track    map[string]string `json:"track"`
		} `json:"command"`
	}
	spotify.Client = &http.Client{Transport: connectTransport(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "/player/command/") {
			json.NewDecoder(r.Body).Decode(&sent)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("{}")), Header: http.Header{}}, nil
	})}
	a.pl.Jump(2)
	if len(a.connect.jobs) != 1 || a.pl.Current().ID != "one" {
		t.Fatal("jump was not routed to Connect")
	}
	if err := (<-a.connect.jobs)(); err != nil {
		t.Fatal(err)
	}
	if sent.Command.Endpoint != "skip_next" || sent.Command.Track["uid"] != "u3" || sent.Command.Track["uri"] != "spotify:track:three" {
		t.Fatalf("jump sent %+v", sent.Command)
	}
}

// The machine names some tracks' artists only by URI: what plays shows the
// artist the cluster names, or else the one named before.
func TestConnectNamesTheArtist(t *testing.T) {
	for _, c := range []struct{ name, meta, want string }{
		{"cluster", `"title":"One","artist_name":"Singer"`, "Singer"},
		{"known", `"title":"One"`, "Known"},
	} {
		a := testConnectApp(t)
		json.Unmarshal([]byte(`{"state_machine_id":"m","states":[{"state_id":"s1","track":0}],"tracks":[{"metadata":{"uri":"spotify:track:one","name":"One","authors":[{"uri":"spotify:artist:s"}]}}]}`), &a.connect.machine)
		a.connect.ref = &spotify.ConnectRef{Index: 0}
		a.connect.known = map[string]spotify.Track{"spotify:track:one": {ID: "one", Name: "One", Artists: []spotify.Artist{{Name: "Known"}}}}
		a.connectEvent(spotify.ConnectEvent{Kind: "cluster", Data: json.RawMessage(`{"active_device_id":"local","player_state":{"duration":"200000","track":{"uri":"spotify:track:one","uid":"u1","metadata":{` + c.meta + `}}}}`)})
		if got := a.pl.Current().ArtistNames(); got != c.want {
			t.Errorf("%s: artist %q, want %q", c.name, got, c.want)
		}
	}
}

// A report answered once the song has moved on takes the new machine, at the
// state of the song now: the machine was dropped, and each report after named
// a state Spotify had replaced, turned down with a 400 every five seconds.
func TestConnectReportAnsweredAfterTheSongEnded(t *testing.T) {
	a := testConnectApp(t)
	json.Unmarshal([]byte(`{"state_machine_id":"old","states":[{"state_id":"old-one","track":0,"transitions":{"advance":{"state_index":1}}},{"state_id":"old-two","track":1}],"tracks":[{"metadata":{"uri":"spotify:track:one"}},{"metadata":{"uri":"spotify:track:two"}}]}`), &a.connect.machine)
	a.connect.ref = &spotify.ConnectRef{Index: 1}
	a.pl.Cue([]spotify.Track{{ID: "two", Name: "Two", Duration: 200000}})

	a.connectEvent(spotify.ConnectEvent{Kind: "report", Data: json.RawMessage(`{"updated_state_ref":{"state_index":0},"state_machine":{"state_machine_id":"new","states":[{"state_id":"new-one","track":0,"transitions":{"advance":{"state_index":1}}},{"state_id":"new-two","track":1}],"tracks":[{"metadata":{"uri":"spotify:track:one"}},{"metadata":{"uri":"spotify:track:two"}}]}}`)})
	if a.connect.machine.ID != "new" || a.connect.ref.Index != 1 {
		t.Fatalf("on machine %q state %d, want new at 1", a.connect.machine.ID, a.connect.ref.Index)
	}

	var reported any
	connectReport = func(_ *spotify.Connect, ref any, _, _ int64, _ bool) error {
		reported = ref
		return nil
	}
	defer func() { connectReport = (*spotify.Connect).Report }()
	a.reportConnect()
	if err := (<-a.connect.jobs)(); err != nil {
		t.Fatal(err)
	}
	if got := reported.(map[string]any)["state_id"]; got != "new-two" {
		t.Errorf("reported state %v, want new-two", got)
	}
}

// A report Spotify turns down says nothing to the listener. Refused for good,
// the device stops reporting and its controls act here until it registers
// again, but a refusal of a state since replaced changes nothing.
func TestConnectReportTurnedDown(t *testing.T) {
	a := testConnectApp(t)
	json.Unmarshal([]byte(`{"state_machine_id":"m","states":[{"state_id":"s1","track":0}],"tracks":[{"metadata":{"uri":"spotify:track:one"}}]}`), &a.connect.machine)
	a.connect.ref = &spotify.ConnectRef{Index: 0}

	connectReport = func(*spotify.Connect, any, int64, int64, bool) error {
		return &spotify.ConnectError{Status: http.StatusBadRequest}
	}
	defer func() { connectReport = (*spotify.Connect).Report }()
	a.reportConnect()
	if err := (<-a.connect.jobs)(); err != nil {
		t.Errorf("the worker would toast: %v", err)
	}

	a.connectTurnedDown(a.connect, "replaced")
	if a.connect.ref == nil {
		t.Fatal("a refusal of a state since replaced gave the device up")
	}

	a.connectTurnedDown(a.connect, "s1")
	a.reportConnect()
	if a.connect.ref != nil || len(a.connect.jobs) != 0 {
		t.Fatal("went on reporting a state Spotify refuses")
	}
	a.pl.TogglePlay()
	if len(a.connect.jobs) != 0 || !a.pl.Playing() {
		t.Fatal("the play button asked Spotify, which turns it down, rather than playing here")
	}

	a.connectEvent(spotify.ConnectEvent{Kind: "receiver_ready"})
	a.pl.TogglePlay()
	if len(a.connect.jobs) != 1 {
		t.Fatal("registered again, the controls should go through Connect")
	}
}

// Played through Connect, a song played next or queued was put only in
// Vinyl's queue: Spotify, which says what plays next, never heard of it,
// and the cluster that followed dropped it. It is now put in Spotify's.
func TestConnectQueuesThroughSpotify(t *testing.T) {
	a := testConnectApp(t)
	a.connect.ref = &spotify.ConnectRef{Index: 0}

	oldClient := spotify.Client
	oldIdentity, _ := spotify.CurrentIdentity()
	defer func() { spotify.Client = oldClient; spotify.SetIdentity(oldIdentity) }()
	spotify.SetIdentity(spotify.Identity{Token: "test", Expires: time.Now().Add(time.Hour)})
	var sent []string
	spotify.Client = &http.Client{Transport: connectTransport(func(r *http.Request) (*http.Response, error) {
		if !strings.Contains(r.URL.Path, "/player/command/") {
			return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("{}")), Header: http.Header{}}, nil
		}
		var body struct {
			Command struct {
				Endpoint string `json:"endpoint"`
				Track    struct {
					URI string `json:"uri"`
				} `json:"track"`
			} `json:"command"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		sent = append(sent, body.Command.Endpoint+" "+body.Command.Track.URI)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("{}")), Header: http.Header{}}, nil
	})}

	// The queue here is what is left of the playlist: "two", further down
	// it, was turned away as queued already.
	a.pl.Cue([]spotify.Track{{ID: "one", Duration: 200000}, {ID: "two", Duration: 200000}})
	a.pl.PlayNext(spotify.Track{ID: "next"})
	if !a.pl.AddToQueue(spotify.Track{ID: "two"}) {
		t.Fatal("a song further down the playlist was turned away as queued")
	}
	a.pl.PlayNext(spotify.Track{ID: "one"}) // the one playing: nothing to do
	for len(a.connect.jobs) > 0 {
		if err := (<-a.connect.jobs)(); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Join(sent, ", ") != "add_to_queue spotify:track:next, add_to_queue spotify:track:two" {
		t.Fatalf("sent %v", sent)
	}
	if len(a.pl.Queue()) != 2 {
		t.Fatal("queued in Vinyl's queue too, which Spotify's replaces")
	}

	// Not played through Connect, the songs are queued here.
	a.connect.ref = nil
	a.pl.PlayNext(spotify.Track{ID: "here"})
	if len(a.connect.jobs) != 0 || len(a.pl.Queue()) != 3 {
		t.Fatal("a song played here was sent to Spotify")
	}
}

// While Connect reconnects, a song played next from its menu is not
// queued: the menu said it would play next all the same, over the toast
// saying to wait.
func TestConnectQueueWhileReconnectingSaysSo(t *testing.T) {
	a := testConnectApp(t)
	a.settings = defaultSettings()
	a.connect.ref = &spotify.ConnectRef{Index: 0}
	a.connect.observer = false
	a.playlists["42"] = &spotify.Playlist{ID: "42", Name: "List", Tracks: []spotify.Track{{ID: "2", Name: "Two", Duration: 200000}}}
	a.route = route{page: pagePlaylist, playlist: "42"}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	for _, item := range []string{"下一首播放", "添加到待播清单"} {
		if err := tt.RightClick("Two"); err != nil {
			t.Fatal(err)
		}
		if err := tt.ChooseMenuItem(item); err != nil {
			t.Fatalf("%v; the menu shows %q", err, tt.Menu())
		}
		if !tt.HasText("Connect 正在重连，请稍后") || tt.HasText("将在下一首播放") || tt.HasText("已添加到待播清单") {
			t.Fatalf("%s did not say to wait, or said it queued", item)
		}
		tt.Advance(10 * time.Second)
		tt.Frame()
	}
	if len(a.connect.jobs) != 0 || len(a.pl.Queue()) != 1 {
		t.Fatal("queued while Connect reconnects")

	}
}

// Discovered Cast speakers keep a stable order across frames and when joining the Connect cluster.
func TestConnectCastDevicesMenuOrderAndTransfer(t *testing.T) {
	a := testConnectApp(t)
	a.speakersRegistry().Seed(
		&spotify.CastSpeaker{ID: "id-bedroom", Name: "Bedroom", Model: "Google Home Mini", IP: "192.0.2.10", Port: 8009, ExpiresAt: time.Now().Add(time.Minute)},
		&spotify.CastSpeaker{ID: "id-group", Name: "Home Group", Model: "Google Cast Group", IP: "192.0.2.10", Port: 32072, ExpiresAt: time.Now().Add(time.Minute)},
		&spotify.CastSpeaker{ID: "id-kitchen", Name: "Kitchen", Model: "Google Nest Mini", IP: "192.0.2.11", Port: 8009, ExpiresAt: time.Now().Add(time.Minute)},
	)
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click(l10n.T("播放设备")); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 15; i++ {
		tt.Frame()
		got := a.speakersRegistry().Speakers()
		if len(got) != 3 || got[0].Name != "Bedroom" || got[1].Name != "Home Group" || got[2].Name != "Kitchen" {
			t.Fatalf("frame %d: unexpected speaker order: %+v", i, got)
		}
	}
	if err := tt.Click("Home Group · Google Cast Group"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(a.connect.jobs) != 1 || a.connect.open {
		t.Fatal("clicking Cast speaker did not queue wakeAndTransferCast job and close menu")
	}
}
