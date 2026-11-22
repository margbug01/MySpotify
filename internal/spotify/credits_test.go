package spotify

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
)

// Credits are asked for page by page until a page comes short, and grouped
// as Spotify groups them, each person once in a group with all they did.
// Asked without a limit, Spotify gave only the first 10, which left out most
// songs' producers, engineers and players.
func TestTrackCreditsReadsEveryPage(t *testing.T) {
	type item struct {
		Name, Role, URI, Group string
	}
	all := []item{
		{"周杰倫", "主要艺人", "spotify:artist:jay", "艺人"},
		{"周杰倫", "作曲者", "spotify:artist:jay", "作曲和作词"},
		{"周杰倫", "作词者", "spotify:artist:jay", "作曲和作词"},
	}
	for i := len(all); i < creditsPage; i++ {
		all = append(all, item{fmt.Sprint("乐手", i), "吉他", "", "表演者"})
	}
	// The second page, past the first hundred.
	all = append(all, item{"杨瑞代", "录音工程师", "spotify:artist:yang", "制作兼工程"}, item{"周杰倫", "制作人", "spotify:artist:jay", "制作兼工程"})

	var offsets []float64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			Variables map[string]any `json:"variables"`
		}
		json.NewDecoder(r.Body).Decode(&q)
		off, _ := q.Variables["contributorsOffset"].(float64)
		lim, _ := q.Variables["contributorsLimit"].(float64)
		offsets = append(offsets, off)
		var items []string
		for _, it := range all[min(int(off), len(all)):min(int(off+lim), len(all))] {
			uri := "null"
			if it.URI != "" {
				uri = `"` + it.URI + `"`
			}
			items = append(items, fmt.Sprintf(`{"name":%q,"role":%q,"uri":%s,"roleGroup":{"name":%q}}`, it.Name, it.Role, uri, it.Group))
		}
		fmt.Fprintf(w, `{"data":{"trackUnion":{"creditsTrait":{"contributors":{"items":[%s]},"sources":{"items":[{"name":"Universal Music Taiwan"}]}}}}}`, strings.Join(items, ","))
	}))
	defer srv.Close()
	pf := pathfinderURL
	pathfinderURL = srv.URL
	SetIdentity(Identity{Token: "t", Expires: time.Now().Add(time.Hour)})
	defer func() {
		pathfinderURL = pf
		SetIdentity(Identity{})
	}()

	cr, err := TrackCredits("5pIcwtJYNJx93l420oR2Vm")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(offsets, []float64{0, creditsPage}) {
		t.Errorf("asked at offsets %v", offsets)
	}
	var groups []string
	for _, g := range cr.Groups {
		groups = append(groups, fmt.Sprint(g.Name, len(g.People)))
	}
	if want := []string{"艺人1", "作曲和作词1", "表演者97", "制作兼工程2"}; !slices.Equal(groups, want) {
		t.Errorf("groups %v, want %v", groups, want)
	}
	if w := cr.Groups[1].People[0]; w.Name != "周杰倫" || w.Artist != "spotify:artist:jay" || !slices.Equal(w.Roles, []string{"作曲者", "作词者"}) {
		t.Errorf("songwriter %+v", w)
	}
	if !slices.Equal(cr.Sources, []string{"Universal Music Taiwan"}) {
		t.Errorf("sources %v", cr.Sources)
	}
}
