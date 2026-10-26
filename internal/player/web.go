package player

import (
	"bufio"
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"

	"vinyl/internal/spotify"
)

//go:embed web
var webFiles embed.FS

// licenseHost is Spotify's licence service; tests swap it for their own.
var licenseHost = "https://spclient.wg.spotify.com/widevine-license/v1"

// Web plays tracks in a web view that never shows, whose Widevine module
// decrypts what Spotify streams, as the web player's does: the one way a
// free account plays whole songs. Its page (web/player.html) takes
// commands through Eval, and reports what plays back to the server on
// 127.0.0.1 it is served from, which passes the media and licences through.
type Web struct {
	addr  string
	win   *mygo.Window
	cmds  chan string
	vol   chan float64 // the latest volume only
	ready chan struct{}
	once  sync.Once

	mu   sync.Mutex
	gen  int
	outs map[int]*webOut
}

// NewWeb starts the page that plays, in a window never shown. It runs on
// the main thread, once the app is ready.
func NewWeb() (*Web, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	w := &Web{
		addr:  ln.Addr().String(),
		cmds:  make(chan string, 256),
		vol:   make(chan float64, 1),
		ready: make(chan struct{}),
		outs:  map[int]*webOut{},
	}
	go http.Serve(ln, w)
	w.win = mygo.NewWindow(mygo.WindowOptions{
		Title: "Vinyl", Width: 320, Height: 240, Hidden: true, SkipTaskbar: true,
		URL: "http://" + w.addr + "/",
	})
	go w.run()
	return w, nil
}

// run evaluates the commands in order, once the page is ready.
func (w *Web) run() {
	<-w.ready
	for {
		select {
		case js := <-w.cmds:
			w.eval(js)
		case v := <-w.vol:
			w.eval(fmt.Sprintf("vinyl.volume(%.4f)", v))
		}
	}
}

func (w *Web) eval(js string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := w.win.Page().EvalContext(ctx, js); err != nil {
		log.Printf("player: %s: %v", js, err)
	}
}

func (w *Web) send(js string) {
	select {
	case w.cmds <- js:
	default:
		log.Printf("player: dropped %s", js)
	}
}

func (w *Web) setVolume(v float64) {
	for {
		select {
		case w.vol <- v:
			return
		default:
			select {
			case <-w.vol: // replaced by v
			default:
			}
		}
	}
}

var uriAttr = regexp.MustCompile(`URI="([^"]*)"`)

// Open loads m to play, paused at its start, and returns what plays it.
func (w *Web) Open(m *spotify.Media) Output {
	o := &webOut{w: w, dur: m.Duration, at: time.Now()}
	w.mu.Lock()
	w.gen++
	o.gen = w.gen
	o.manifest, o.sources = localManifest(m.Manifest, "/m/"+strconv.Itoa(o.gen)+"/")
	w.outs[o.gen] = o
	w.mu.Unlock()
	w.send(fmt.Sprintf("vinyl.load(%d)", o.gen))
	return o
}

// localManifest points the manifest's init section and segments at the
// server, under prefix, and returns where they really are.
func localManifest(manifest, prefix string) (string, []string) {
	var sources []string
	index := map[string]int{}
	local := func(u string) string {
		i, ok := index[u]
		if !ok {
			i = len(sources)
			index[u] = i
			sources = append(sources, u)
		}
		return prefix + strconv.Itoa(i)
	}
	var out []string
	sc := bufio.NewScanner(strings.NewReader(manifest))
	sc.Buffer(nil, 1<<20)
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
	return strings.Join(out, "\n") + "\n", sources
}

func (w *Web) out(gen int) *webOut {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.outs[gen]
}

func (w *Web) close(gen int) {
	w.mu.Lock()
	delete(w.outs, gen)
	w.mu.Unlock()
	w.send(fmt.Sprintf("vinyl.unload(%d)", gen))
}

func (w *Web) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	// Only the page, not another site whose name points at 127.0.0.1.
	if r.Host != w.addr {
		http.Error(rw, "forbidden", http.StatusForbidden)
		return
	}
	switch p := r.URL.Path; {
	case p == "/":
		b, _ := webFiles.ReadFile("web/player.html")
		rw.Header().Set("Content-Type", "text/html; charset=utf-8")
		rw.Write(b)
	case p == "/hls.js":
		b, _ := webFiles.ReadFile("web/hls.min.js")
		rw.Header().Set("Content-Type", "text/javascript")
		rw.Write(b)
	case strings.HasPrefix(p, "/m/"):
		w.serveMedia(rw, r, strings.TrimPrefix(p, "/m/"))
	case p == "/license" && r.Method == http.MethodPost:
		w.forward(rw, r, http.MethodPost, licenseHost+"/audio/license")
	case p == "/cert":
		w.forward(rw, r, http.MethodGet, licenseHost+"/application-certificate")

	case p == "/event" && r.Method == http.MethodPost:
		var e struct {
			Gen  int     `json:"gen"`
			Type string  `json:"type"`
			T    float64 `json:"t"`
			Msg  string  `json:"msg"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&e) != nil {
			http.Error(rw, "bad event", http.StatusBadRequest)
			return
		}
		if e.Type == "ready" {
			w.once.Do(func() { close(w.ready) })
		} else if o := w.out(e.Gen); o != nil {
			o.event(e.Type, time.Duration(e.T*float64(time.Second)), e.Msg)
		}
		rw.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(rw, r)
	}
}

// serveMedia serves gen's manifest, or passes a byte range of one of its
// parts through from the CDN.
func (w *Web) serveMedia(rw http.ResponseWriter, r *http.Request, rest string) {
	g, part, _ := strings.Cut(rest, "/")
	gen, _ := strconv.Atoi(g)
	o := w.out(gen)
	if o == nil {
		http.NotFound(rw, r)
		return
	}
	if part == "track.m3u8" {
		rw.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		io.WriteString(rw, o.manifest)
		return
	}
	i, err := strconv.Atoi(part)
	if err != nil || i < 0 || i >= len(o.sources) {
		http.NotFound(rw, r)
		return
	}
	req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, o.sources[i], nil)
	if rg := r.Header.Get("Range"); rg != "" {
		req.Header.Set("Range", rg)
	}
	res, err := spotify.Client.Do(req)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadGateway)
		return
	}
	defer res.Body.Close()
	for _, h := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges"} {
		if v := res.Header.Get(h); v != "" {
			rw.Header().Set(h, v)
		}
	}
	rw.WriteHeader(res.StatusCode)
	io.Copy(rw, res.Body)
}

// forward sends the Widevine module's request to Spotify's licence
// service, as the web player: once more, renewed, if the service turned
// the identity down. The track it is for, gen in the query, notes the
// answer.
func (w *Web) forward(rw http.ResponseWriter, r *http.Request, method, target string) {
	var body []byte
	if method == http.MethodPost {
		b, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(rw, err.Error(), http.StatusBadRequest)
			return
		}
		body = b
	}
	gen, _ := strconv.Atoi(r.URL.Query().Get("gen"))
	o := w.out(gen)
	var res *http.Response
	var out []byte
	for attempt := 0; ; attempt++ {
		id, err := spotify.CurrentIdentity()
		if attempt > 0 {
			id, err = spotify.RenewedIdentity()
		}
		if err != nil {
			log.Printf("player: %s: %v", r.URL.Path, err)
			o.answered(http.StatusUnauthorized, 0)
			http.Error(rw, err.Error(), http.StatusUnauthorized)
			return
		}
		req, _ := http.NewRequestWithContext(r.Context(), method, target, bytes.NewReader(body))
		req.Header.Set("client-token", id.ClientToken)
		req.Header.Set("User-Agent", spotify.UserAgent)
		req.Header.Set("Referer", "https://open.spotify.com/")
		if method == http.MethodPost {
			req.Header.Set("Authorization", "Bearer "+id.Token)
			req.Header.Set("Content-Type", "application/octet-stream")
		}
		if res, err = spotify.Client.Do(req); err != nil {
			http.Error(rw, err.Error(), http.StatusBadGateway)
			return
		}
		out, _ = io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != http.StatusUnauthorized || attempt > 0 {
			break
		}
		log.Printf("player: %s: 401, renewing the identity", r.URL.Path)
	}
	wait := spotify.RetryAfter(res.Header)
	if res.StatusCode != http.StatusOK {
		log.Printf("player: %s: %s, retry after %v: %s", r.URL.Path, res.Status, wait, bytes.TrimSpace(out[:min(len(out), 200)]))
	}
	o.answered(res.StatusCode, wait)
	rw.WriteHeader(res.StatusCode)
	rw.Write(out)
}

// webOut is one track in the page, which reports where it is a few times
// a second: in between, its position moves on with the clock.
type webOut struct {
	w        *Web
	gen      int
	dur      time.Duration
	manifest string
	sources  []string

	mu      sync.Mutex
	pos     time.Duration // where it was at at
	at      time.Time
	running bool // the page plays it: its time moves
	playing bool // it was asked to play, and has not ended
	onEnd   func()
	err     string
	licence int           // how the licence service last answered
	wait    time.Duration // how long it asked to wait, with a 429
}

func (o *webOut) Play() {
	o.mu.Lock()
	o.playing = true
	o.mu.Unlock()
	o.w.send(fmt.Sprintf("vinyl.play(%d)", o.gen))
}

func (o *webOut) Pause() {
	o.mu.Lock()
	o.pos, o.at = o.position(time.Now()), time.Now()
	o.playing, o.running = false, false
	o.mu.Unlock()
	o.w.send(fmt.Sprintf("vinyl.pause(%d)", o.gen))
}

func (o *webOut) Playing() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.playing
}

func (o *webOut) Seek(t time.Duration) {
	o.mu.Lock()
	o.pos, o.at = t, time.Now()
	o.mu.Unlock()
	o.w.send(fmt.Sprintf("vinyl.seek(%d,%.3f)", o.gen, t.Seconds()))
}

func (o *webOut) Position() time.Duration {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.position(time.Now())
}

func (o *webOut) position(now time.Time) time.Duration {
	p := o.pos
	if o.running {
		p += now.Sub(o.at)
	}
	return min(max(p, 0), o.dur)
}

func (o *webOut) Duration() time.Duration { return o.dur }

func (o *webOut) OnEnd(fn func()) (off func()) {
	o.mu.Lock()
	o.onEnd = fn
	o.mu.Unlock()
	return func() {
		o.mu.Lock()
		o.onEnd = nil
		o.mu.Unlock()
	}
}

func (o *webOut) SetVolume(v float64) { o.w.setVolume(v) }
func (o *webOut) Close()              { o.w.close(o.gen) }

// Fault returns why the page could not play the track, "" if it could;
// with how the licence service last answered, and how long it asked to
// wait.
func (o *webOut) Fault() (msg string, licence int, wait time.Duration) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.err, o.licence, o.wait
}

// answered notes how the licence service answered the track's request. It
// does nothing for a track gone.
func (o *webOut) answered(status int, wait time.Duration) {
	if o == nil {
		return
	}
	o.mu.Lock()
	o.licence, o.wait = status, wait
	o.mu.Unlock()
}

// event takes in what the page reports: an event of its audio at t.
func (o *webOut) event(kind string, t time.Duration, msg string) {
	o.mu.Lock()
	var end func()
	switch kind {
	case "time", "seeked":
	case "playing":
		o.running = true
	case "pause", "waiting":
		o.running = false
	case "ended":
		o.running, o.playing = false, false
		t, end = o.dur, o.onEnd
	case "retry":
		// The page tries again, as after a network error.
		log.Printf("player: track %d: retrying after %s", o.gen, msg)
		o.mu.Unlock()
		return
	case "error":
		log.Printf("player: track %d: %s", o.gen, msg)
		o.running, o.playing, o.err = false, false, msg
		end = o.onEnd
	default:
		o.mu.Unlock()
		return
	}
	// A seek on its way lands where it was asked, not where the page was.
	if kind != "error" {
		o.pos, o.at = t, time.Now()
	}
	o.mu.Unlock()
	if end != nil {
		end()
	}
}
