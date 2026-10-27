package spotify

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/coder/websocket"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// Dealer payloads can be plain JSON or gzip/base64; invalid data never becomes a command.
func TestConnectPayload(t *testing.T) {
	raw := []byte(`{"type":"replace_state","state_ref":null}`)
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	writer.Write(raw)
	writer.Close()
	encoded, _ := json.Marshal(base64.StdEncoding.EncodeToString(compressed.Bytes()))
	for _, input := range [][]byte{raw, encoded} {
		got, err := decodeConnectPayload(input)
		if err != nil || !bytes.Equal(got, raw) {
			t.Fatalf("decode: %s %v", got, err)
		}
	}
	if _, err := decodeConnectPayload([]byte(`"not base64"`)); err == nil {
		t.Fatal("accepted invalid payload")
	}
}

// Registration uses separate observer/receiver sessions and unregisters both on shutdown.
func TestConnectSessions(t *testing.T) {
	oldClient := Client
	idMu.Lock()
	oldID := current
	idMu.Unlock()
	defer func() { Client = oldClient; SetIdentity(oldID) }()
	SetIdentity(Identity{Token: "test-token", ClientToken: "test-client", Expires: time.Now().Add(time.Hour)})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		conn.Write(ctx, websocket.MessageText, []byte(`{"type":"message","uri":"hm://pusher/v1/connections/test","headers":{"Spotify-Connection-Id":"connection"}}`))
		for {
			_, _, err := conn.Read(ctx)
			if err != nil {
				return
			}
		}
	}))
	defer server.Close()
	address, _ := url.Parse(server.URL)
	var mu sync.Mutex
	var calls []string
	Client = &http.Client{Transport: lyricsTransport(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "dealer.spotify.com" {
			clone := req.Clone(req.Context())
			clone.URL.Scheme = address.Scheme
			clone.URL.Host = address.Host
			return http.DefaultTransport.RoundTrip(clone)
		}
		mu.Lock()
		calls = append(calls, req.Method+" "+req.URL.Path)
		mu.Unlock()
		if req.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("missing authorization")
		}
		body := `{"initial_seq_num":4}`
		if strings.Contains(req.URL.Path, "connect-state") {
			body = `{"devices":{}}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})}
	events := make(chan ConnectEvent, 16)
	client := NewConnect("test-device", "Vinyl", func(event ConnectEvent) { events <- event })
	client.Start()
	ready := map[string]bool{}
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	for !ready["observer_ready"] || !ready["receiver_ready"] {
		select {
		case event := <-events:
			if event.Err != nil {
				client.Close()
				t.Fatal(event.Err)
			}
			ready[event.Kind] = true
		case <-timeout.C:
			client.Close()
			t.Fatal("registration timeout")
		}
	}
	client.Close()
	mu.Lock()
	defer mu.Unlock()
	for _, want := range []string{"POST /track-playback/v1/devices", "PUT /connect-state/v1/devices/test-device-observer", "DELETE /track-playback/v1/devices/test-device", "DELETE /connect-state/v1/devices/test-device-observer"} {
		found := false
		for _, call := range calls {
			if call == want {
				found = true
			}
		}
		if !found {
			t.Errorf("missing %s", want)
		}
	}
}

// Server numbers may be strings, and an unsupported content URI is not silently played as a song.
func TestConnectModels(t *testing.T) {
	cluster, err := ParseConnectCluster([]byte(`{"cluster":{"active_device_id":"phone","server_timestamp_ms":"1200","devices":{"phone":{"name":"Phone","volume":"32768"}},"player_state":{"position_as_of_timestamp":"1000","duration":"200000","track":{"uri":"spotify:track:abc","metadata":{"title":"Song","artist_name":"Artist","image_url":"spotify:image:cover"}}}}}`))
	if err != nil || cluster.Active != "phone" || cluster.Track().ID != "abc" || cluster.Track().Album.PicURL != "https://i.scdn.co/image/cover" || cluster.Player.Position != 1000 {
		t.Fatalf("bad cluster: %v", err)
	}
	var command ConnectCommand
	json.Unmarshal([]byte(`{"type":"replace_state","state_ref":{"state_index":0,"paused":true},"state_machine":{"state_machine_id":"machine","tracks":[{"metadata":{"uri":"spotify:track:abc","name":"Song","duration":200000,"authors":[{"name":"Artist"}]}}],"states":[{"state_id":"state","track":0,"transitions":{"advance":{"state_index":0}}}]}}`), &command)
	track, err := command.Machine.Track(0)
	if err != nil || track.ID != "abc" || !command.Ref.Paused {
		t.Fatal("bad machine")
	}
	if _, err = command.Machine.Track(-1); err == nil {
		t.Fatal("negative state accepted")
	}
	command.Machine.Tracks[0].Metadata.URI = "spotify:ad:test"
	if _, err = command.Machine.Track(0); err == nil {
		t.Fatal("unsupported content accepted")
	}
}
