package spotify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// LyricsError is a lyrics site, Site, answering Status rather than with
// lyrics.
type LyricsError struct {
	Site   string
	Status int
}

func (e *LyricsError) Error() string { return fmt.Sprintf("%s: HTTP %d", e.Site, e.Status) }

// Temporary reports whether the site may answer if asked again in a
// while: it is busy, or, NetEase, asked too often (405 in its answer, or
// 406).
func (e *LyricsError) Temporary() bool {
	return e.Status >= 500 || e.Status == http.StatusTooManyRequests || e.Site == "netease" && (e.Status == 405 || e.Status == 406)
}

// LRCLyrics asks LRCLIB for the lyrics of exactly t: its name, first
// artist, album and length.
func LRCLyrics(track Track) (synced, plain string, err error) {
	if !lrclibAsks(track) {
		return "", "", nil
	}
	params := url.Values{
		"track_name":  {track.Name},
		"artist_name": {track.Artists[0].Name},
		"album_name":  {track.Album.Name},
		"duration":    {strconv.FormatFloat(track.Length().Seconds(), 'f', 3, 64)},
	}
	var result struct {
		TrackName    string  `json:"trackName"`
		ArtistName   string  `json:"artistName"`
		Duration     float64 `json:"duration"`
		Instrumental bool    `json:"instrumental"`
		Synced       string  `json:"syncedLyrics"`
		Plain        string  `json:"plainLyrics"`
	}
	if ok, err := lrclib("get", params, &result); !ok || err != nil {
		return "", "", err
	}
	if result.Instrumental || !strings.EqualFold(strings.TrimSpace(result.TrackName), strings.TrimSpace(track.Name)) || !strings.EqualFold(strings.TrimSpace(result.ArtistName), strings.TrimSpace(track.Artists[0].Name)) || math.Abs(result.Duration-track.Length().Seconds()) > 2 {
		return "", "", nil
	}
	return result.Synced, result.Plain, nil
}

// LRCSearch looks among all LRCLIB has of t's name and first artist for
// timed lyrics: of the same title, by an artist credit naming that artist,
// nearest t's length within 2 seconds. The exact lookup can find a record
// with plain text only, of the many LRCLIB keeps of a song.
func LRCSearch(track Track) (synced, plain string, err error) {
	if !lrclibAsks(track) {
		return "", "", nil
	}
	var results []struct {
		TrackName    string  `json:"trackName"`
		ArtistName   string  `json:"artistName"`
		Duration     float64 `json:"duration"`
		Instrumental bool    `json:"instrumental"`
		Synced       string  `json:"syncedLyrics"`
	}
	artist := strings.ToLower(strings.TrimSpace(track.Artists[0].Name))
	if ok, err := lrclib("search", url.Values{"track_name": {track.Name}, "artist_name": {track.Artists[0].Name}}, &results); !ok || err != nil {
		return "", "", err
	}
	nearest := 2.0
	for _, r := range results {
		d := math.Abs(r.Duration - track.Length().Seconds())
		if r.Instrumental || r.Synced == "" || d > nearest || !strings.EqualFold(strings.TrimSpace(r.TrackName), strings.TrimSpace(track.Name)) || !strings.Contains(strings.ToLower(r.ArtistName), artist) {
			continue
		}
		synced, nearest = r.Synced, d
	}
	return synced, "", nil
}

// lrclibAsks reports whether LRCLIB can be asked of t: without an artist
// it turns the lookup down (400).
func lrclibAsks(t Track) bool {
	return !t.Episode && t.Name != "" && len(t.Artists) > 0 && t.Artists[0].Name != "" && t.Length() > 0
}

// lrclib gets LRCLIB's api/path into out, and reports whether it had it.
func lrclib(path string, params url.Values, out any) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://lrclib.net/api/"+path+"?"+params.Encode(), nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("User-Agent", "Vinyl/0.1 (lyrics fallback)")
	res, err := Client.Do(req)
	if err != nil {
		return false, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if res.StatusCode != http.StatusOK {
		return false, &LyricsError{"lrclib", res.StatusCode}
	}
	return true, json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(out)
}
