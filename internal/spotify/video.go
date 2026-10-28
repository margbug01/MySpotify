package spotify

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Video is how a music video plays in the web player: HLS that the web
// player writes itself from the video's manifest, a master playlist naming
// a playlist of its sound and one of each size of its picture, which the
// browser's key system decrypts (Widevine, or FairPlay on macOS: see
// videoKeySystem).
type Video struct {
	Master    string            // HLS, its playlists named as in Playlists
	Playlists map[string]string // HLS of each, its segments signed CDN URLs
	Duration  time.Duration
	License   string // the path on spclient the key system's licence requests go to
}

// videoManifest is the part of a video's manifest the web player plays
// from: one encoding, in profiles of a sound or a picture, each in
// segments a few seconds long under the templates.
type videoManifest struct {
	Contents []struct {
		EncodingID    string           `json:"encoding_id"`
		SegmentLength int              `json:"segment_length"` // seconds
		Start         int64            `json:"start_time_millis"`
		End           int64            `json:"end_time_millis"`
		Profiles      []videoProfile   `json:"profiles"`
		Encryption    []encryptionInfo `json:"encryption_infos"`
	} `json:"contents"`
	InitTemplate    string   `json:"initialization_template"`
	SegmentTemplate string   `json:"segment_template"`
	BaseURLs        []string `json:"base_urls"`
}

type videoProfile struct {
	ID           int    `json:"id"`
	FileType     string `json:"file_type"`
	AudioCodec   string `json:"audio_codec"`
	VideoCodec   string `json:"video_codec"`
	AudioBitrate int    `json:"audio_bitrate"`
	VideoBitrate int    `json:"video_bitrate"`
	MaxBitrate   int    `json:"max_bitrate"`
	Width        int    `json:"video_width"`
	Height       int    `json:"video_height"`
	Encryption   []int  `json:"encryption_indices"`
	KeyID        string `json:"key_id"` // base64
}

type encryptionInfo struct {
	KeySystem string `json:"key_system"`
	Endpoint  string `json:"license_server_endpoint"`
	Data      string `json:"encryption_data"` // a PSSH box, base64
}

// ResolveVideo returns the HLS of the music video whose manifest is id, as
// Media.Video names it.
func ResolveVideo(id string) (*Video, error) {
	// The manifests name the video's source as a GID, and the track's answer
	// may write it as a URI's ID instead: take whichever came, in hex.
	gid, err := GID(id)
	if err != nil {
		return nil, fmt.Errorf("video: bad manifest ID %q", id)
	}
	raw, err := spGet("/manifests/v9/json/sources/"+gid+"/options/supports_drm", "application/json")
	if err != nil {
		return nil, err
	}
	var m videoManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("video: %w", err)
	}
	return videoHLS(m)
}

// videoHLS writes the HLS the web player plays m from: the sound and every
// size of the picture in the files the key system decrypts
// (videoFileType), as many seconds a segment as m says.
func videoHLS(m videoManifest) (*Video, error) {
	if len(m.Contents) == 0 || len(m.BaseURLs) == 0 {
		return nil, errors.New("video: empty manifest")
	}
	c := m.Contents[0]
	key := slices.IndexFunc(c.Encryption, func(e encryptionInfo) bool { return e.KeySystem == videoKeySystem })
	if key < 0 || c.SegmentLength <= 0 || c.End <= c.Start {
		return nil, fmt.Errorf("video: nothing %s plays", videoKeySystem)
	}
	enc := c.Encryption[key]
	var sound *videoProfile
	var sizes []videoProfile
	for _, p := range c.Profiles {
		if p.FileType != videoFileType || !slices.Contains(p.Encryption, key) {
			continue
		}
		switch {
		case p.VideoCodec != "":
			sizes = append(sizes, p)
		case p.AudioCodec != "" && (sound == nil || p.AudioBitrate > sound.AudioBitrate):
			sound = &p
		}
	}
	if sound == nil || len(sizes) == 0 {
		return nil, fmt.Errorf("video: no %s profiles", videoFileType)
	}
	slices.SortFunc(sizes, func(a, b videoProfile) int { return a.VideoBitrate - b.VideoBitrate })
	dur := time.Duration(c.End-c.Start) * time.Millisecond
	fill := func(t string, p videoProfile) string {
		t = strings.ReplaceAll(t, "{{profile_id}}", strconv.Itoa(p.ID))
		return m.BaseURLs[0] + strings.ReplaceAll(t, "{{file_type}}", p.FileType)
	}
	playlist := func(p videoProfile) string {
		var b strings.Builder
		fmt.Fprintf(&b, "#EXTM3U\n#EXT-X-VERSION:6\n#EXT-X-TARGETDURATION:%d\n#EXT-X-PLAYLIST-TYPE:VOD\n#EXT-X-MEDIA-SEQUENCE:0\n", c.SegmentLength)
		b.WriteString(videoKey(enc, p, c.EncodingID) + "\n")
		if p.FileType != "ts" {
			fmt.Fprintf(&b, "#EXT-X-MAP:URI=\"%s\"\n", fill(m.InitTemplate, p))
		}
		step := time.Duration(c.SegmentLength) * time.Second
		for at := time.Duration(0); at < dur; at += step {
			fmt.Fprintf(&b, "#EXTINF:%.3f,\n%s\n", min(step, dur-at).Seconds(), strings.ReplaceAll(fill(m.SegmentTemplate, p), "{{segment_timestamp}}", strconv.Itoa(int(at/time.Second))))
		}
		b.WriteString("#EXT-X-ENDLIST\n")
		return b.String()
	}
	v := &Video{Playlists: map[string]string{}, Duration: dur, License: videoLicense(enc)}
	name := func(p videoProfile) string { return "p" + strconv.Itoa(p.ID) + ".m3u8" }
	var b strings.Builder
	b.WriteString("#EXTM3U\n#EXT-X-VERSION:6\n#EXT-X-INDEPENDENT-SEGMENTS\n")
	fmt.Fprintf(&b, "#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID=\"audio\",NAME=\"audio\",DEFAULT=YES,AUTOSELECT=YES,URI=\"%s\"\n", name(*sound))
	v.Playlists[name(*sound)] = playlist(*sound)
	for _, p := range sizes {
		fmt.Fprintf(&b, "#EXT-X-STREAM-INF:BANDWIDTH=%d,AVERAGE-BANDWIDTH=%d,RESOLUTION=%dx%d,CODECS=\"%s,%s\",AUDIO=\"audio\"\n%s\n",
			sound.MaxBitrate+p.MaxBitrate, sound.AudioBitrate+p.VideoBitrate, p.Width, p.Height, sound.AudioCodec, p.VideoCodec, name(p))
		v.Playlists[name(p)] = playlist(p)
	}
	v.Master = b.String()
	return v, nil
}
