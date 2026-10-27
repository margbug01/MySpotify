package spotify

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// videoFixture is a music video's manifest as the service writes it, cut
// down: 10.5 seconds in 4-second segments, the sound and two sizes of the
// picture in MP4 for Widevine and in MPEG-TS for FairPlay, and a size in
// WebM, which neither plays.
const videoFixture = `{
 "contents": [{
  "encoding_id": "51920b90cf1311f186341dcda1c4ee21", "segment_length": 4, "start_time_millis": 0, "end_time_millis": 10500,
  "profiles": [
   {"id": 15, "file_type": "mp4", "audio_codec": "mp4a.40.2", "audio_bitrate": 160000, "max_bitrate": 167732, "encryption_indices": [0, 1], "key_id": "wHco3T1WF+urSj9Y/q6Vow=="},
   {"id": 1, "file_type": "mp4", "video_codec": "avc1.4d4020", "video_bitrate": 6000000, "max_bitrate": 6106536, "video_width": 1280, "video_height": 720, "encryption_indices": [0, 1], "key_id": "wHco3T1WF+urSj9Y/q6Vow=="},
   {"id": 5, "file_type": "mp4", "video_codec": "avc1.4d400d", "video_bitrate": 300000, "max_bitrate": 372338, "video_width": 320, "video_height": 180, "encryption_indices": [0, 1], "key_id": "wHco3T1WF+urSj9Y/q6Vow=="},
   {"id": 28, "file_type": "ts", "audio_codec": "mp4a.40.2", "audio_bitrate": 160000, "max_bitrate": 219960, "encryption_indices": [2]},
   {"id": 22, "file_type": "ts", "video_codec": "avc1.4d4020", "video_bitrate": 6100000, "max_bitrate": 6254760, "video_width": 1280, "video_height": 720, "encryption_indices": [2]},
   {"id": 26, "file_type": "ts", "video_codec": "avc1.4d400d", "video_bitrate": 320000, "max_bitrate": 397432, "video_width": 320, "video_height": 180, "encryption_indices": [2]},
   {"id": 16, "file_type": "webm", "video_codec": "vp9", "video_bitrate": 8000000, "max_bitrate": 8478452, "video_width": 1920, "video_height": 1080, "encryption_indices": [0, 1]}
  ],
  "encryption_infos": [
   {"key_system": "widevine", "license_server_endpoint": "/widevine-license/v1/video/license", "encryption_data": "AAAAVXBzc2g="},
   {"key_system": "playready", "license_server_endpoint": "/playready-license/v1/video/license", "encryption_data": "AAAA"},
   {"key_system": "fairplay", "license_server_endpoint": "/fairplay-license/v2/video/license/51920b90cf1311f186341dcda1c4ee21?mpegTs=true", "encryption_data": "AAAA"}
  ]
 }],
 "initialization_template": "v1/e/{{profile_id}}/init.{{file_type}}?t=1",
 "segment_template": "v1/e/{{profile_id}}/{{segment_timestamp}}.{{file_type}}?t=1",
 "base_urls": ["https://video-fa.scdn.co/", "https://video-cf.spotifycdn.com/"]
}`

func fixtureVideo(t *testing.T) *Video {
	t.Helper()
	var m videoManifest
	if err := json.Unmarshal([]byte(videoFixture), &m); err != nil {
		t.Fatal(err)
	}
	v, err := videoHLS(m)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// The master playlist offers the sound and each size of the picture the key
// system plays, smallest first, and each playlist covers the whole video
// in its segments, the last as long as what is left.
func TestVideoHLS(t *testing.T) {
	v := fixtureVideo(t)
	sound := regexp.MustCompile(`#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio".*URI="([^"]+)"`).FindStringSubmatch(v.Master)
	sizes := regexp.MustCompile(`#EXT-X-STREAM-INF:.*RESOLUTION=(\d+x\d+).*AUDIO="audio"\n(\S+)`).FindAllStringSubmatch(v.Master, -1)
	if sound == nil || len(sizes) != 2 || sizes[0][1] != "320x180" || sizes[1][1] != "1280x720" {
		t.Fatalf("master:\n%s", v.Master)
	}
	if len(v.Playlists) != 3 {
		t.Fatalf("%d playlists, want the sound and 2 sizes", len(v.Playlists))
	}
	for _, name := range []string{sound[1], sizes[0][2], sizes[1][2]} {
		pl, ok := v.Playlists[name]
		if !ok {
			t.Fatalf("no playlist %s", name)
		}
		segs := regexp.MustCompile(`#EXTINF:([\d.]+),\n(\S+)`).FindAllStringSubmatch(pl, -1)
		if len(segs) != 3 || segs[0][1] != "4.000" || segs[2][1] != "2.500" || !strings.HasSuffix(pl, "#EXT-X-ENDLIST\n") {
			t.Fatalf("%s:\n%s", name, pl)
		}
		for i, at := range []string{"0", "4", "8"} {
			if !strings.HasPrefix(segs[i][2], "https://video-fa.scdn.co/v1/e/") || !strings.HasSuffix(segs[i][2], "/"+at+"."+videoFileType+"?t=1") {
				t.Fatalf("segment %d of %s: %s", i, name, segs[i][2])
			}
		}
	}
	if v.Duration.Milliseconds() != 10500 {
		t.Fatalf("duration %v", v.Duration)
	}
}

// ResolveMedia finds a song's music video in the entry of its video
// version, and plays the song's own sound: also when the song is relinked
// for the region, and the answer has no entry under the URI asked for. The
// video's files have no bitrate (null), which once failed the whole answer.
func TestResolveMediaFindsTheVideo(t *testing.T) {
	old := Client
	defer func() { Client = old }()
	SetIdentity(Identity{Token: "t", ClientToken: "c"})
	defer SetIdentity(Identity{})
	for _, asked := range []string{"spotify:track:song", "spotify:track:elsewhere"} {
		var media string
		Client = &http.Client{Transport: lyricsTransport(func(req *http.Request) (*http.Response, error) {
			body := "#EXTM3U\n#EXT-X-ENDLIST\n"
			if strings.HasPrefix(req.URL.Path, "/track-playback/v1/media/") {
				media = req.URL.RawQuery
				body = `{"media": {
				 "spotify:track:song": {"item": {"track_type": "AUDIO", "metadata": {"duration": 169000}, "manifest": {"` + mediaFormat + `": [{"file_id": "aa11", "bitrate": 128000}]}}, "video_version_uri": "spotify:track:video"},
				 "spotify:track:video": {"item": {"track_type": "VIDEO", "metadata": {"duration": 172000}, "manifest": {"` + mediaFormat + `": [{"file_id": "bb22", "bitrate": 128000}], "manifest_ids_video": [{"bitrate": null, "file_id": "160a9d67", "track_type": "VIDEO", "format": null}]}}}
				}}`
			} else if !strings.Contains(req.URL.Path, "/aa11/") {
				t.Errorf("%s: plays the video's sound: %s", asked, req.URL.Path)
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
		})}
		m, err := ResolveMedia(asked)
		if err != nil {
			t.Fatal(asked, err)
		}
		if !strings.Contains(media, "manifestFileFormat=manifest_ids_video") {
			t.Fatalf("%s: asked for %s, not the video", asked, media)
		}
		if m.Video != "160a9d67" || m.Duration.Milliseconds() != 169000 {
			t.Fatalf("%s: video %q, duration %v", asked, m.Video, m.Duration)
		}
	}
}
