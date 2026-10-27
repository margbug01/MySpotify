package spotify

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// homeJSON is a home as the web player gets it, cut down: a section of
// mixed cards, one with nothing the page shows, the recently played, and
// cards the feed gives one by one.
const homeJSON = `{"data":{"home":{"greeting":{"transformedLabel":"下午好"},"sectionContainer":{"sections":{"items":[
{"uri":"spotify:section:mixed","data":{"__typename":"HomeGenericSectionData","title":{"transformedLabel":"热门专辑与单曲"}},
 "sectionItems":{"totalCount":30,"items":[
  {"content":{"data":{"__typename":"Album","uri":"spotify:album:a1","name":"专辑","coverArt":{"sources":[{"url":"https://i.scdn.co/image/a","width":300}]},"artists":{"items":[{"uri":"spotify:artist:x","profile":{"name":"歌手"}}]}}}},
  {"content":{"data":{"__typename":"Artist","uri":"spotify:artist:x","profile":{"name":"歌手"}}}},
  {"content":{"data":{"__typename":"Playlist","uri":"spotify:playlist:37i9dQZF1E3mix","name":"Daily Mix 1"}}},
  {"content":{"data":{"__typename":"Album","uri":"spotify:album:a1","name":"专辑"}}}]}},
{"uri":"spotify:section:onboarding","data":{"__typename":"HomeOnboardingSectionDataV2"},"sectionItems":{"items":[{"content":{}}]}},
{"uri":"spotify:section:recent","data":{"__typename":"HomeRecentlyPlayedSectionData","title":{"transformedLabel":"最近播放"}},
 "sectionItems":{"totalCount":1,"items":[{"content":{"data":{"__typename":"Playlist","uri":"spotify:playlist:p1","name":"Happy Hits!"}}}]}},
{"uri":"spotify:section:f1","data":{"__typename":"HomeFeedBaselineSectionData","title":{"transformedLabel":"热门专辑与单曲"}},
 "sectionItems":{"items":[{"content":{"data":{"__typename":"Album","uri":"spotify:album:a2","name":"另一张"}}}]}},
{"uri":"spotify:section:f2","data":{"__typename":"HomeFeedBaselineSectionData","title":{"transformedLabel":"你可能喜欢的单集"}},
 "sectionItems":{"items":[{"content":{"data":{"__typename":"Episode","uri":"spotify:episode:e1","name":"一集","duration":{"totalMilliseconds":1285596},
  "playability":{"playable":true},"releaseDate":{"isoString":"2026-10-26T04:30:00Z"},
  "podcastV2":{"data":{"uri":"spotify:show:s1","name":"节目","publisher":{"name":"主播"},"coverArt":{"sources":[{"url":"https://i.scdn.co/image/s","width":300}]}}}}}}]}},
{"uri":"spotify:section:f3","data":{"__typename":"HomeFeedBaselineSectionData","title":{"transformedLabel":""}},
 "sectionItems":{"items":[{"content":{"data":{"__typename":"Playlist","uri":"spotify:playlist:p2","name":"无题"}}}]}},
{"uri":"spotify:section:f4","data":{"__typename":"HomeFeedBaselineSectionData","title":{"transformedLabel":"你可能喜欢的单集"}},
 "sectionItems":{"items":[{"content":{"data":{"__typename":"Episode","uri":"spotify:episode:e1","name":"一集"}}}]}}
]}}}}}`

func TestHomeFeedKeepsSpotifysSections(t *testing.T) {
	var facet any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			Variables map[string]any `json:"variables"`
		}
		json.NewDecoder(r.Body).Decode(&q)
		facet = q.Variables["facet"]
		fmt.Fprint(w, homeJSON)
	}))
	defer srv.Close()
	pf := pathfinderURL
	pathfinderURL = srv.URL
	SetIdentity(Identity{Token: "t", Expires: time.Now().Add(time.Hour)})
	defer func() {
		pathfinderURL = pf
		SetIdentity(Identity{})
	}()

	h, err := HomeFeed("music-chip")
	if err != nil {
		t.Fatal(err)
	}
	if facet != "music-chip" {
		t.Errorf("facet sent = %v", facet)
	}
	var got []string
	for _, s := range h.Sections {
		var ids []string
		for _, it := range s.Items {
			ids = append(ids, it.ID())
		}
		got = append(got, fmt.Sprintf("%s|%s|%d|%v", s.Title, s.URI, s.Total, ids))
	}
	want := []string{
		// The album twice is once; the feed's card of the same title joins it.
		"热门专辑与单曲|spotify:section:mixed|30|[spotify:album:a1 spotify:artist:x spotify:playlist:37i9dQZF1E3mix spotify:album:a2]",
		"最近播放|spotify:section:recent|1|[spotify:playlist:p1]",
		"你可能喜欢的单集||1|[spotify:episode:e1]",
		"更多推荐||1|[spotify:playlist:p2]",
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("sections\n got %q\nwant %q", got, want)
	}
	for i, s := range h.Sections {
		if s.Recent != (i == 1) {
			t.Errorf("section %q: recent = %v", s.Title, s.Recent)
		}
	}
	if len(h.Shortcuts) != 1 || h.Shortcuts[0].ID() != "spotify:playlist:p1" {
		t.Errorf("shortcuts = %+v, want the recently played", h.Shortcuts)
	}
	if h.Daily != "spotify:playlist:37i9dQZF1E3mix" {
		t.Errorf("daily = %q", h.Daily)
	}
	it := h.Sections[2].Items[0]
	ep := it.Episode
	if ep == nil || !ep.Episode || ep.Duration != 1285596 || ep.Album.ID != "spotify:show:s1" || ep.Album.PicURL == "" ||
		ep.Artists[0].Name != "主播" || ep.Unplayable || it.Released != "2026-10-26" {
		t.Errorf("episode = %+v, released %q", ep, it.Released)
	}
	if al := h.Sections[0].Items[0].List; al == nil || al.Creator.Nickname != "歌手" || al.CoverURL == "" {
		t.Errorf("album = %+v", al)
	}
}

// Decorate names only the tracks without names, keeping their places in the
// queue, and does not take them for unplayable for want of a playability.
func TestDecorateNamesWhatIsOnlyAURI(t *testing.T) {
	var asked []any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			Variables map[string][]any `json:"variables"`
		}
		json.NewDecoder(r.Body).Decode(&q)
		asked = q.Variables["uris"]
		fmt.Fprint(w, `{"data":{"tracks":[{"__typename":"Track","uri":"spotify:track:b","name":"B","duration":{"totalMilliseconds":1000},"artists":{"items":[{"profile":{"name":"Singer"},"uri":"spotify:artist:s"}]},"albumOfTrack":{"uri":"spotify:album:x","name":"X"}},{"__typename":"Track","uri":"spotify:track:c","name":"C","artists":{"items":[{"profile":{"name":"Singer"},"uri":"spotify:artist:s"}]}}]}}`)
	}))
	defer srv.Close()
	pf := pathfinderURL
	pathfinderURL = srv.URL
	SetIdentity(Identity{Token: "t", Expires: time.Now().Add(time.Hour)})
	defer func() {
		pathfinderURL = pf
		SetIdentity(Identity{})
	}()

	// b only by URI, twice; c named without its artist, as Connect can.
	got, err := Decorate([]Track{{ID: "a", Name: "A", UID: "ua", Artists: []Artist{{Name: "Someone"}}}, {ID: "b", UID: "ub"}, {ID: "b", UID: "ub2"}, {ID: "c", Name: "C", Artists: []Artist{{ID: "spotify:artist:s"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(asked) != 2 || asked[0] != "spotify:track:b" || asked[1] != "spotify:track:c" {
		t.Fatalf("asked of %v", asked)
	}
	if got[0].Name != "A" || got[1].Name != "B" || got[1].UID != "ub" || got[2].UID != "ub2" || got[1].ArtistNames() != "Singer" || got[1].Duration != 1000 || !got[1].Playable() || got[3].ArtistNames() != "Singer" {
		t.Fatalf("decorated %+v", got)
	}
}
