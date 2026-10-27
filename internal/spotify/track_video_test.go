package spotify

import (
	"encoding/json"
	"testing"
)

// A track Spotify has a music video of says so, as the lists give it: a
// playlist, an album, an artist's page or the library.
func TestTracksKnowTheirVideos(t *testing.T) {
	var items []*gqlTrack
	err := json.Unmarshal([]byte(`[
		{"__typename": "Track", "uri": "spotify:track:a", "name": "A", "trackDuration": {"totalMilliseconds": 1000}, "playability": {"playable": true},
		 "associationsV3": {"audioAssociations": {"__typename": "TrackAudioAssociationPage", "items": []}, "videoAssociations": {"totalCount": 1}}},
		{"__typename": "Track", "uri": "spotify:track:b", "name": "B", "duration": {"totalMilliseconds": 1000}, "playability": {"playable": true},
		 "associationsV3": {"audioAssociations": {"totalCount": 0}, "videoAssociations": {"totalCount": 0}}},
		{"__typename": "Track", "uri": "spotify:track:c", "name": "C", "playability": {"playable": true}}
	]`), &items)
	if err != nil {
		t.Fatal(err)
	}
	got := tracks(items, nil)
	if len(got) != 3 || !got[0].Video || got[1].Video || got[2].Video {
		t.Fatalf("videos: %+v", got)
	}
}
