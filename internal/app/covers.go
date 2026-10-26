package app

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"vinyl/internal/spotify"
)

// coversMax is how much the covers kept on disk may take: some thousands.
const coversMax = 100 << 20

// coversKept counts the covers fetchCover wrote, to prune them every while
// as the app runs on, in the notification area perhaps, for days.
var coversKept atomic.Int32

// coverFile is where the cover at url is kept in dir.
func coverFile(dir, url string) string {
	h := sha256.Sum256([]byte(url))
	return filepath.Join(dir, hex.EncodeToString(h[:16]))
}

// fetchCover returns the cover at url: from dir, if fetched before, else
// fetched, and kept there. Spotify's image CDNs never change what is under
// an address, unlike Gravatar, whose pictures are fetched each time.
func fetchCover(dir, url string) ([]byte, error) {
	host, _, _ := strings.Cut(strings.TrimPrefix(url, "https://"), "/")
	if dir == "" || !strings.HasSuffix(host, ".scdn.co") && !strings.HasSuffix(host, ".spotifycdn.com") {
		return spotify.Fetch(url)
	}
	path := coverFile(dir, url)
	if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
		// Used now: pruneCovers drops those used longest ago.
		now := time.Now()
		os.Chtimes(path, now, now)
		return b, nil
	}
	b, err := spotify.Fetch(url)
	if err != nil {
		return nil, err
	}
	if os.MkdirAll(dir, 0o755) == nil {
		if f, err := os.CreateTemp(dir, "*.part"); err == nil {
			_, err = f.Write(b)
			if f.Close() != nil || err != nil || os.Rename(f.Name(), path) != nil {
				os.Remove(f.Name())
			}
		}
	}
	if coversKept.Add(1)%500 == 0 {
		go pruneCovers(dir, coversMax)
	}
	return b, nil
}

// pruneCovers drops the covers in dir used longest ago, once they take more
// than limit, down to three quarters of it.
func pruneCovers(dir string, limit int64) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type kept struct {
		path string
		size int64
		used time.Time
	}
	var all []kept
	var total int64
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		all = append(all, kept{filepath.Join(dir, e.Name()), info.Size(), info.ModTime()})
		total += info.Size()
	}
	if total <= limit {
		return
	}
	slices.SortFunc(all, func(a, b kept) int { return a.used.Compare(b.used) })
	for _, k := range all {
		if total <= limit*3/4 {
			break
		}
		if os.Remove(k.path) == nil {
			total -= k.size
		}
	}
}
