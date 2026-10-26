package spotify

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// The persisted queries' hashes change as Spotify updates its web player:
// a query whose hash the service no longer knows fails, and the web
// player's scripts, read again, say the new one.

// errStaleQuery is what a query fails with whose hash the service does
// not know.
var errStaleQuery = errors.New("spotify: query hash not known")

var (
	opsMu sync.RWMutex
	// OpsFile, set by the app, keeps the hashes read since, for the next
	// start.
	OpsFile string
	// scriptsURL is the page whose scripts hold the hashes; tests swap it.
	scriptsURL = WebPlayerURL

	refreshMu sync.Mutex
	refreshed time.Time // when the scripts were last read
)

func opHash(op string) string {
	opsMu.RLock()
	defer opsMu.RUnlock()
	return ops[op]
}

// LoadOps reads the hashes OpsFile keeps, of the queries Vinyl makes.
func LoadOps() {
	b, err := os.ReadFile(OpsFile)
	if err != nil {
		return
	}
	var kept map[string]string
	if json.Unmarshal(b, &kept) != nil {
		return
	}
	opsMu.Lock()
	defer opsMu.Unlock()
	for op, h := range kept {
		if _, ok := ops[op]; ok && hash64.MatchString(h) {
			ops[op] = h
		}
	}
}

var hash64 = regexp.MustCompile(`^[0-9a-f]{64}$`)

// refreshOps reads the web player's scripts again, as op's hash used was
// not known, and reports whether op has another now. Asked again within
// ten minutes, it doesn't read them again.
func refreshOps(op, used string) bool {
	refreshMu.Lock()
	defer refreshMu.Unlock()
	if opHash(op) != used {
		return true // read meanwhile
	}
	if time.Since(refreshed) < 10*time.Minute {
		return false
	}
	refreshed = time.Now()
	opsMu.RLock()
	need := make(map[string]bool, len(ops))
	for name := range ops {
		need[name] = true
	}
	opsMu.RUnlock()
	found, err := scriptHashes(need)
	if err != nil {
		log.Printf("spotify: reading the web player's queries: %v", err)
		return false
	}
	opsMu.Lock()
	for name, h := range found {
		if need[name] {
			ops[name] = h
		}
	}
	kept, _ := json.Marshal(ops)
	opsMu.Unlock()
	log.Printf("spotify: %d query hashes read again, %s now %s", len(found), op, opHash(op))
	if OpsFile != "" {
		os.WriteFile(OpsFile, kept, 0o600)
	}
	return opHash(op) != used
}

var (
	scriptSrc = regexp.MustCompile(`<script[^>]+src="([^"]+\.js)"`)
	opDecl    = regexp.MustCompile(`"(\w+)","(?:query|mutation)","([0-9a-f]{64})"`)
	chunkMap  = regexp.MustCompile(`\.u=\w+=>""\+\(\(\{([^}]*)\}\)\[\w+\]\|\|\w+\)\+"\."\+\(\{([^}]*)\}\)\[\w+\]\+"\.js"`)
	chunkPair = regexp.MustCompile(`(\d+):"([^"]+)"`)
)

// scriptHashes returns the hashes of the queries the web player's scripts
// declare: those of its main script, and of the scripts it loads as
// needed, until it has all of need.
func scriptHashes(need map[string]bool) (map[string]string, error) {
	page, err := fetchText(scriptsURL)
	if err != nil {
		return nil, err
	}
	var main string
	for _, m := range scriptSrc.FindAllStringSubmatch(page, -1) {
		if strings.Contains(m[1], "/web-player.") {
			main = m[1]
		}
	}
	if main == "" {
		return nil, errors.New("no web-player script")
	}
	base, err := url.Parse(scriptsURL)
	if err != nil {
		return nil, err
	}
	mainURL, err := base.Parse(main)
	if err != nil {
		return nil, err
	}
	js, err := fetchText(mainURL.String())
	if err != nil {
		return nil, err
	}
	found := map[string]string{}
	scan := func(js string) {
		for _, m := range opDecl.FindAllStringSubmatch(js, -1) {
			found[m[1]] = m[2]
		}
	}
	scan(js)
	missing := func() bool {
		for name := range need {
			if found[name] == "" {
				return true
			}
		}
		return false
	}
	m := chunkMap.FindStringSubmatch(js)
	if m == nil || !missing() {
		return found, nil
	}
	names := map[string]string{}
	for _, p := range chunkPair.FindAllStringSubmatch(m[1], -1) {
		names[p[1]] = p[2]
	}
	var chunks []string
	for _, p := range chunkPair.FindAllStringSubmatch(m[2], -1) {
		name := names[p[1]]
		if name == "" {
			name = p[1]
		}
		if u, err := mainURL.Parse(name + "." + p[2] + ".js"); err == nil {
			chunks = append(chunks, u.String())
		}
	}
	// A few at a time, until none is missing.
	var mu sync.Mutex
	work := make(chan string)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for u := range work {
				if js, err := fetchText(u); err == nil {
					mu.Lock()
					scan(js)
					mu.Unlock()
				}
			}
		})
	}
	for _, u := range chunks {
		mu.Lock()
		more := missing()
		mu.Unlock()
		if !more {
			break
		}
		work <- u
	}
	close(work)
	wg.Wait()
	return found, nil
}

// fetchText gets the page or script at url, as a browser would.
func fetchText(url string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", UserAgent)
	res, err := Client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s: %s", url, res.Status)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	return string(b), err
}
