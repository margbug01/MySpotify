package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"

	librespot "github.com/devgianlu/go-librespot"
	downloadpb "github.com/devgianlu/go-librespot/proto/spotify/download"
	"google.golang.org/protobuf/proto"
)

// mediaFile is one encrypted MP4 rendition listed by track-playback.
type mediaFile struct {
	FileID  string   `json:"file_id"`
	Format  looseInt `json:"format"`
	Bitrate looseInt `json:"bitrate"`
}

// looseInt accepts 11 and "11": the service sends either.
type looseInt int

func (n *looseInt) UnmarshalJSON(b []byte) error {
	var v int
	if err := json.Unmarshal(bytes.Trim(b, `"`), &v); err != nil {
		return fmt.Errorf("not a number: %s", b)
	}
	*n = looseInt(v)
	return nil
}

// Media is what the browser-side player needs for one track: the HLS manifest
// (key system data + byte ranges) and where to download the encrypted file.
type Media struct {
	Name     string
	Duration int64 // ms
	File     mediaFile
	Manifest string
	CDN      []string
}

func (s *Session) spGet(ctx context.Context, path string, query url.Values, accept string) ([]byte, error) {
	res, err := s.sp.Request(ctx, http.MethodGet, path, query, http.Header{"Accept": {accept}}, nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", path, res.StatusCode)
	}
	return body, nil
}

// ResolveMedia lists the track's CENC MP4 files, takes the lowest bitrate, and fetches
// its HLS manifest and CDN addresses.
func (s *Session) ResolveMedia(ctx context.Context, id librespot.SpotifyId) (*Media, error) {
	uri := id.Uri()
	raw, err := s.spGet(ctx, "/track-playback/v1/media/"+uri,
		url.Values{"manifestFileFormat": {"file_ids_mp4"}}, "application/json")
	if err != nil {
		return nil, err
	}
	var body struct {
		Media map[string]struct {
			Item struct {
				Metadata struct {
					Name     string `json:"name"`
					Duration int64  `json:"duration"`
				} `json:"metadata"`
				Manifest struct {
					MP4 []mediaFile `json:"file_ids_mp4"`
				} `json:"manifest"`
			} `json:"item"`
		} `json:"media"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, fmt.Errorf("track-playback: %w", err)
	}
	entry, ok := body.Media[uri]
	if !ok { // relinked under another id for this region
		for _, e := range body.Media {
			entry, ok = e, true
			break
		}
	}
	files := entry.Item.Manifest.MP4
	if !ok || len(files) == 0 {
		return nil, fmt.Errorf("track-playback: no MP4 files")
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Bitrate < files[j].Bitrate })
	m := &Media{Name: entry.Item.Metadata.Name, Duration: entry.Item.Metadata.Duration, File: files[0]}

	manifest, err := s.spGet(ctx, "/sneaktables/v2/hls/1/"+m.File.FileID+"/audio.m3u8", nil, "application/vnd.apple.mpegurl")
	if err != nil {
		return nil, err
	}
	m.Manifest = string(manifest)

	if _, err := hex.DecodeString(m.File.FileID); err != nil {
		return nil, fmt.Errorf("bad file id %q", m.File.FileID)
	}
	sr, err := s.spGet(ctx, fmt.Sprintf("/storage-resolve/v2/files/audio/interactive/%d/%s", m.File.Format, m.File.FileID), nil, "application/x-protobuf")
	if err != nil {
		return nil, err
	}
	var res downloadpb.StorageResolveResponse
	if err := proto.Unmarshal(sr, &res); err != nil {
		return nil, fmt.Errorf("storage resolve: %w", err)
	}
	if res.Result != downloadpb.StorageResolveResponse_CDN || len(res.Cdnurl) == 0 {
		return nil, fmt.Errorf("storage resolve: %s", res.Result)
	}
	m.CDN = res.Cdnurl
	return m, nil
}
