package spotify

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A query whose hash the service no longer knows reads the web player's
// scripts again, the main one and one it loads as needed, takes the new
// hash, keeps it for the next start, and goes again; a hash still not
// known fails, without reading them again at once.
func TestStaleQueryReadsScriptsAgain(t *testing.T) {
	newSearch := strings.Repeat("b", 64)
	newProfile := strings.Repeat("c", 64)
	var reads int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			reads++
			fmt.Fprintf(w, `<script src="/cdn/web-player.f00.js"></script>`)
		case "/cdn/web-player.f00.js":
			fmt.Fprintf(w, `x=new aQ.l("profileAttributes","query","%s",null);`+
				`r.u=e=>""+(({7:"xpui-routes-search"})[e]||e)+"."+({7:"abcd1234",9:"ffff0000"})[e]+".js",u.min`, newProfile)
		case "/cdn/xpui-routes-search.abcd1234.js":
			fmt.Fprintf(w, `y=new aQ.l("searchTracks","query","%s",null)`, newSearch)
		case "/cdn/9.ffff0000.js":
			fmt.Fprint(w, `nothing`)
		case "/query":
			var q struct {
				Extensions struct {
					PersistedQuery struct {
						Hash string `json:"sha256Hash"`
					} `json:"persistedQuery"`
				} `json:"extensions"`
			}
			json.NewDecoder(r.Body).Decode(&q)
			if h := q.Extensions.PersistedQuery.Hash; h != newSearch && h != newProfile {
				w.WriteHeader(http.StatusPreconditionFailed)
				fmt.Fprint(w, `"Invalid query hash"`)
				return
			}
			fmt.Fprint(w, `{"data":{"ok":true}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	saved := maps.Clone(ops)
	pf, scripts := pathfinderURL, scriptsURL
	pathfinderURL, scriptsURL = srv.URL+"/query", srv.URL+"/"
	OpsFile = filepath.Join(t.TempDir(), "ops.json")
	SetIdentity(Identity{Token: "t", Expires: time.Now().Add(time.Hour)})
	defer func() {
		ops, pathfinderURL, scriptsURL, OpsFile = saved, pf, scripts, ""
		refreshed = time.Time{}
		SetIdentity(Identity{})
	}()
	refreshed = time.Time{}

	var out struct{ OK bool }
	if err := query("searchTracks", nil, &out); err != nil || !out.OK {
		t.Fatalf("query: %v, %+v", err, out)
	}
	if ops["searchTracks"] != newSearch || ops["profileAttributes"] != newProfile || reads != 1 {
		t.Fatalf("hashes %s %s after %d reads", ops["searchTracks"], ops["profileAttributes"], reads)
	}
	b, _ := os.ReadFile(OpsFile)
	if !strings.Contains(string(b), newSearch) {
		t.Errorf("kept %s", b)
	}

	// The scripts know no other hash of this one: it fails, and they
	// aren't read again so soon.
	if err := query("searchArtists", nil, &out); err == nil || reads != 1 {
		t.Errorf("unknown query: %v, after %d reads", err, reads)
	}
}

// With LIVE set, reads open.spotify.com's scripts, and finds there the
// hashes of every query Vinyl makes.
func TestLiveScriptHashes(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip("LIVE not set")
	}
	need := map[string]bool{}
	for name := range ops {
		need[name] = true
	}
	found, err := scriptHashes(need)
	if err != nil {
		t.Fatal(err)
	}
	for name, h := range ops {
		switch {
		case found[name] == "":
			t.Errorf("%s: not found", name)
		case found[name] != h:
			t.Logf("%s: %s now, was %s", name, found[name], h)
		}
	}
	t.Logf("%d hashes in the scripts", len(found))
}
