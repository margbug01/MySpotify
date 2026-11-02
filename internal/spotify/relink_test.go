package spotify

import (
	"encoding/json"
	"testing"
)

// A track Spotify plays another of in the listener's country keeps the ID
// asked for, which the library has: hearts went by the one played, and
// showed songs the listener likes as not liked. As Spotify sent Heaven and
// Hell, asked for as 6kLC…, in place of 0zI7….
func TestConnectTrackKeepsTheIDAskedFor(t *testing.T) {
	var m ConnectMachine
	err := json.Unmarshal([]byte(`{"state_machine_id":"m","tracks":[
		{"metadata":{"uri":"spotify:track:0zI7PWQsDQkdoNH5UC0UOC","linked_from_uri":"spotify:track:6kLCHFM39wkFjOuyPGLGeQ","name":"Heaven and Hell","duration":281000}},
		{"metadata":{"uri":"spotify:track:0j2eDItputVxyMSgrQgySV","linked_from_uri":null,"name":"無緣","duration":256773}}],
		"states":[{"state_id":"a","track":0},{"state_id":"b","track":1}]}`), &m)
	if err != nil {
		t.Fatal(err)
	}
	relinked, _ := m.Track(0)
	if relinked.ID != "0zI7PWQsDQkdoNH5UC0UOC" || relinked.LinkedFrom != "6kLCHFM39wkFjOuyPGLGeQ" {
		t.Errorf("relinked: %+v", relinked)
	}
	if same, _ := m.Track(1); same.LinkedFrom != "" {
		t.Errorf("not relinked: %+v", same)
	}
	cluster := ConnectTrack{URI: "spotify:track:0zI7PWQsDQkdoNH5UC0UOC", Metadata: map[string]string{"requested_uri": "spotify:track:6kLCHFM39wkFjOuyPGLGeQ", "title": "Heaven and Hell"}}.track()
	if cluster.LinkedFrom != "6kLCHFM39wkFjOuyPGLGeQ" {
		t.Errorf("another device's: %+v", cluster)
	}
	asked := ConnectTrack{URI: "spotify:track:0j2eDItputVxyMSgrQgySV", Metadata: map[string]string{"requested_uri": "spotify:track:0j2eDItputVxyMSgrQgySV"}}.track()
	if asked.LinkedFrom != "" {
		t.Errorf("as asked: %+v", asked)
	}
}
