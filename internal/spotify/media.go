package spotify

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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
	// Video is the manifest of the track's music video, for ResolveVideo,
	// "" for a track without one; or of a video podcast's episode.
	Video string
	// File is a file that plays as it is, with no manifest and no licence:
	// a podcast episode's audio where its podcast keeps it. With it, there
	// is no Manifest.
	File string
}

// videoFormat is what the web player asks for besides, to know whether a
// track has a music video and where its manifest is.
const videoFormat = "manifest_ids_video"

// externalFormat is what it asks for of an episode besides: where its
// podcast keeps its audio, as the podcast's feed names it, for an episode
// Spotify keeps none of.
const externalFormat = "file_urls_external"

type mediaEntry struct {
	Item struct {
		TrackType string `json:"track_type"` // AUDIO, VIDEO or MIXED
		Metadata  struct {
			Duration int64  `json:"duration"`
			Name     string `json:"name"`
			Show     string `json:"group_name"` // an episode's podcast
			Authors  []struct {
				Name string `json:"name"`
			} `json:"authors"`
		} `json:"metadata"`
		Manifest map[string][]mediaFile `json:"manifest"`
	} `json:"item"`
	// VideoVersion is the music video of an audio track: another entry of
	// the answer.
	VideoVersion string `json:"video_version_uri"`
}

type mediaFile struct {
	FileID    string   `json:"file_id"`
	Format    looseInt `json:"format"`
	Bitrate   looseInt `json:"bitrate"`
	TrackType string   `json:"track_type"` // AUDIO, or a video podcast's VIDEO
	FileURL   string   `json:"file_url"`   // externalFormat's
}

// looseInt accepts 11 and "11": the service sends either; and null, as
// for the files of a music video, which have no bitrate.
type looseInt int

func (n *looseInt) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*n = 0
		return nil
	}
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

// highQuality has ResolveMedia take the highest bitrate offered, 256k, whose
// licence Premium accounts get; a free account is refused it (403). denied
// holds the URIs it was refused for, which play at the lowest bitrate again.
var (
	highQuality atomic.Bool
	denied      sync.Map
)

// SetHighQuality chooses the highest bitrate where the account is licensed
// to play it, rather than always the lowest.
func SetHighQuality(on bool) { highQuality.Store(on) }

// Downgrade is called as the licence of a track playing at bitrate was
// refused. It reports whether the track can be asked for again at the lowest
// bitrate, which it then is; not if it played at that already.
func Downgrade(uri string, bitrate int) bool {
	if !highQuality.Load() || bitrate <= lowestBitrate {
		return false
	}
	_, had := denied.LoadOrStore(uri, true)
	return !had
}

// lowestBitrate is the bitrate a free account plays at.
const lowestBitrate = 128000

// ResolveMedia returns the manifest of uri, a track's or a podcast
// episode's, at the lowest bitrate: 128k, which a free account is licensed
// to play; the licence of the 256k one it is refused (403). With
// SetHighQuality it is at the highest, unless refused for uri before.
func ResolveMedia(uri string) (*Media, error) {
	formats := append([]string{mediaFormat}, mediaFallbacks...)
	asked := append(slices.Clone(formats), videoFormat)
	episode := strings.HasPrefix(uri, "spotify:episode:")
	if episode {
		asked = append(asked, externalFormat)
	}
	query := ""
	for _, f := range asked {
		query += "&manifestFileFormat=" + f
	}
	raw, err := spGet("/track-playback/v1/media/"+uri+"?"+strings.TrimPrefix(query, "&"), "application/json")
	if err != nil {
		return nil, err
	}
	var body struct {
		Media map[string]mediaEntry `json:"media"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, fmt.Errorf("track-playback: %w", err)
	}
	entry, ok := body.Media[uri]
	if !ok { // relinked to another ID for this region; not its video
		for _, e := range body.Media {
			if e.Item.TrackType != "VIDEO" {
				entry, ok = e, true
				break
			}
		}
	}
	// The first format that names any of the track's audio: the one this
	// platform plays, or a fallback of it.
	var files []mediaFile
	for _, format := range formats {
		for _, f := range entry.Item.Manifest[format] {
			if (f.TrackType == "" || f.TrackType == "AUDIO") && f.FileID != "" {
				files = append(files, f)
			}
		}
		if len(files) > 0 {
			break
		}
	}
	if ok && len(files) == 0 && episode {
		return episodeMedia(uri, entry, body.Media)
	}
	if !ok || len(files) == 0 {
		return nil, fmt.Errorf("track-playback: no MP4 files for %s", uri)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Bitrate < files[j].Bitrate })
	f := files[0]
	if _, no := denied.Load(uri); highQuality.Load() && !no {
		f = files[len(files)-1]
	}
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
		Video:    videoOf(entry, body.Media),
	}, nil
}

// ErrNoAudioHere is what ResolveMedia says of an episode with no audio
// this platform plays: as on macOS, of one Spotify keeps only in MP4s for
// Widevine (cenc), which FairPlay does not decrypt.
var ErrNoAudioHere = errors.New("spotify: nothing of the episode plays here")

// episodeMedia is how an episode plays that Spotify has no file of for
// this platform's key system, as the web player in Safari finds most:
// a video podcast's sound from its video, which is not encrypted, in step
// with its picture; or else its audio where its podcast keeps it, as
// Spotify names it or as the podcast's own feed does.
func episodeMedia(uri string, entry mediaEntry, media map[string]mediaEntry) (*Media, error) {
	duration := time.Duration(entry.Item.Metadata.Duration) * time.Millisecond
	if id := videoOf(entry, media); id != "" {
		v, err := ResolveVideo(id)
		switch {
		case err != nil:
			log.Printf("spotify: %s: video: %v", uri, err)
		case v.License == "" && v.Playlists[v.Sound] != "":
			return &Media{Manifest: v.Playlists[v.Sound], Duration: duration, Video: id}, nil
		}
	}
	for _, f := range entry.Item.Manifest[externalFormat] {
		if webURL(f.FileURL) {
			return &Media{File: f.FileURL, Duration: duration}, nil
		}
	}
	meta := entry.Item.Metadata
	publisher := ""
	if len(meta.Authors) > 0 {
		publisher = meta.Authors[0].Name
	}
	if file, err := feedAudio(meta.Show, publisher, meta.Name); err != nil {
		log.Printf("spotify: %s: feed: %v", uri, err)
	} else if file != "" {
		return &Media{File: file, Duration: duration}, nil
	}
	return nil, ErrNoAudioHere
}

// videoOf returns the manifest of e's music video, as the web player finds
// it: in e itself, with both sound and picture, or in its video version.
func videoOf(e mediaEntry, media map[string]mediaEntry) string {
	if e.Item.TrackType != "MIXED" {
		var ok bool
		if e, ok = media[e.VideoVersion]; !ok {
			return ""
		}
	}
	for _, f := range e.Item.Manifest[videoFormat] {
		if f.FileID != "" {
			return f.FileID
		}
	}
	return ""
}
