package spotify

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// resolveWith resolves a track whose answer names files under the formats
// manifest gives, as the service would, and returns the file played and the
// query asked with.
func resolveWith(t *testing.T, manifest string) (file, query string) {
	t.Helper()
	old := Client
	t.Cleanup(func() { Client = old; SetIdentity(Identity{}) })
	SetIdentity(Identity{Token: "t", ClientToken: "c"})
	Client = &http.Client{Transport: lyricsTransport(func(req *http.Request) (*http.Response, error) {
		body := "#EXTM3U\n#EXT-X-ENDLIST\n"
		switch {
		case strings.HasPrefix(req.URL.Path, "/track-playback/v1/media/"):
			query = req.URL.RawQuery
			body = `{"media": {"spotify:track:song": {"item": {"track_type": "AUDIO", "metadata": {"duration": 169000}, "manifest": {` + manifest + `}}}}}`
		case strings.HasPrefix(req.URL.Path, "/sneaktables/v2/hls/1/"):
			file = strings.Split(strings.TrimPrefix(req.URL.Path, "/sneaktables/v2/hls/1/"), "/")[0]
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})}
	if _, err := ResolveMedia("spotify:track:song"); err != nil {
		t.Fatal(err)
	}
	return file, query
}

// A track with no file in the platform's format plays from a fallback
// format, asked for in the same request; one that has both plays the
// platform's.
func TestResolveMediaFallsBack(t *testing.T) {
	old := mediaFallbacks
	defer func() { mediaFallbacks = old }()
	mediaFallbacks = []string{"file_ids_other"}

	file, query := resolveWith(t, `"`+mediaFormat+`": [], "file_ids_other": [{"file_id": "bb22", "bitrate": 128000}]`)
	if file != "bb22" {
		t.Fatalf("played %q, want the fallback's file", file)
	}
	if !strings.Contains(query, "manifestFileFormat="+mediaFormat+"&manifestFileFormat=file_ids_other") {
		t.Fatalf("asked with %q, want the format, then the fallback", query)
	}

	file, _ = resolveWith(t, `"`+mediaFormat+`": [{"file_id": "aa11", "bitrate": 128000}], "file_ids_other": [{"file_id": "bb22", "bitrate": 96000}]`)
	if file != "aa11" {
		t.Fatalf("played %q, want the platform's own file", file)
	}
}
