package player

import (
	"log"
	"net/http"
	"sync"
	"time"

	"vinyl/internal/spotify"
)

// The manifest names one CDN for a file; Spotify serves the same bytes on
// six. When that one does not answer — a CDN having a bad day, or a network
// that cannot reach it — the part is asked of another, so that a song plays
// through rather than stopping on the CDN's behalf.
//
// A CDN that failed is left out for a while, so that the rest of the track
// does not wait on it again; the one the manifest names is always tried
// first, as it is the one signed into the manifest.

// mirrorDown is how long a CDN that failed is passed over.
const mirrorDown = 5 * time.Minute

type mirrorState struct {
	mu   sync.Mutex
	dead map[string]time.Time // CDN host → until when it is passed over
}

var mirrors mirrorState

func (m *mirrorState) down(host string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	until, ok := m.dead[host]
	return ok && time.Now().Before(until)
}

func (m *mirrorState) failed(host string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.dead == nil {
		m.dead = map[string]time.Time{}
	}
	if _, ok := m.dead[host]; !ok {
		log.Printf("player: %s is not answering (%v); its parts come from another CDN for %v", host, err, mirrorDown)
	}
	m.dead[host] = time.Now().Add(mirrorDown)
}

// fetchPart gets a byte range of the file at source, from source itself, or
// from one of the CDNs that serve the same file if that one does not
// answer. It returns the response of whichever did: the caller caches under
// source either way, as that is the URL the next play names.
func fetchPart(r *http.Request, source, rg string) (*http.Response, error) {
	host := hostOf(source)
	// A CDN that failed is left out for a while, the one the manifest names
	// included: asking it again each time would cost every part the same
	// wait, and the track a stutter every few seconds.
	if !mirrors.down(host) {
		res, err := getRange(r, source, rg)
		if err == nil && answers(res) {
			return res, nil
		}
		if err == nil {
			res.Body.Close()
		}
		// A CDN that turns a request down properly is passed over for a
		// while too: it answered, and the answer was no.
		mirrors.failed(host, err)
	}

	for _, u := range otherMirrors(source) {
		if mirrors.down(hostOf(u)) {
			continue
		}
		res, err := getRange(r, u, rg)
		if err != nil || !answers(res) {
			if err == nil {
				res.Body.Close()
			}
			mirrors.failed(hostOf(u), err)
			continue
		}
		log.Printf("player: %s plays from %s", host, hostOf(u))
		return res, nil
	}
	// None answered: report what the one the manifest names said, whether a
	// failure or a refusal, rather than nothing.
	return getRange(r, source, rg)
}

// otherMirrors returns the URLs of the file source serves, but source
// itself: none if they cannot be had.
func otherMirrors(source string) []string {
	id := spotify.FileIDOf(source)
	if id == "" {
		return nil
	}
	all, err := mirrorsOf(id)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(all))
	for _, u := range all {
		if u != source {
			out = append(out, u)
		}
	}
	return out
}

func getRange(r *http.Request, u, rg string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	if rg != "" {
		req.Header.Set("Range", rg)
	}
	return cdnDo(req)
}

// cdnDo sends a request to a CDN. Tests swap it, rather than the client
// Spotify's own requests go through, which other tests are using.
var cdnDo = func(req *http.Request) (*http.Response, error) { return spotify.Client.Do(req) }

// mirrorsOf asks Spotify where else a file is served. Tests swap it for the
// same reason.
var mirrorsOf = spotify.Mirrors

// answers reports whether res carries the part: a range of it, or the whole
// file for one asked from its start.
func answers(res *http.Response) bool {
	switch res.StatusCode {
	case http.StatusPartialContent, http.StatusOK:
		return true
	}
	return false
}

// hostOf returns the host of u, for the log and for telling CDNs apart.
func hostOf(u string) string {
	if i := len("https://"); len(u) > i && u[:i] == "https://" {
		u = u[i:]
	}
	for i := 0; i < len(u); i++ {
		if u[i] == '/' {
			return u[:i]
		}
	}
	return u
}
