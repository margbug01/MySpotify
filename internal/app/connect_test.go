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
	"vinyl/internal/player"
	"vinyl/internal/spotify"
)

func testConnectApp(t *testing.T) *app {
	t.Helper()
	a := newTestApp()
	a.connect = &connectState{client: spotify.NewConnect("local", "Vinyl", nil), observer: true, receiver: true, jobs: make(chan func() error, 32), done: make(chan struct{})}
	t.Cleanup(a.connect.client.Close)
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

// A revoked state or disconnected receiver must immediately stop local playback.
func TestConnectRevokesOwnership(t *testing.T) {
	for _, kind := range []string{"command", "receiver_offline"} {
		a := testConnectApp(t)
		a.pl.Hold(time.Second)
		a.connect.ref = &spotify.ConnectRef{Index: 0}
		a.connectEvent(spotify.ConnectEvent{Kind: kind, Data: json.RawMessage(`{"type":"replace_state","state_ref":null}`)})
		if a.pl.Playing() || a.connect.ref != nil {
			t.Fatalf("%s did not revoke playback", kind)
		}
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
	if err := tt.Click("播放设备"); err != nil {
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
	if err := tt.Click("此电脑 · " + appName); err != nil {
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
	a.connectEvent(spotify.ConnectEvent{Kind: "cluster", Data: json.RawMessage(`{"active_device_id":"local","player_state":{"duration":"200000","track":{"uri":"spotify:track:one","uid":"u1","metadata":{"title":"One"}},"next_tracks":[{"uri":"spotify:delimiter","uid":"delimiter0","metadata":{"hidden":"true"}},{"uri":"spotify:track:two","uid":"u2","metadata":{"title":"Two"}},{"uri":"spotify:track:three","uid":"u3"},{"uri":"spotify:track:one","uid":"u1","metadata":{"hidden":"true","title":"One"}}]}}`)})
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
