package app

import (
	"encoding/json"
	"testing"

	"vinyl/internal/spotify"
)

// connectQueueApp is filterApp's playlist wired for Connect, whose cluster
// says another device, phone, is the active one, playing nothing.
func connectQueueApp(t *testing.T) *app {
	t.Helper()
	a := filterApp()
	a.connect = &connectState{client: spotify.NewConnect("local", "Vinyl", nil), observer: true, receiver: true, jobs: make(chan func() error, 32), done: make(chan struct{})}
	t.Cleanup(a.connect.client.Close)
	a.pl.Control = a.connectControl
	a.pl.Cue([]spotify.Track{{ID: "one", Name: "One", Duration: 200000}})
	a.connectEvent(spotify.ConnectEvent{Kind: "cluster", Data: json.RawMessage(`{"active_device_id":"phone","server_timestamp_ms":"1000"}`)})
	return a
}

// The matches of a filter played while another device is the cluster's
// active one go to it as Spotify's play command: the matches in the
// playlist's order on the command's track page, at the track asked, with
// no context of their own, so the device plays the few and not the
// playlist whole again.
func TestConnectFilteredQueuePlaysInOrder(t *testing.T) {
	defer func(old func(*spotify.Connect, string, string, map[string]any) error) { connectCommand = old }(connectCommand)
	var sent []struct {
		target, endpoint string
		extra            map[string]any
	}
	connectCommand = func(_ *spotify.Connect, target, endpoint string, extra map[string]any) error {
		sent = append(sent, struct {
			target, endpoint string
			extra            map[string]any
		}{target, endpoint, extra})
		return nil
	}
	a := connectQueueApp(t)
	pl := a.playlists["42"]
	matches := tracksAt(pl.Tracks, filterTracks(pl.Tracks, "叶"))
	a.pl.PlayQueue(matches, firstPlayable(matches))
	for len(a.connect.jobs) > 0 {
		(<-a.connect.jobs)()
	}
	if len(sent) != 1 || sent[0].endpoint != "play" || sent[0].target != "phone" {
		t.Fatalf("the filtered queue went as %v", sent)
	}
	body := sent[0].extra
	if body["play_origin"] == nil {
		t.Fatal("the play command carried no origin")
	}
	ctx := body["context"].(map[string]any)
	if ctx["uri"] != "" {
		t.Fatalf("the play command carried a context %q, which the device would read the playlist whole from", ctx["uri"])
	}
	tracks := ctx["pages"].([]any)[0].(map[string]any)["tracks"].([]map[string]string)
	if len(tracks) != len(matches) {
		t.Fatalf("the track page held %d of the %d matches", len(tracks), len(matches))
	}
	for i, tr := range tracks {
		if tr["uri"] != matches[i].URI() {
			t.Fatalf("the track page held %q where the playlist's order has %q", tr["uri"], matches[i].URI())
		}
	}
	skipTo := body["options"].(map[string]any)["skip_to"].(map[string]any)
	if skipTo["track_index"] != firstPlayable(matches) {
		t.Fatalf("the play started at %v, want the first playable %d", skipTo["track_index"], firstPlayable(matches))
	}
}

// Shuffled, the shuffle is asked of the device before the queue, whose
// command starts where the shuffle picked: the device shuffles the matches
// itself, from that one.
func TestConnectFilteredQueuePlaysShuffled(t *testing.T) {
	defer func(old func(*spotify.Connect, string, string, map[string]any) error) { connectCommand = old }(connectCommand)
	var asked []string
	var startIndex any
	connectCommand = func(_ *spotify.Connect, _, endpoint string, extra map[string]any) error {
		asked = append(asked, endpoint)
		if endpoint == "play" {
			startIndex = extra["options"].(map[string]any)["skip_to"].(map[string]any)["track_index"]
		}
		return nil
	}
	defer func(old func(int) int) { randomPick = old }(randomPick)
	randomPick = func(int) int { return 1 }
	a := connectQueueApp(t)
	pl := a.playlists["42"]
	matches := tracksAt(pl.Tracks, filterTracks(pl.Tracks, "叶"))
	a.pl.ToggleShuffle()
	a.pl.PlayQueue(matches, randomPlayable(matches))
	for len(a.connect.jobs) > 0 {
		(<-a.connect.jobs)()
	}
	if len(asked) != 2 || asked[0] != "set_shuffling_context" || asked[1] != "play" {
		t.Fatalf("asked %v, want the shuffle before the queue", asked)
	}
	if startIndex != randomPlayable(matches) {
		t.Fatalf("the play started at %v, want where the shuffle picked %d", startIndex, randomPlayable(matches))
	}
}
