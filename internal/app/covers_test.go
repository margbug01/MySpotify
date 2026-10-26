package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A cover kept on disk loads from there, without the network: the address
// does not exist.
func TestFetchCoverReadsWhatIsKept(t *testing.T) {
	dir := t.TempDir()
	url := "https://i.scdn.co/image/vinyl-test-not-a-cover"
	if err := os.WriteFile(coverFile(dir, url), []byte("kept"), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := fetchCover(dir, url)
	if err != nil || string(b) != "kept" {
		t.Fatalf("fetchCover = %q, %v", b, err)
	}
}

// Past the limit, the covers used longest ago go, down to three quarters.
func TestPruneCoversDropsTheLeastRecentlyUsed(t *testing.T) {
	dir := t.TempDir()
	t0 := time.Now().Add(-time.Hour)
	for i, name := range []string{"oldest", "older", "newer", "newest"} {
		path := filepath.Join(dir, name)
		os.WriteFile(path, make([]byte, 30), 0o644)
		at := t0.Add(time.Duration(i) * time.Minute)
		os.Chtimes(path, at, at)
	}
	pruneCovers(dir, 100)
	var left []string
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		left = append(left, e.Name())
	}
	if len(left) != 2 || left[0] != "newer" || left[1] != "newest" {
		t.Fatalf("left %v", left)
	}
	// Within the limit, nothing goes.
	pruneCovers(dir, 100)
	if entries, _ := os.ReadDir(dir); len(entries) != 2 {
		t.Fatalf("pruned within the limit: %d left", len(entries))
	}
}
