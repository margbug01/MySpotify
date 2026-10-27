package spotify

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// A file Spotify serves is on several CDNs at once, each with a signature of
// its own: the manifest names one of them, and storage-resolve names them
// all. They hold the same bytes, so when the one the manifest names does not
// answer — a CDN having a bad day, or a network that cannot reach it — the
// same byte range can be asked of another.
//
// Mirrors asks storage-resolve for those others.

// mirrorTTL is how long a signature lasts: a day, as storage-resolve says.
// They are kept a little less than that.
const mirrorTTL = 23 * time.Hour

type mirrorEntry struct {
	urls []string
	at   time.Time
}

var (
	mirrorMu sync.Mutex
	mirrors  = map[string]mirrorEntry{}
)

// Mirrors returns the URLs fileID is served at, all of them, the one a
// manifest would name among them. They answer any byte range of the file,
// signed apart from one another. An error means none: the caller keeps the
// one URL it has.
//
// A file that is a music video's has its own lane; audio is what is asked
// for, as a song's file always is.
func Mirrors(fileID string) ([]string, error) {
	key := strings.ToLower(fileID)
	mirrorMu.Lock()
	if e, ok := mirrors[key]; ok && time.Since(e.at) < mirrorTTL {
		mirrorMu.Unlock()
		return e.urls, nil
	}
	mirrorMu.Unlock()

	path := "/storage-resolve/v2/files/audio/interactive/10/" + key +
		"?version=10000000&product=9&platform=39&alt=json"
	raw, err := spGet(path, "application/json")
	if err != nil {
		return nil, fmt.Errorf("mirrors: %w", err)
	}
	var v struct {
		Result string   `json:"result"`
		URLs   []string `json:"cdnurl"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("mirrors: %w", err)
	}
	if len(v.URLs) == 0 {
		return nil, fmt.Errorf("mirrors: none for %s", key)
	}
	mirrorMu.Lock()
	mirrors[key] = mirrorEntry{urls: v.URLs, at: time.Now()}
	// A few a day is all the app ever asks for; a session's worth is plenty.
	if len(mirrors) > 64 {
		for k, e := range mirrors {
			if time.Since(e.at) >= mirrorTTL {
				delete(mirrors, k)
			}
		}
	}
	mirrorMu.Unlock()
	return v.URLs, nil
}

// FileIDOf returns the file a CDN URL serves: its last path part, which is
// the file's ID in hex, lowercased. It is empty for a URL that does not end
// in one.
func FileIDOf(u string) string {
	if i := strings.IndexByte(u, '?'); i >= 0 {
		u = u[:i]
	}
	u = strings.TrimSuffix(u, "/")
	if i := strings.LastIndexByte(u, '/'); i >= 0 {
		u = u[i+1:]
	}
	if len(u) != 40 {
		return ""
	}
	for _, c := range u {
		if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F') {
			return ""
		}
	}
	return strings.ToLower(u)
}
