package main

import (
	"bufio"
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed web
var webFiles embed.FS

const (
	licenseHost = "https://spclient.wg.spotify.com/widevine-license/v1"
	browserUA   = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36 Edg/141.0.0.0"
)

// webIdentity is what the web player obtained for itself (see web/capture.js). Licences
// are only issued to this identity, not to the desktop one used for metadata.
type webIdentity struct {
	Token *struct {
		AccessToken string `json:"accessToken"`
		Expires     int64  `json:"accessTokenExpirationTimestampMs"`
		Anonymous   bool   `json:"isAnonymous"`
	} `json:"token"`
	ClientToken string `json:"clientToken"`
	Bearer      string `json:"bearer"`
}

func (w webIdentity) accessToken() string {
	if w.Token != nil && !w.Token.Anonymous {
		return w.Token.AccessToken
	}
	return ""
}

// playerServer serves the in-WebView player on 127.0.0.1: the page, the manifest with
// its segments pointed back here, the encrypted file (byte ranges, fetched from the
// CDN on demand) and the licence/certificate requests, forwarded with the web identity.
type playerServer struct {
	client   *http.Client
	media    *Media
	manifest string
	sources  []string // segment/init URLs in the original manifest
	addr     string
	fallback string // desktop client token

	mu  sync.Mutex
	web webIdentity
}

var uriAttr = regexp.MustCompile(`URI="([^"]*)"`)

func newPlayerServer(s *Session, m *Media) (*playerServer, error) {
	ps := &playerServer{client: &http.Client{Timeout: 30 * time.Second}, media: m, fallback: s.clientTok}
	index := map[string]int{}
	local := func(u string) string {
		i, ok := index[u]
		if !ok {
			i = len(ps.sources)
			index[u] = i
			ps.sources = append(ps.sources, u)
		}
		return "/media/" + strconv.Itoa(i)
	}
	var out []string
	sc := bufio.NewScanner(strings.NewReader(m.Manifest))
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		switch {
		case strings.HasPrefix(line, "#EXT-X-MAP:"):
			line = uriAttr.ReplaceAllStringFunc(line, func(attr string) string {
				return `URI="` + local(uriAttr.FindStringSubmatch(attr)[1]) + `"`
			})
		case line != "" && !strings.HasPrefix(line, "#"):
			line = local(line)
		}
		out = append(out, line)
	}
	ps.manifest = strings.Join(out, "\n") + "\n"

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	ps.addr = ln.Addr().String()
	go http.Serve(ln, ps)
	return ps, nil
}

func (ps *playerServer) URL() string { return "http://" + ps.addr + "/" }

func (ps *playerServer) setWeb(w webIdentity) {
	ps.mu.Lock()
	ps.web = w
	ps.mu.Unlock()
}

func (ps *playerServer) identity() (token, clientToken string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	token, clientToken = ps.web.accessToken(), ps.web.ClientToken
	if token == "" {
		token = ps.web.Bearer
	}
	if clientToken == "" {
		clientToken = ps.fallback
	}
	return
}

func (ps *playerServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Host != ps.addr { // only our own page, not another site resolving to 127.0.0.1
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	switch p := r.URL.Path; {
	case p == "/":
		b, _ := webFiles.ReadFile("web/player.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(b)
	case p == "/hls.js":
		b, _ := webFiles.ReadFile("web/hls.min.js")
		w.Header().Set("Content-Type", "text/javascript")
		w.Write(b)
	case p == "/info":
		json.NewEncoder(w).Encode(map[string]any{"name": ps.media.Name, "duration": ps.media.Duration})
	case p == "/track.m3u8":
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		io.WriteString(w, ps.manifest)
	case strings.HasPrefix(p, "/media/"):
		ps.serveMedia(w, r, strings.TrimPrefix(p, "/media/"))
	case p == "/license" && r.Method == http.MethodPost:
		ps.forward(w, r, http.MethodPost, licenseHost+"/audio/license", true)
	case p == "/cert":
		ps.forward(w, r, http.MethodGet, licenseHost+"/application-certificate", false)
	case p == "/log" && r.Method == http.MethodPost:
		var e struct{ Type, Msg string }
		json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&e)
		fmt.Printf("  page %-9s %s\n", e.Type, e.Msg)
	default:
		http.NotFound(w, r)
	}
}

// serveMedia passes a byte-range request through to the CDN.
func (ps *playerServer) serveMedia(w http.ResponseWriter, r *http.Request, idx string) {
	i, err := strconv.Atoi(idx)
	if err != nil || i < 0 || i >= len(ps.sources) {
		http.NotFound(w, r)
		return
	}
	req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, ps.sources[i], nil)
	if rg := r.Header.Get("Range"); rg != "" {
		req.Header.Set("Range", rg)
	}
	res, err := ps.client.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer res.Body.Close()
	for _, h := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges"} {
		if v := res.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	w.WriteHeader(res.StatusCode)
	io.Copy(w, res.Body)
}

// forward sends the CDM's request to the licence service with the web identity.
func (ps *playerServer) forward(w http.ResponseWriter, r *http.Request, method, target string, auth bool) {
	var body io.Reader
	var n int
	if method == http.MethodPost {
		b, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		body, n = bytes.NewReader(b), len(b)
	}
	token, clientToken := ps.identity()
	req, _ := http.NewRequestWithContext(r.Context(), method, target, body)
	req.Header.Set("client-token", clientToken)
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("Referer", "https://open.spotify.com/")
	if auth {
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	res, err := ps.client.Do(req)
	if err != nil {
		fmt.Printf("  %s: %v\n", r.URL.Path, err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer res.Body.Close()
	out, _ := io.ReadAll(res.Body)
	fmt.Printf("  %s: sent %dB → HTTP %d, %dB\n", r.URL.Path, n, res.StatusCode, len(out))
	if res.StatusCode != http.StatusOK && len(out) < 300 {
		fmt.Printf("    %s\n", bytes.TrimSpace(out))
	}
	w.WriteHeader(res.StatusCode)
	w.Write(out)
}

// pageEval is the bit of mygo.Page that waitWebIdentity needs.
type pageEval interface {
	EvalContext(ctx context.Context, code string) (any, error)
	LoadURL(url string) error
}

const loginURL = "https://accounts.spotify.com/login?continue=https%3A%2F%2Fopen.spotify.com%2F"

// waitWebIdentity polls the page (open.spotify.com with web/capture.js) until the web
// player has a signed-in token, sending the user to the login page if it is anonymous.
func waitWebIdentity(ctx context.Context, page pageEval, onLogin func()) (webIdentity, error) {
	asked := false
	var since time.Time
	for {
		select {
		case <-ctx.Done():
			return webIdentity{}, ctx.Err()
		case <-time.After(700 * time.Millisecond):
		}
		v, err := page.EvalContext(ctx, "return JSON.stringify(window.__vinyl || null)")
		s, _ := v.(string)
		if err != nil || s == "" || s == "null" {
			continue
		}
		var w webIdentity
		if json.Unmarshal([]byte(s), &w) != nil {
			continue
		}
		if w.Token != nil && w.Token.Anonymous && !asked {
			asked = true
			onLogin()
			page.LoadURL(loginURL)
			continue
		}
		if w.accessToken() == "" {
			continue
		}
		if since.IsZero() {
			since = time.Now()
		}
		// the client token usually shows up right after; don't wait forever for it
		if w.ClientToken != "" || time.Since(since) > 8*time.Second {
			return w, nil
		}
	}
}
