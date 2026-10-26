package player

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"time"
)

// Cache keeps on disk the parts of tracks the page asked the CDN for, as
// it served them, encrypted: a track played again plays from them, without
// waiting on the network, though its licence is still asked for. A part is
// known by the path of its file on the CDN and the bytes asked for: the
// URL's signature, in its query, changes from day to day. Once they take
// more than the limit, those used longest ago go first.
type Cache struct {
	dir string

	mu    sync.Mutex
	limit int64 // in bytes; 0 keeps nothing
	used  int64 // what the parts take, -1 until measured
}

// NewCache returns a cache in dir of limit bytes.
func NewCache(dir string, limit int64) *Cache {
	return &Cache{dir: dir, limit: limit, used: -1}
}

// SetLimit changes how much the cache may take, and drops what is over.
func (c *Cache) SetLimit(limit int64) {
	c.mu.Lock()
	c.limit = limit
	c.mu.Unlock()
	c.prune()
}

// partHead heads a part kept: how the CDN answered.
type partHead struct {
	Status int    `json:"status"`
	Type   string `json:"type,omitempty"`
	Range  string `json:"range,omitempty"`
}

// file is where the part of source the range rg asks for is kept.
func (c *Cache) file(source, rg string) string {
	path := source
	if u, err := url.Parse(source); err == nil {
		path = u.Path
	}
	h := sha256.Sum256([]byte(path + "\n" + rg))
	return filepath.Join(c.dir, hex.EncodeToString(h[:16])+".part")
}

// serve answers with the part of source rg asks for, if kept, and reports
// whether it did.
func (c *Cache) serve(rw http.ResponseWriter, source, rg string) bool {
	if c == nil {
		return false
	}
	path := c.file(source, rg)
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	br := bufio.NewReader(f)
	line, err := br.ReadBytes('\n')
	var head partHead
	info, serr := f.Stat()
	if err != nil || serr != nil || json.Unmarshal(line, &head) != nil || head.Status == 0 {
		return false
	}
	// Used now: the last to go.
	now := time.Now()
	os.Chtimes(path, now, now)
	h := rw.Header()
	if head.Type != "" {
		h.Set("Content-Type", head.Type)
	}
	if head.Range != "" {
		h.Set("Content-Range", head.Range)
	}
	h.Set("Content-Length", strconv.FormatInt(info.Size()-int64(len(line)), 10))
	h.Set("Accept-Ranges", "bytes")
	rw.WriteHeader(head.Status)
	io.Copy(rw, br)
	return true
}

// keeping writes down a part as it passes through, to keep once all of
// it has come. A nil one keeps nothing.
type keeping struct {
	c      *Cache
	f      *os.File
	path   string
	want   int64 // the bytes of the body
	n      int64
	failed bool
}

// keep starts keeping the part of source rg asks for, which res brings,
// or returns nil if it is not to be kept.
func (c *Cache) keep(source, rg string, res *http.Response) *keeping {
	if c == nil || res.StatusCode != http.StatusOK && res.StatusCode != http.StatusPartialContent || res.ContentLength <= 0 {
		return nil
	}
	c.mu.Lock()
	limit := c.limit
	c.mu.Unlock()
	// A part of more than a sixteenth of it would push out too many.
	if res.ContentLength > limit/16 || os.MkdirAll(c.dir, 0o700) != nil {
		return nil
	}
	f, err := os.CreateTemp(c.dir, "*.tmp")
	if err != nil {
		return nil
	}
	head, _ := json.Marshal(partHead{res.StatusCode, res.Header.Get("Content-Type"), res.Header.Get("Content-Range")})
	if _, err := f.Write(append(head, '\n')); err != nil {
		f.Close()
		os.Remove(f.Name())
		return nil
	}
	return &keeping{c: c, f: f, path: c.file(source, rg), want: res.ContentLength}
}

// Write writes b down; failing to, it keeps nothing, but never fails what
// passes the part on.
func (k *keeping) Write(b []byte) (int, error) {
	if k != nil && !k.failed {
		if _, err := k.f.Write(b); err != nil {
			k.failed = true
		}
		k.n += int64(len(b))
	}
	return len(b), nil
}

// done keeps the part, if all of it came, ok.
func (k *keeping) done(ok bool) {
	if k == nil {
		return
	}
	tmp := k.f.Name()
	info, err := k.f.Stat()
	k.f.Close()
	if !ok || k.failed || k.n != k.want || err != nil || os.Rename(tmp, k.path) != nil {
		os.Remove(tmp)
		return
	}
	k.c.added(info.Size())
}

func (c *Cache) added(n int64) {
	c.mu.Lock()
	if c.used >= 0 {
		c.used += n
	}
	over := c.used < 0 || c.used > c.limit
	c.mu.Unlock()
	if over {
		c.prune()
	}
}

type cachedPart struct {
	path string
	size int64
	used time.Time
}

func (c *Cache) parts() []cachedPart {
	entries, _ := os.ReadDir(c.dir)
	var out []cachedPart
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		path := filepath.Join(c.dir, e.Name())
		switch filepath.Ext(e.Name()) {
		case ".part":
			out = append(out, cachedPart{path, info.Size(), info.ModTime()})
		case ".tmp":
			// Left by an app that quit as a part came in.
			if time.Since(info.ModTime()) > time.Hour {
				os.Remove(path)
			}
		}
	}
	return out
}

// prune drops the parts used longest ago until the rest fit the limit,
// with room to spare, rather than one at each part kept.
func (c *Cache) prune() {
	c.mu.Lock()
	defer c.mu.Unlock()
	parts := c.parts()
	var total int64
	for _, p := range parts {
		total += p.size
	}
	if total > c.limit {
		slices.SortFunc(parts, func(a, b cachedPart) int { return a.used.Compare(b.used) })
		for _, p := range parts {
			if total <= c.limit*9/10 {
				break
			}
			if os.Remove(p.path) == nil {
				total -= p.size
			}
		}
	}
	c.used = total
}

// Size returns how many bytes the parts kept take.
func (c *Cache) Size() int64 {
	var n int64
	for _, p := range c.parts() {
		n += p.size
	}
	return n
}

// Clear drops every part kept.
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, p := range c.parts() {
		os.Remove(p.path)
	}
	c.used = 0
}
