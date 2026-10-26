package player

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// A part of a track asked for again comes from the cache, though the CDN's
// signature has changed since; another part of it comes from the CDN.
func TestCacheServesPartsAgain(t *testing.T) {
	file := bytes.Repeat([]byte("0123456789"), 100)
	var asked int
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked++
		http.ServeContent(w, r, "a.mp4", time.Time{}, bytes.NewReader(file))
	}))
	defer cdn.Close()
	w := &Web{addr: "127.0.0.1:1", cmds: make(chan string, 16), outs: map[int]*webOut{}, Cache: NewCache(t.TempDir(), 1<<20)}
	o := &webOut{w: w, gen: 1, sources: []string{cdn.URL + "/audio/abc?__token__=monday"}}
	w.outs[1] = o
	get := func(rg string) (int, string, string) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://"+w.addr+"/m/1/0", nil)
		req.Header.Set("Range", rg)
		w.ServeHTTP(rec, req)
		return rec.Code, rec.Header().Get("Content-Range"), rec.Body.String()
	}

	code, cr, body := get("bytes=100-199")
	if code != http.StatusPartialContent || body != string(file[100:200]) || asked != 1 {
		t.Fatalf("first: %d %q %q, CDN asked %d times", code, cr, body, asked)
	}
	o.sources[0] = cdn.URL + "/audio/abc?__token__=tuesday"
	if code2, cr2, body2 := get("bytes=100-199"); code2 != code || cr2 != cr || body2 != body || asked != 1 {
		t.Fatalf("again: %d %q %q, CDN asked %d times", code2, cr2, body2, asked)
	}
	if _, _, body := get("bytes=200-299"); body != string(file[200:300]) || asked != 2 {
		t.Fatalf("another part: %q, CDN asked %d times", body, asked)
	}
}

// keepPart keeps a part of n bytes as source's range rg, used at at.
func keepPart(t *testing.T, c *Cache, source, rg string, n int, at time.Time) {
	t.Helper()
	res := &http.Response{StatusCode: http.StatusPartialContent, ContentLength: int64(n), Header: http.Header{}}
	k := c.keep(source, rg, res)
	if k == nil {
		t.Fatal("not kept")
	}
	io.Copy(k, strings.NewReader(strings.Repeat("x", n)))
	k.done(true)
	os.Chtimes(c.file(source, rg), at, at)
}

func kept(c *Cache, rg string) bool {
	_, err := os.Stat(c.file("/audio/abc", rg))
	return err == nil
}

// Past the limit, the parts used longest ago go: one served again stays.
func TestCacheDropsLeastUsed(t *testing.T) {
	c := NewCache(t.TempDir(), 20000)
	start := time.Now().Add(-time.Hour)
	for i := range 18 {
		keepPart(t, c, "/audio/abc", fmt.Sprint(i), 1000, start.Add(time.Duration(i)*time.Second))
	}
	if !kept(c, "0") || !kept(c, "1") {
		t.Fatal("dropped parts under the limit")
	}
	// Served again, the first is the last used.
	if !c.serve(httptest.NewRecorder(), "/audio/abc?__token__=x", "0") {
		t.Fatal("not served")
	}
	for i := 18; i < 20; i++ {
		keepPart(t, c, "/audio/abc", fmt.Sprint(i), 1000, time.Now())
	}
	if !kept(c, "0") || kept(c, "1") || !kept(c, "19") {
		t.Errorf("kept 0 %v, 1 %v, 19 %v; want the first, served again, kept and the second dropped", kept(c, "0"), kept(c, "1"), kept(c, "19"))
	}
	if n := c.Size(); n > 20000 {
		t.Errorf("takes %d, over the limit", n)
	}
	c.SetLimit(0)
	if n := c.Size(); n != 0 {
		t.Errorf("takes %d with no room", n)
	}
}

// A part that did not all come is not kept.
func TestCacheKeepsOnlyWholeParts(t *testing.T) {
	c := NewCache(t.TempDir(), 1<<20)
	res := &http.Response{StatusCode: http.StatusPartialContent, ContentLength: 100, Header: http.Header{}}
	k := c.keep("/audio/abc", "0", res)
	k.Write(make([]byte, 60))
	k.done(true)
	if kept(c, "0") || c.Size() != 0 {
		t.Error("kept a part cut short")
	}
}
