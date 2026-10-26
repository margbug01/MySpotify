// Package spotify reaches Spotify as its web player does: with the identity
// the web player gets for itself (see CaptureJS), its GraphQL queries
// (Pathfinder) and the spclient endpoints for lyrics and media.
package spotify

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// pathfinderURL is where the GraphQL queries go; tests swap it.
var pathfinderURL = "https://api-partner.spotify.com/pathfinder/v2/query"

const (
	spclientURL = "https://spclient.wg.spotify.com"
	// UserAgent is the browser the requests say they come from, as the web
	// player's own do.
	UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36 Edg/141.0.0.0"
)

// Client is the HTTP client of every request to Spotify and its CDNs.
var Client = &http.Client{
	Timeout:   30 * time.Second,
	Transport: &http.Transport{Proxy: systemProxy, ForceAttemptHTTP2: true, MaxIdleConnsPerHost: 8, IdleConnTimeout: 90 * time.Second},
}

// CaptureJS runs before open.spotify.com's own scripts and notes the
// identity the web player gets: see ParseIdentity.
//
//go:embed capture.js
var CaptureJS string

// WebPlayerURL is the page that signs the web player in, if it can.
const WebPlayerURL = "https://open.spotify.com/"

// LoginURL is where a listener signs in, coming back to the web player.
const LoginURL = "https://accounts.spotify.com/login?continue=https%3A%2F%2Fopen.spotify.com%2F"

// Identity is who the requests go as: the web player's access token and
// client token.
type Identity struct {
	Token       string
	ClientToken string
	Expires     time.Time
	Anonymous   bool
}

// ParseIdentity reads what CaptureJS noted (window.__vinyl, as JSON). It
// returns false until the web player has a token.
func ParseIdentity(s string) (Identity, bool) {
	var v struct {
		Token *struct {
			AccessToken string `json:"accessToken"`
			Expires     int64  `json:"accessTokenExpirationTimestampMs"`
			Anonymous   bool   `json:"isAnonymous"`
		} `json:"token"`
		ClientToken string `json:"clientToken"`
		Bearer      string `json:"bearer"`
	}
	if json.Unmarshal([]byte(s), &v) != nil {
		return Identity{}, false
	}
	id := Identity{ClientToken: v.ClientToken}
	switch {
	case v.Token != nil && v.Token.AccessToken != "":
		id.Token, id.Anonymous = v.Token.AccessToken, v.Token.Anonymous
		id.Expires = time.UnixMilli(v.Token.Expires)
	case v.Bearer != "":
		// Seen on a request only: its lifetime unknown, an hour at most.
		id.Token, id.Expires = v.Bearer, time.Now().Add(45*time.Minute)
	default:
		return Identity{}, false
	}
	return id, true
}

// ErrSignedOut is what requests fail with while nobody is signed in.
var ErrSignedOut = errors.New("spotify: signed out")

// ErrNotFound is what a request for something Spotify doesn't have fails with.
var ErrNotFound = errors.New("spotify: not found")

// Throttled is what a request fails with while Spotify turns requests away
// for coming too often. Wait is how long it asked to wait, 0 if it did not
// say.
type Throttled struct{ Wait time.Duration }

func (e *Throttled) Error() string {
	return fmt.Sprintf("spotify: too many requests, again in %v", e.Wait)
}

// RetryAfter reads how long a response asks to wait before asking again:
// its Retry-After, in seconds or as a date. It is 0 if it does not say.
func RetryAfter(h http.Header) time.Duration {
	v := strings.TrimSpace(h.Get("Retry-After"))
	if n, err := strconv.Atoi(v); err == nil {
		return max(time.Duration(n)*time.Second, 0)
	}
	if t, err := http.ParseTime(v); err == nil {
		return max(time.Until(t), 0)
	}
	return 0
}

var (
	idMu     sync.Mutex
	current  Identity
	renewing chan struct{} // closed once the renewal under way ends
	renewErr error
	// Renew, set by the app, gets a fresh identity from the web player,
	// blocking until it has one.
	Renew func() (Identity, error)
)

// SetIdentity makes id the one requests go as.
func SetIdentity(id Identity) {
	idMu.Lock()
	current = id
	idMu.Unlock()
}

// CurrentIdentity returns the identity requests go as, renewed first if it
// is about to run out.
func CurrentIdentity() (Identity, error) { return identity(false) }

// RenewedIdentity renews the identity now, as the service turned the
// current one down, and returns it.
func RenewedIdentity() (Identity, error) { return identity(true) }

// identity returns a usable identity: the current one, unless it runs out
// within two minutes or stale says the service turned it down.
func identity(stale bool) (Identity, error) {
	idMu.Lock()
	id := current
	fresh := id.Token != "" && !id.Anonymous && time.Until(id.Expires) > 2*time.Minute
	if fresh && !stale {
		idMu.Unlock()
		return id, nil
	}
	if Renew == nil {
		idMu.Unlock()
		if id.Token == "" || id.Anonymous {
			return id, ErrSignedOut
		}
		return id, nil
	}
	// One renewal at a time; whoever else asks waits for it.
	wait := renewing
	if wait == nil {
		wait = make(chan struct{})
		renewing = wait
		go func() {
			next, err := Renew()
			idMu.Lock()
			if err == nil {
				current = next
			}
			renewErr, renewing = err, nil
			idMu.Unlock()
			close(wait)
		}()
	}
	idMu.Unlock()
	<-wait
	idMu.Lock()
	defer idMu.Unlock()
	if renewErr != nil {
		return current, renewErr
	}
	if current.Anonymous || current.Token == "" {
		return current, ErrSignedOut
	}
	return current, nil
}

// do sends the request build makes as the web player, renewing the
// identity once if the service turns it down, and returns the body of a 200.
func do(build func() (*http.Request, error)) ([]byte, error) {
	for attempt := 0; ; attempt++ {
		id, err := identity(attempt > 0)
		if err != nil {
			return nil, err
		}
		req, err := build()
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+id.Token)
		if id.ClientToken != "" {
			req.Header.Set("client-token", id.ClientToken)
		}
		req.Header.Set("app-platform", "WebPlayer")
		req.Header.Set("User-Agent", UserAgent)
		req.Header.Set("Accept-Language", "zh-CN")
		res, err := Client.Do(req)
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
		res.Body.Close()
		if err != nil {
			return nil, err
		}
		switch {
		case res.StatusCode == http.StatusOK:
			return body, nil
		case res.StatusCode == http.StatusUnauthorized && attempt == 0:
			continue
		case res.StatusCode == http.StatusNotFound:
			return nil, ErrNotFound
		case res.StatusCode == http.StatusTooManyRequests:
			return nil, &Throttled{RetryAfter(res.Header)}
		case res.StatusCode == http.StatusPreconditionFailed:
			// "Invalid query hash": the web player was updated.
			return nil, fmt.Errorf("%w: %s", errStaleQuery, snippet(body))
		}
		return nil, fmt.Errorf("spotify: %s %s: %s", req.URL.Host, res.Status, snippet(body))
	}
}

func snippet(b []byte) string {
	s := strings.Join(strings.Fields(string(b)), " ")
	if len(s) > 160 {
		s = s[:160] + "…"
	}
	return s
}

// ops are the web player's persisted GraphQL queries, by name: the hash
// stands for the query, which Spotify's servers hold.
//
// They are those of web-player.f954508a.js (2026-10-26): when the service
// no longer knows one, refreshOps reads the web player's scripts again.
var ops = map[string]string{
	"profileAttributes":                  "08ffb4730af3746e04a8301396f20875dbbce10c75243803091a9274eacc8ac0",
	"home":                               "5fb7da7a03ee2776c0670856609cb85a189a3cecc36cc160b91cde79063b9cf1",
	"browsePage":                         "f5c4e6d668f5716464a231c1cc8b22c1cbf6ad68b09929fd7de813a30581298b",
	"fetchPlaylist":                      "21ea11a6ac75a5f3e01fd365d29c18e0a4a8b7dc09f89bbb5d73910190c6acaf",
	"getAlbum":                           "0b6ce3511e1b02e4a40fa95440f992acbe85080eb308af9194322d708fc1914f",
	"searchTracks":                       "b02683192a98dde7966b5e6655a79eeb62713eab703eda9902c932818dd52751",
	"libraryV3":                          "390c78e5b951029bad359785e69b07b536a509c581cbcd0aded5e5067f187455",
	"fetchLibraryTracks":                 "087278b20b743578a6262c2b0b4bcd20d879c503cc359a2285baf083ef944240",
	"addToLibrary":                       "896ebcb47815681340860d121cb5d494e157e2a78d3950385cd54e0393c67148",
	"removeFromLibrary":                  "896ebcb47815681340860d121cb5d494e157e2a78d3950385cd54e0393c67148",
	"queryArtistOverview":                "1ac33ddab5d39a3a9c27802774e6d78b9405cc188c6f75aed007df2a32737c72",
	"queryArtistDiscographyAlbums":       "5e07d323febb57b4a56a42abbf781490e58764aa45feb6e3dc0591564fc56599",
	"queryArtistDiscographySingles":      "5e07d323febb57b4a56a42abbf781490e58764aa45feb6e3dc0591564fc56599",
	"queryArtistDiscographyCompilations": "5e07d323febb57b4a56a42abbf781490e58764aa45feb6e3dc0591564fc56599",
	"searchArtists":                      "7bf95d754fdbe32c8b161fbbe54d1ae50974900df4dce4c8f1afcbcad153224d",
	"addToPlaylist":                      "b907ad6b088d5bf7680fa81198996d58398c2d55dfcdf59fcc959e0be98a418a",
	"removeFromPlaylist":                 "b907ad6b088d5bf7680fa81198996d58398c2d55dfcdf59fcc959e0be98a418a",
	"fetchEntitiesForRecentlyPlayed":     "cf5d2e94ffd82788470788ae1f6090cc3e9e774fb8fd383580634c6e6f50f7be",
	"searchAlbums":                       "202cb3305e31e5a0767ba7925f28bd728cf8f8b0217e6da43909056071cd70e9",
	"searchPlaylists":                    "d520014e748f9ea44f7707d8df1819867ac1205e8b7f3e28f22fe5fc858921b1",
	"getTrack":                           "fe16b8083eec2305854db612ca75589bcb71e009dc7a4d014f29bd534a4c01f5",
	"internalLinkRecommenderTrack":       "642923b2e705fceb9984697c1620fecd42615fd8b116b8a9faf2786b389d3e4a",
	"queryTrackCreditsGroupedModal":      "f135fb9be58a72d041ab5d214d817021a272405d883860468e2627afb01a3ca9",
	"browseAll":                          "dbd8b55e09a58afc52eab438bc228ba28fd72ac2f2148c6c26354980e4579001",
	"queryShowMetadataV2":                "b475447846f37cc426add800b995a7859c169c53c72b62de6338c0c40a5dacea",
	"queryPodcastEpisodes":               "3539d746cf882f3909660de40b4ef472b3f5893a0761d617c282541de5d412c5",
	"searchPodcasts":                     "0195d9f61b43606d490bca64c3456e3593528cea6cc05c7e822c7c42beed0f4e",
}

// query runs the web player's GraphQL operation op with vars, decoding its
// data into out: again, once, if the service no longer knew the query, and
// the web player's scripts say another hash.
func query(op string, vars map[string]any, out any) error {
	h := opHash(op)
	err := queryAs(op, h, vars, out)
	if errors.Is(err, errStaleQuery) && refreshOps(op, h) {
		err = queryAs(op, opHash(op), vars, out)
	}
	return err
}

// queryAs runs op, its hash h.
func queryAs(op, h string, vars map[string]any, out any) error {
	body, err := json.Marshal(map[string]any{
		"variables":     vars,
		"operationName": op,
		"extensions":    map[string]any{"persistedQuery": map[string]any{"version": 1, "sha256Hash": h}},
	})
	if err != nil {
		return err
	}
	raw, err := do(func() (*http.Request, error) {
		req, err := http.NewRequest(http.MethodPost, pathfinderURL, bytes.NewReader(body))
		if err == nil {
			req.Header.Set("Content-Type", "application/json;charset=UTF-8")
			req.Header.Set("Accept", "application/json")
		}
		return req, err
	})
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	var res struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if len(res.Errors) > 0 && (len(res.Data) == 0 || string(res.Data) == "null") {
		if strings.Contains(res.Errors[0].Message, "PersistedQueryNotFound") {
			return fmt.Errorf("%s: %w", op, errStaleQuery)
		}
		return fmt.Errorf("%s: %s", op, res.Errors[0].Message)
	}
	return json.Unmarshal(res.Data, out)
}

// spGet gets path from spclient, as the web player does.
func spGet(path, accept string) ([]byte, error) {
	return do(func() (*http.Request, error) {
		req, err := http.NewRequest(http.MethodGet, spclientURL+path, nil)
		if err == nil && accept != "" {
			req.Header.Set("Accept", accept)
		}
		return req, err
	})
}

// Fetch returns what is at url, as a cover the image CDN serves.
func Fetch(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	res, err := Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, errors.New(res.Status)
	}
	return io.ReadAll(res.Body)
}
