package spotify

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

// clearVideoFixture is a video podcast's manifest as the service writes
// it, cut down: not encrypted at all, its sound and two sizes of its
// picture in MP4, and in WebM, which plays nowhere here.
const clearVideoFixture = `{
 "contents": [{
  "encoding_id": "e1", "segment_length": 4, "start_time_millis": 0, "end_time_millis": 10500,
  "profiles": [
   {"id": 15, "file_type": "mp4", "audio_codec": "mp4a.40.2", "audio_bitrate": 96000, "max_bitrate": 99000},
   {"id": 5, "file_type": "mp4", "video_codec": "avc1.4d400d", "video_bitrate": 67677, "max_bitrate": 80000, "video_width": 320, "video_height": 180},
   {"id": 1, "file_type": "mp4", "video_codec": "avc1.4d4020", "video_bitrate": 415631, "max_bitrate": 500000, "video_width": 1280, "video_height": 720},
   {"id": 20, "file_type": "webm", "audio_codec": "opus", "audio_bitrate": 128000},
   {"id": 19, "file_type": "webm", "video_codec": "vp9", "video_bitrate": 233182, "video_width": 568, "video_height": 320}
  ]
 }],
 "initialization_template": "v1/e/{{profile_id}}/init.{{file_type}}?t=1",
 "segment_template": "v1/e/{{profile_id}}/{{segment_timestamp}}.{{file_type}}?t=1",
 "base_urls": ["https://video-fa.scdn.co/"]
}`

// A video podcast's video plays as it is, on every platform: its MP4s, no
// key, no licence; Sound names its sound's playlist.
func TestVideoHLSNotEncrypted(t *testing.T) {
	var m videoManifest
	if err := json.Unmarshal([]byte(clearVideoFixture), &m); err != nil {
		t.Fatal(err)
	}
	v, err := videoHLS(m)
	if err != nil {
		t.Fatal(err)
	}
	if v.License != "" || len(v.Playlists) != 3 || v.Sound != "p15.m3u8" {
		t.Fatalf("licence %q, %d playlists, sound %q", v.License, len(v.Playlists), v.Sound)
	}
	if strings.Contains(v.Master, "webm") || !strings.Contains(v.Master, "RESOLUTION=320x180") || !strings.Contains(v.Master, "RESOLUTION=1280x720") {
		t.Fatalf("master:\n%s", v.Master)
	}
	for name, pl := range v.Playlists {
		if strings.Contains(pl, "#EXT-X-KEY") {
			t.Fatalf("%s has a key:\n%s", name, pl)
		}
	}
	sound := v.Playlists[v.Sound]
	if !strings.Contains(sound, `#EXT-X-MAP:URI="https://video-fa.scdn.co/v1/e/15/init.mp4?t=1"`) || !strings.Contains(sound, "https://video-fa.scdn.co/v1/e/15/8.mp4?t=1") {
		t.Fatalf("sound:\n%s", sound)
	}
}

// episodeWith resolves an episode of trackType whose answer names the
// files of manifest, the service answering a video's manifest with video
// ("" for none). It returns what it resolved, the query asked with, and
// the paths asked for.
func episodeWith(t *testing.T, trackType, manifest, video string) (*Media, error, string, []string) {
	t.Helper()
	old := Client
	t.Cleanup(func() { Client = old; SetIdentity(Identity{}) })
	SetIdentity(Identity{Token: "t", ClientToken: "c"})
	var query string
	var paths []string
	Client = &http.Client{Transport: lyricsTransport(func(req *http.Request) (*http.Response, error) {
		paths = append(paths, req.URL.Path)
		status, body := 200, "#EXTM3U\n#EXT-X-ENDLIST\n"
		switch {
		case strings.HasPrefix(req.URL.Path, "/track-playback/v1/media/"):
			query = req.URL.RawQuery
			body = `{"media": {"spotify:episode:ep": {"item": {"track_type": "` + trackType + `", "metadata": {"duration": 1174000}, "manifest": {` + manifest + `}}}}}`
		case strings.HasPrefix(req.URL.Path, "/manifests/v9/"):
			if body = video; video == "" {
				status = 404
			}
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})}
	m, err := ResolveMedia("spotify:episode:ep")
	return m, err, query, paths
}

const (
	episodeVideo    = `"manifest_ids_video": [{"file_id": "d508fbf1975a5571b7763e6498d49622", "track_type": "VIDEO", "bitrate": null, "format": null}]`
	episodeExternal = `"file_urls_external": [{"file_url": "https://podcast.test/ep.mp3", "track_type": "AUDIO", "format": null}]`
	// Most episodes are in MP4s for Widevine only (cenc), which macOS's
	// FairPlay does not decrypt: a format the platform does not ask for.
	episodeCenc = `"file_ids_cenc_only": [{"file_id": "cc33", "bitrate": 128000, "track_type": "AUDIO"}]`
)

// A video podcast's episode, with no audio the platform plays, plays its
// video's sound, which is not encrypted, in step with the picture: rather
// than its audio where its podcast keeps it.
func TestEpisodePlaysItsVideosSound(t *testing.T) {
	m, err, query, paths := episodeWith(t, "MIXED", episodeCenc+","+episodeVideo+","+episodeExternal, clearVideoFixture)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(query, "manifestFileFormat="+externalFormat) || !strings.Contains(query, "manifestFileFormat="+videoFormat) {
		t.Fatalf("asked with %q", query)
	}
	if m.File != "" || !strings.Contains(m.Manifest, "/v1/e/15/0.mp4") || strings.Contains(m.Manifest, "#EXT-X-KEY") {
		t.Fatalf("file %q, manifest:\n%s", m.File, m.Manifest)
	}
	if m.Video != "d508fbf1975a5571b7763e6498d49622" || m.Duration.Milliseconds() != 1174000 {
		t.Fatalf("video %q, duration %v", m.Video, m.Duration)
	}
	for _, p := range paths {
		if strings.HasPrefix(p, "/sneaktables/") {
			t.Fatalf("asked for %s, the audio it cannot play", p)
		}
	}
}

// An episode with nothing else the platform plays plays where its podcast
// keeps it, as it is: as BBC's do, which Spotify keeps none of.
func TestEpisodePlaysWhereItsPodcastKeepsIt(t *testing.T) {
	m, err, _, _ := episodeWith(t, "AUDIO", episodeCenc+","+episodeExternal, "")
	if err != nil {
		t.Fatal(err)
	}
	if m.File != "https://podcast.test/ep.mp3" || m.Manifest != "" || m.Duration.Milliseconds() != 1174000 {
		t.Fatalf("file %q, manifest %q, duration %v", m.File, m.Manifest, m.Duration)
	}
}

// An episode the platform has a file of plays it, as a song does.
func TestEpisodeWithAFileOfThePlatformsPlaysIt(t *testing.T) {
	m, err, _, paths := episodeWith(t, "MIXED", `"`+mediaFormat+`": [{"file_id": "aa11", "bitrate": 128000, "track_type": "AUDIO"}],`+episodeVideo+","+episodeExternal, clearVideoFixture)
	if err != nil {
		t.Fatal(err)
	}
	if m.File != "" || !strings.Contains(strings.Join(paths, " "), "/sneaktables/v2/hls/1/aa11/") {
		t.Fatalf("file %q, asked %v", m.File, paths)
	}
	if m.Video != "d508fbf1975a5571b7763e6498d49622" {
		t.Fatalf("video %q", m.Video)
	}
}

// An episode whose video is encrypted does not play its sound: it would
// want the video's licence. Nor does one whose podcast keeps it at no
// address of the web. With nothing else, nothing plays here.
func TestEpisodeWithNothingThatPlaysHere(t *testing.T) {
	m, err, _, _ := episodeWith(t, "MIXED", episodeCenc+","+episodeVideo+","+episodeExternal, videoFixture)
	if err != nil || m.File != "https://podcast.test/ep.mp3" {
		t.Fatalf("encrypted video, kept elsewhere: %v, %+v", err, m)
	}
	for _, manifest := range []string{
		episodeCenc,
		episodeCenc + "," + episodeVideo,
		episodeCenc + `,"file_urls_external": [{"file_url": "", "track_type": "AUDIO"}, {"file_url": "ftp://podcast.test/ep.mp3"}]`,
	} {
		_, err, _, _ := episodeWith(t, "AUDIO", manifest, "")
		if !errors.Is(err, ErrNoAudioHere) {
			t.Fatalf("%s: %v, want ErrNoAudioHere", manifest, err)
		}
	}
	if _, err, _, _ := episodeWith(t, "MIXED", episodeCenc+","+episodeVideo, videoFixture); !errors.Is(err, ErrNoAudioHere) {
		t.Fatalf("encrypted video only: %v, want ErrNoAudioHere", err)
	}
}

// A song is asked for as before: where a podcast keeps it is no song's.
func TestSongsAskForNoPodcastsFile(t *testing.T) {
	_, query := resolveWith(t, `"`+mediaFormat+`": [{"file_id": "aa11", "bitrate": 128000}]`)
	if strings.Contains(query, externalFormat) {
		t.Fatalf("asked with %q", query)
	}
}
