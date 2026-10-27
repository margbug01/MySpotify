package spotify

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"
)

// Media is how a track plays in the web player: an HLS manifest of the
// encrypted MP4 the CDN serves, which the browser's key system decrypts
// with a licence Spotify issues to the web player: Widevine, or FairPlay
// on macOS (see mediaFormat).
type Media struct {
	Manifest string // HLS, its init and segments signed CDN URLs with byte ranges
	Duration time.Duration
	Bitrate  int // bits per second
}

type mediaFile struct {
	FileID    string   `json:"file_id"`
	Format    looseInt `json:"format"`
	Bitrate   looseInt `json:"bitrate"`
	TrackType string   `json:"track_type"` // AUDIO, or a video podcast's VIDEO
}

// looseInt accepts 11 and "11": the service sends either.
type looseInt int

func (n *looseInt) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		v, err := strconv.Atoi(s)
		*n = looseInt(v)
		return err
	}
	var v int
	err := json.Unmarshal(b, &v)
	*n = looseInt(v)
	return err
}

// ResolveMedia returns the manifest of the lowest bitrate of uri, a
// track's or a podcast episode's: 128k, which a free account is licensed to
// play; the licence of the 256k one it is refused (403).
func ResolveMedia(uri string) (*Media, error) {
	raw, err := spGet("/track-playback/v1/media/"+uri+"?manifestFileFormat="+mediaFormat, "application/json")
	if err != nil {
		return nil, err
	}
	var body struct {
		Media map[string]struct {
			Item struct {
				Metadata struct {
					Duration int64 `json:"duration"`
				} `json:"metadata"`
				Manifest map[string][]mediaFile `json:"manifest"`
			} `json:"item"`
		} `json:"media"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, fmt.Errorf("track-playback: %w", err)
	}
	entry, ok := body.Media[uri]
	if !ok { // relinked to another ID for this region
		for _, e := range body.Media {
			entry, ok = e, true
			break
		}
	}
	var files []mediaFile
	for _, f := range entry.Item.Manifest[mediaFormat] {
		if f.TrackType == "" || f.TrackType == "AUDIO" {
			files = append(files, f)
		}
	}
	if !ok || len(files) == 0 {
		return nil, fmt.Errorf("track-playback: no MP4 files for %s", uri)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Bitrate < files[j].Bitrate })
	f := files[0]
	if _, err := hex.DecodeString(f.FileID); err != nil || f.FileID == "" {
		return nil, fmt.Errorf("track-playback: bad file ID %q", f.FileID)
	}
	manifest, err := spGet("/sneaktables/v2/hls/1/"+f.FileID+"/audio.m3u8", "application/vnd.apple.mpegurl")
	if err != nil {
		return nil, err
	}
	return &Media{
		Manifest: adaptManifest(string(manifest), f.FileID),
		Duration: time.Duration(entry.Item.Metadata.Duration) * time.Millisecond,
		Bitrate:  int(f.Bitrate),
	}, nil
}
