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

//go:embed web/player.html web/hls.min.js
var webFiles embed.FS

// Web plays tracks in a web view that never shows, whose key system
// (keySystem: Widevine, or FairPlay on macOS) decrypts what Spotify
// streams, as the web player's does: the one way a
// free account plays whole songs. Its page (web/player.html) takes
// commands through Eval, and reports what plays back to the server on
// 127.0.0.1 it is served from, which passes the media and licences through.
type Web struct {
	addr     string
	win      *mygo.Window
	view     *mygo.WebView // the page of NewVideoWeb, in its host's view
	cmds     chan string
	vol      chan float64 // the latest volume only
	ready    chan struct{}
	once     sync.Once
	done     chan struct{}
	stopOnce sync.Once
	server   *http.Server
	ctx      context.Context
	cancel   context.CancelFunc

	mu   sync.Mutex
	gen  int
	outs map[int]*webOut
	// heads are the starts of files fetched ahead of playing (Prefetch).
	heads []*head

	// Cache, if set, keeps the parts of tracks the page asks for.
	Cache *Cache
}

// NewWeb starts the page that plays, in a window never shown. It runs on
// the main thread, once the app is ready.
func NewWeb() (*Web, error) {
	w, err := listen()
	if err != nil {
		return nil, err
	}
	w.win = mygo.NewWindow(mygo.WindowOptions{
		Title: "Vinyl", Width: 320, Height: 240, Hidden: true, SkipTaskbar: true,
		URL: "http://" + w.addr + "/?ks=" + keySystem,
	})
	keepAwake(w.win)
	go w.run()
	return w, nil
}

// NewVideoWeb starts a page that plays music videos (OpenVideo), the
// picture filling it, in a web view of host's, which host's view shows
// where it places View with ui.WebView. It runs on the main thread.
func NewVideoWeb(host *mygo.Window) (*Web, error) {
	w, err := listen()
	if err != nil {
		return nil, err
	}
	w.view, err = host.NewWebView(mygo.WebViewOptions{
		URL:             "http://" + w.addr + "/?video=1&ks=" + keySystem,
		BackgroundColor: "#000000",
	})
	if err != nil {
		w.Close()
		return nil, err
	}
	keepAwake(host)
	go w.run()
	return w, nil
}

// View is the web view of NewVideoWeb's page, nil for NewWeb's.
func (w *Web) View() *mygo.WebView { return w.view }

// listen serves a page on 127.0.0.1, for a web view to load.
func listen() (*Web, error) {
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
	w.done = make(chan struct{})
	w.ctx, w.cancel = context.WithCancel(context.Background())
	w.server = &http.Server{Handler: w, BaseContext: func(net.Listener) context.Context { return w.ctx }}
	go w.server.Serve(ln)
	return w, nil
}

func (w *Web) Close() {
	w.stopOnce.Do(func() {
		if w.done != nil {
			close(w.done)
		}
		if w.cancel != nil {
			w.cancel()
		}
		if w.server != nil {
			w.server.Close()
		}
		if w.win != nil {
			w.win.Close()
		}
		if w.view != nil {
			w.view.Destroy()
		}
		w.mu.Lock()
		clear(w.outs)
		w.mu.Unlock()
	})
}

func (w *Web) run() {
	select {
	case <-w.ready:
	case <-w.done:
		return
	}
	for {
		select {
		case <-w.done:
			return
		case js := <-w.cmds:
			w.eval(js)
		case v := <-w.vol:
			w.eval(fmt.Sprintf("vinyl.volume(%.4f)", v))
		}
	}
}

func (w *Web) eval(js string) {
	ctx, cancel := context.WithTimeout(w.ctx, 5*time.Second)
	defer cancel()
	page := w.win.Page
	if w.view != nil {
		page = w.view.Page
	}
	if _, err := page().EvalContext(ctx, js); err != nil {
		log.Printf("player: %s: %v", js, err)
	}
}

func (w *Web) send(js string) {
	select {
	case <-w.done:
		return
	default:
	}
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
	if m.File != "" {
		// A file plays as it is: the page asks for its bytes a range at a
		// time, passed through from where it is.
		o.sources, o.file = []string{m.File}, true
	} else {
		o.manifest, o.sources = localManifest(m.Manifest, "/m/"+strconv.Itoa(o.gen)+"/")
	}
	w.outs[o.gen] = o
	w.mu.Unlock()
	if o.file {
		w.send(fmt.Sprintf("vinyl.load(%d,'file')", o.gen))
	} else {
		w.send(fmt.Sprintf("vinyl.load(%d)", o.gen))
	}
	return o
}

// OpenVideo loads v to play, paused at its start, and returns what plays
// it: its master playlist is the track's, its other playlists beside it.
func (w *Web) OpenVideo(v *spotify.Video) Output {
	o := &webOut{w: w, dur: v.Duration, at: time.Now(), license: v.License, video: true}
	w.mu.Lock()
	w.gen++
	o.gen = w.gen
	l := localizer{prefix: "/m/" + strconv.Itoa(o.gen) + "/", index: map[string]int{}}
	o.manifest, o.playlists = v.Master, map[string]string{}
	for name, pl := range v.Playlists {
		o.playlists[name] = l.local(pl)
	}
	o.sources = l.sources
	w.outs[o.gen] = o
	w.mu.Unlock()
	w.send(fmt.Sprintf("vinyl.load(%d)", o.gen))
	return o
}

// localManifest points the manifest's init section and segments at the
// server, under prefix, and returns where they really are.
func localManifest(manifest, prefix string) (string, []string) {
	l := localizer{prefix: prefix, index: map[string]int{}}
	return l.local(manifest), l.sources
}

// localizer points manifests at the server, under prefix, numbering what
// they name in sources.
type localizer struct {
	prefix  string
	sources []string
	index   map[string]int
}

func (l *localizer) local(manifest string) string {
	local := func(u string) string {
		i, ok := l.index[u]
		if !ok {
			i = len(l.sources)
			l.index[u] = i
			l.sources = append(l.sources, u)
		}
		return l.prefix + strconv.Itoa(i)
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
	return strings.Join(out, "\n") + "\n"
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
		gen, _ := strconv.Atoi(r.URL.Query().Get("gen"))
		o := w.out(gen)
		if o == nil {
			// Closed, its page about to unload it: Spotify isn't asked for
			// a licence nothing plays, nor a video's at a song's address.
			http.Error(rw, "gone", http.StatusGone)
			return
		}
		target := licenseHost + "/audio/license" + licenseQuery
		if o.license != "" {
			target = licenseOrigin + o.license
		}
		w.forward(rw, r, http.MethodPost, target)
	case p == "/cert":
		w.forward(rw, r, http.MethodGet, licenseHost+"/application-certificate")

	case p == "/event" && r.Method == http.MethodPost:
		var e struct {
			Gen  int     `json:"gen"`
			Type string  `json:"type"`
			T    float64 `json:"t"`
			Msg  string  `json:"msg"`
			Seq  int     `json:"seq"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&e) != nil {
			http.Error(rw, "bad event", http.StatusBadRequest)
			return
		}
		if e.Type == "ready" {
			w.once.Do(func() { close(w.ready) })
		} else if o := w.out(e.Gen); o != nil {
			o.event(e.Type, time.Duration(e.T*float64(time.Second)), e.Msg, e.Seq)
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
	if pl, ok := o.playlists[part]; ok || part == "track.m3u8" {
		if !ok {
			pl = o.manifest
		}
		rw.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		io.WriteString(rw, pl)
		return
	}
	i, err := strconv.Atoi(part)
	if err != nil || i < 0 || i >= len(o.sources) {
		http.NotFound(rw, r)
		return
	}
	rg := r.Header.Get("Range")
	// A video's segments, megabytes each, are not kept: the cache is for
	// songs. Nor is a file's, an episode's tens of megabytes.
	cache := w.Cache
	if o.video || o.file {
		cache = nil
	}
	if !o.video && !o.file && w.serveHead(rw, o.sources[i], rg) || cache.serve(rw, o.sources[i], rg) {
		return
	}
	// The part comes from the CDN the manifest names, or, when that one does
	// not answer, from another that serves the same file. A file has only
	// where it is: no other to ask, nor a CDN to pass over for a while, and
	// asked once, its failure is the page's to try again.
	fetch, source := fetchPart, o.sources[i]
	if o.file {
		fetch, source = getRange, o.went(source)
	}
	res, err := fetch(r, source, rg)
	if o.file {
		res, err = o.follow(r, i, source, rg, res, err)
	}
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
	k := cache.keep(o.sources[i], rg, res)
	_, err = io.Copy(rw, io.TeeReader(res.Body, k))
	k.done(err == nil)
}

// licenseOrigin is where a music video's licence path (spotify.Video's
// License) is; tests swap it for their own.
var licenseOrigin = "https://spclient.wg.spotify.com"

// forward sends the key system's request to Spotify's licence
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
	// A music video's: the playlists its manifest names, where its licence
	// comes from on spclient, and that it is one.
	playlists map[string]string
	license   string
	video     bool
	file      bool // a file that plays as it is (spotify.Media's File)
	// wentTo is where the file's address led, through its redirects: a
	// podcast's goes through a few, seconds each, to a counter of plays
	// and back, and the page asks for a range many times as it starts.
	wentTo string

	mu      sync.Mutex
	pos     time.Duration // where it was at at
	at      time.Time
	running bool // the page plays it: its time moves
	playing bool // it was asked to play, and has not ended
	asked   int  // how many times Vinyl asked the page to play or pause
	onEnd   func()
	// onSystem hears what the system did to the page rather than Vinyl.
	onSystem func(string)
	endSeq   int
	err      string
	licence  int           // how the licence service last answered
	wait     time.Duration // how long it asked to wait, with a 429
}

// went returns where to ask for the file at source: where it led last.
func (o *webOut) went(source string) string {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.wentTo != "" {
		return o.wentTo
	}
	return source
}

// follow takes the answer for the file asked for at source, sources[i]
// itself or where it led: where it led, for the next range; or, as where it
// led does not answer, as a signed address that ran out, sources[i] asked
// again, through its redirects.
func (o *webOut) follow(r *http.Request, i int, source, rg string, res *http.Response, err error) (*http.Response, error) {
	if (err != nil || !answers(res)) && source != o.sources[i] {
		if err == nil {
			res.Body.Close()
		}
		o.mu.Lock()
		o.wentTo = ""
		o.mu.Unlock()
		source = o.sources[i]
		res, err = getRange(r, source, rg)
	}
	if err == nil && answers(res) && res.Request != nil && res.Request.URL.String() != source {
		o.mu.Lock()
		o.wentTo = res.Request.URL.String()
		o.mu.Unlock()
	}
	return res, err
}

func (o *webOut) Play() {
	o.mu.Lock()
	o.playing = true
	o.asked++
	asked := o.asked
	o.mu.Unlock()
	o.w.send(fmt.Sprintf("vinyl.play(%d,%d)", o.gen, asked))
}

func (o *webOut) Pause() {
	o.mu.Lock()
	o.pos, o.at = o.position(time.Now()), time.Now()
	o.playing, o.running = false, false
	o.asked++
	asked := o.asked
	o.mu.Unlock()
	o.w.send(fmt.Sprintf("vinyl.pause(%d,%d)", o.gen, asked))
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

func (o *webOut) Duration() time.Duration {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.dur
}

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

// OnSystem has fn hear what the system did to the page rather than Vinyl:
// "pause" or "play", which WebKit carries out on the page itself for the
// media keys, Control Center and apps such as Mami, which pauses what plays
// while one talks; or "next" or "previous", the skips it passes the page.
func (o *webOut) OnSystem(fn func(string)) (off func()) {
	o.mu.Lock()
	o.onSystem = fn
	o.mu.Unlock()
	return func() {
		o.mu.Lock()
		o.onSystem = nil
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
func (o *webOut) event(kind string, t time.Duration, msg string, seq int) {
	o.mu.Lock()
	var end func()
	var system string
	switch kind {
	case "time", "seeked":
	case "playing":
		o.running = true
	case "pause", "waiting":
		o.running = false
	case "system-pause", "system-play":
		// Unless Vinyl has asked the page to play or pause since: that
		// stands.
		if seq != o.asked {
			o.mu.Unlock()
			return
		}
		system = strings.TrimPrefix(kind, "system-")
		o.playing = system == "play"
		o.running = o.running && o.playing
	case "system-next", "system-previous":
		fn := o.onSystem
		o.mu.Unlock()
		if fn != nil {
			fn(strings.TrimPrefix(kind, "system-"))
		}
		return
	case "ended":
		if seq <= o.endSeq {
			o.mu.Unlock()
			return
		}
		o.endSeq = seq
		log.Printf("player: track %d ended seq=%d at=%v duration=%v (%s)", o.gen, seq, t, o.dur, msg)
		o.running, o.playing = false, false
		t, end = o.dur, o.onEnd
	case "duration":
		// A file says how long it is once it loads: an episode's, where its
		// podcast keeps it, may have more in it than Spotify's, as ads.
		if o.file && t > 0 {
			o.dur = t
		}
		o.mu.Unlock()
		return
	case "retry":
		// The page tries again, as after a network error.
		log.Printf("player: track %d: retrying after %s", o.gen, msg)
		o.mu.Unlock()
		return
	case "error":
		if o.err != "" {
			// Reported once already: as the licence and the media element
			// both fail.
			o.mu.Unlock()
			return
		}
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
	fn := o.onSystem
	o.mu.Unlock()
	if end != nil {
		end()
	}
	if system != "" && fn != nil {
		fn(system)
	}
}
