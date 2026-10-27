package player

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"vinyl/internal/spotify"
)

// headSegments is how many segments the start fetched ahead covers: the
// first plays at once, and the rest while the CDN sends what follows. A
// segment is ten seconds, so six is a minute of the track at hand before
// the CDN is needed at all.
const headSegments = 6

// maxHeads is how many starts are kept: the next track's, and a few the
// queue went past.
const maxHeads = 3

// head is the start of a track's file, fetched before it plays: the page's
// first requests for the file are answered from it, without the CDN.
type head struct {
	source string // the file's CDN URL, as the manifest names it
	data   []byte // its bytes from the first
	total  string // its length, for Content-Range, or "*"
	ctype  string
}

var byteRangeAttr = regexp.MustCompile(`BYTERANGE="(\d+)(?:@(\d+))?"`)

// headOf reads the start of a manifest's file: its URL, and how many bytes
// its init section and first segments take, where they run on from the
// first byte. It returns 0 bytes where they don't.
func headOf(manifest string) (string, int64) {
	var source string
	var end int64
	n, off, pending := int64(0), int64(0), false
	segments := 0
	sc := bufio.NewScanner(strings.NewReader(manifest))
	sc.Buffer(nil, 1<<20)
loop:
	for sc.Scan() && segments < headSegments {
		line := strings.TrimRight(sc.Text(), "\r")
		switch {
		case strings.HasPrefix(line, "#EXT-X-MAP:"):
			u, br := uriAttr.FindStringSubmatch(line), byteRangeAttr.FindStringSubmatch(line)
			if u == nil || br == nil || br[2] != "" && br[2] != "0" || source != "" {
				return "", 0
			}
			source = u[1]
			end, _ = strconv.ParseInt(br[1], 10, 64)
		case strings.HasPrefix(line, "#EXT-X-BYTERANGE:"):
			length, at, ok := strings.Cut(strings.TrimPrefix(line, "#EXT-X-BYTERANGE:"), "@")
			n, _ = strconv.ParseInt(length, 10, 64)
			off = end
			if ok {
				off, _ = strconv.ParseInt(at, 10, 64)
			}
			pending = n > 0
		case line != "" && !strings.HasPrefix(line, "#"):
			if !pending || off != end || source != "" && line != source {
				break loop
			}
			source, end, pending = line, off+n, false
			segments++
		}
	}
	if segments == 0 {
		return "", 0
	}
	return source, end
}

// Prefetch fetches the start of m's file, so that once m is opened the
// page's first requests for it are answered at once.
//
// The start comes in parts, each a range of the same file, and they are
// asked for at the same time: one request carrying a minute of AAC takes a
// while on a slow link, and the page wants its first part in a fraction of
// a second. The parts are joined once they are all in, the page's requests
// being answered from the single run of bytes.
func (w *Web) Prefetch(m *spotify.Media) {
	source, n := headOf(m.Manifest)
	if n <= 0 || w.head(source) != nil {
		return
	}
	parts := splitHead(n)
	if parts == nil {
		return
	}
	data := make([]byte, n)
	var meta struct { // what the parts say of the whole, the first to speak
		sync.Mutex
		total, ctype string
	}
	var wg sync.WaitGroup
	var ok atomic.Bool
	ok.Store(true)
	for _, p := range parts {
		wg.Add(1)
		go func(p headPart) {
			defer wg.Done()
			req, err := http.NewRequest(http.MethodGet, source, nil)
			if err != nil {
				ok.Store(false)
				return
			}
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", p.from, p.to))
			res, err := cdnDo(req)
			if err != nil {
				ok.Store(false)
				return
			}
			defer res.Body.Close()
			if res.StatusCode != http.StatusPartialContent && res.StatusCode != http.StatusOK {
				ok.Store(false)
				return
			}
			body, err := io.ReadAll(io.LimitReader(res.Body, p.to-p.from+1))
			if err != nil || int64(len(body)) != p.to-p.from+1 {
				ok.Store(false)
				return
			}
			copy(data[p.from:], body)
			meta.Lock()
			if meta.total == "" {
				if _, rest, found := strings.Cut(res.Header.Get("Content-Range"), "/"); found && rest != "" && !strings.ContainsAny(rest, "*") {
					meta.total = rest
				}
			}
			if meta.ctype == "" {
				meta.ctype = res.Header.Get("Content-Type")
			}
			meta.Unlock()
		}(p)
	}
	wg.Wait()
	// One part short is a hole in the middle of the start: the page reads
	// it as silence or as an error it has to recover from, so it is better
	// to hand it nothing and let the CDN send the start itself.
	if !ok.Load() {
		log.Printf("player: prefetch of %s fell short; its start comes from the CDN", source)
		return
	}
	total := meta.total
	if total == "" {
		total = strconv.FormatInt(n, 10)
	}
	w.mu.Lock()
	w.heads = append(w.heads, &head{source: source, data: data, total: total, ctype: meta.ctype})
	if len(w.heads) > maxHeads {
		w.heads = w.heads[len(w.heads)-maxHeads:]
	}
	w.mu.Unlock()
}

// headPart is a range of the start fetched ahead: both ends, as the CDN
// wants them.
type headPart struct{ from, to int64 }

// splitHead cuts the start of n bytes into the parts they are fetched in,
// at most headParallel of them, each a run of the file.
func splitHead(n int64) []headPart {
	if n <= 0 {
		return nil
	}
	parts := headParallel
	if n < int64(parts) {
		parts = int(n)
	}
	size := (n + int64(parts) - 1) / int64(parts)
	out := make([]headPart, 0, parts)
	for from := int64(0); from < n; from += size {
		out = append(out, headPart{from: from, to: min(from+size-1, n-1)})
	}
	return out
}

// headParallel is how many parts of a start come at once: enough to keep a
// slow link busy, few enough not to look like a download to the CDN.
const headParallel = 4

func (w *Web) head(source string) *head {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, h := range w.heads {
		if h.source == source {
			return h
		}
	}
	return nil
}

// serveHead answers a request for bytes within the start fetched ahead of
// source, and reports whether it did.
func (w *Web) serveHead(rw http.ResponseWriter, source, rg string) bool {
	h := w.head(source)
	if h == nil {
		return false
	}
	from, to, ok := parseRange(rg)
	if !ok || to >= int64(len(h.data)) {
		return false
	}
	if h.ctype != "" {
		rw.Header().Set("Content-Type", h.ctype)
	}
	rw.Header().Set("Accept-Ranges", "bytes")
	rw.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%s", from, to, h.total))
	rw.Header().Set("Content-Length", strconv.FormatInt(to-from+1, 10))
	rw.WriteHeader(http.StatusPartialContent)
	rw.Write(h.data[from : to+1])
	return true
}

// parseRange reads a Range of one span with both ends, as the page asks:
// bytes=a-b.
func parseRange(rg string) (int64, int64, bool) {
	spec, ok := strings.CutPrefix(rg, "bytes=")
	if !ok {
		return 0, 0, false
	}
	a, b, ok := strings.Cut(spec, "-")
	from, err1 := strconv.ParseInt(a, 10, 64)
	to, err2 := strconv.ParseInt(b, 10, 64)
	return from, to, ok && err1 == nil && err2 == nil && from <= to
}
