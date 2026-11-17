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

// A Daily Mix only in the grid of shortcuts is still the day's songs: with
// none in the sections, the daily page said 暂时没有推荐.
func TestHomeFeedFindsTheDailyMixInTheShortcuts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":{"home":{"sectionContainer":{"sections":{"items":[
{"uri":"spotify:section:shorts","data":{"__typename":"HomeShortsSectionData"},
 "sectionItems":{"items":[{"content":{"data":{"__typename":"Playlist","uri":"spotify:playlist:37i9dQZF1E35mix","name":"每日推荐 2"}}}]}},
{"uri":"spotify:section:radio","data":{"__typename":"HomeGenericSectionData","title":{"transformedLabel":"推荐电台"}},
 "sectionItems":{"items":[{"content":{"data":{"__typename":"Playlist","uri":"spotify:playlist:37i9dQZF1E4radio","name":"电台"}}}]}}
]}}}}}`)
	}))
	defer srv.Close()
	pf := pathfinderURL
	pathfinderURL = srv.URL
	SetIdentity(Identity{Token: "t", Expires: time.Now().Add(time.Hour)})
	defer func() {
		pathfinderURL = pf
		SetIdentity(Identity{})
	}()

	h, err := HomeFeed("")
	if err != nil {
		t.Fatal(err)
	}
	if h.Daily != "spotify:playlist:37i9dQZF1E35mix" {
		t.Errorf("daily = %q", h.Daily)
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

// A podcast's page decodes each episode's release date alongside its duration and playability.
func TestShowEpisodesIncludeReleaseDate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			OperationName string `json:"operationName"`
		}
		json.NewDecoder(r.Body).Decode(&q)
		switch q.OperationName {
		case "queryShowMetadataV2":
			fmt.Fprint(w, `{"data":{"podcastUnionV2":{"__typename":"Podcast","uri":"spotify:show:s1","name":"相声精选","publisher":{"name":"德云社"}}}}`)
		case "queryPodcastEpisodes":
			fmt.Fprint(w, `{"data":{"podcastUnionV2":{"episodesV2":{"totalCount":1,"items":[{"entity":{"_uri":"spotify:episode:e1","data":{"name":"第一集","duration":{"totalMilliseconds":665000},"releaseDate":{"isoString":"2026-03-15T08:00:00Z"},"playability":{"playable":true}}}}]}}}}`)
		}
	}))
	defer srv.Close()
	pf := pathfinderURL
	pathfinderURL = srv.URL
	SetIdentity(Identity{Token: "t", Expires: time.Now().Add(time.Hour)})
	defer func() {
		pathfinderURL = pf
		SetIdentity(Identity{})
	}()

	pl, err := PlaylistDetail("spotify:show:s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(pl.Tracks) != 1 || pl.Tracks[0].Released != "2026-03-15" || pl.Tracks[0].Duration != 665000 {
		t.Fatalf("tracks = %+v", pl.Tracks)
	}
}

// The liked songs Spotify names and pictures in the listener's language,
// as in what was played lately, take Vinyl's name and no picture: they
// showed as 已点赞的歌曲 beside 我喜欢的音乐 in the sidebar (#14).
func TestLikedSongsKeepTheirNameFromSpotify(t *testing.T) {
	var e gqlEntity
	err := json.Unmarshal([]byte(`{"__typename":"Playlist","uri":"spotify:collection:tracks","name":"已点赞的歌曲",
		"images":{"items":[{"sources":[{"url":"https://misc.scdn.co/liked-songs/liked-songs-640.png","width":640}]}]},"content":{"totalCount":12}}`), &e)
	if err != nil {
		t.Fatal(err)
	}
	pl, ok := e.playlist()
	if !ok || !pl.Liked() || pl.Name != likedName || pl.Cover() != "" || pl.TrackCount != 12 {
		t.Errorf("liked songs = %+v, %v", pl, ok)
	}
}

// A home with no Daily Mix anywhere, as some days, takes the first of the
// Made For You page's: the daily page said 暂时没有推荐 (#42). A chip's home
// asks for none.
func TestHomeFeedFindsTheDailyMixMadeForYou(t *testing.T) {
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			Op        string         `json:"operationName"`
			Variables map[string]any `json:"variables"`
		}
		json.NewDecoder(r.Body).Decode(&q)
		asked = append(asked, q.Op)
		if q.Op == "browsePage" {
			if q.Variables["uri"] != madeForYouPage {
				t.Errorf("browsed %v", q.Variables["uri"])
			}
			fmt.Fprint(w, `{"data":{"browse":{"sections":{"items":[
{"uri":"spotify:section:personal","sectionItems":{"items":[{"content":{"data":{"__typename":"Playlist","uri":"spotify:playlist:37i9dQZF1EP6daylist","name":"daylist"}}}]}},
{"uri":"spotify:section:daily","sectionItems":{"items":[{"content":{"data":{"__typename":"Playlist","uri":"spotify:playlist:37i9dQZF1E36mix","name":"每日推荐 1"}}}]}}
]}}}}`)
			return
		}
		fmt.Fprint(w, `{"data":{"home":{"sectionContainer":{"sections":{"items":[
{"uri":"spotify:section:radio","data":{"__typename":"HomeGenericSectionData","title":{"transformedLabel":"推荐电台"}},
 "sectionItems":{"items":[{"content":{"data":{"__typename":"Playlist","uri":"spotify:playlist:37i9dQZF1E4radio","name":"电台"}}}]}}
]}}}}}`)
	}))
	defer srv.Close()
	pf := pathfinderURL
	pathfinderURL = srv.URL
	SetIdentity(Identity{Token: "t", Expires: time.Now().Add(time.Hour)})
	defer func() {
		pathfinderURL = pf
		SetIdentity(Identity{})
	}()

	h, err := HomeFeed("")
	if err != nil {
		t.Fatal(err)
	}
	if h.Daily != "spotify:playlist:37i9dQZF1E36mix" {
		t.Errorf("daily = %q", h.Daily)
	}
	asked = nil
	if _, err := HomeFeed("music-chip"); err != nil || fmt.Sprint(asked) != "[home]" {
		t.Errorf("a chip's home asked %v, %v", asked, err)
	}
}

// A failed daily lookup keeps the usable home and reports failure, rather than clearing the cached songs (#42).
func TestHomeFeedReportsDailyFailure(t *testing.T) {
	for _, tc := range []struct {
		name, browse string
		failed       bool
	}{
		{"null with error", `{"data":{"browse":null},"errors":[{"message":"unavailable"}]}`, true},
		{"missing page", `{"data":{"browse":null}}`, true},
		{"no mixes", `{"data":{"browse":{"sections":{"items":[]}}}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var q struct {
					Op string `json:"operationName"`
				}
				json.NewDecoder(r.Body).Decode(&q)
				if q.Op == "browsePage" {
					fmt.Fprint(w, tc.browse)
				} else {
					fmt.Fprint(w, `{"data":{"home":{"greeting":{"transformedLabel":"hello"},"sectionContainer":{"sections":{"items":[]}}}}}`)
				}
			}))
			defer srv.Close()
			pf := pathfinderURL
			pathfinderURL = srv.URL
			SetIdentity(Identity{Token: "t", Expires: time.Now().Add(time.Hour)})
			defer func() { pathfinderURL = pf; SetIdentity(Identity{}) }()
			h, err := HomeFeed("")
			if (err != nil) != tc.failed || h == nil || h.Greeting != "hello" || h.Daily != "" {
				t.Fatalf("home = %+v, error = %v, want failed=%v and usable home", h, err, tc.failed)
			}
		})
	}
}

// Daily Mixes beyond the first page are found, while an explicit last page and broken pagination stop (#42).
func TestMadeForYouDailyFollowsPagination(t *testing.T) {
	mix := `{"content":{"data":{"__typename":"Playlist","uri":"spotify:playlist:37i9dQZF1E3later","name":"Daily Mix 1"}}}`
	for _, tc := range []struct {
		name               string
		wantMix, wantError bool
	}{
		{"sections", true, false}, {"items", true, false}, {"short item page", true, false}, {"last page", false, false}, {"limit", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var offsets [][2]int
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var q struct {
					Variables struct {
						Page struct {
							Offset int `json:"offset"`
						} `json:"pagePagination"`
						Section struct {
							Offset int `json:"offset"`
						} `json:"sectionPagination"`
					} `json:"variables"`
				}
				json.NewDecoder(r.Body).Decode(&q)
				p, s := q.Variables.Page.Offset, q.Variables.Section.Offset
				offsets = append(offsets, [2]int{p, s})
				if p == 20 || s == 20 || s == 5 {
					fmt.Fprintf(w, `{"data":{"browse":{"sections":{"pagingInfo":{"nextOffset":null},"items":[{"sectionItems":{"items":[%s]}}]}}}}`, mix)
					return
				}
				switch tc.name {
				case "sections":
					fmt.Fprint(w, `{"data":{"browse":{"sections":{"totalCount":21,"pagingInfo":{"nextOffset":20},"items":[{"sectionItems":{"items":[]}}]}}}}`)
				case "items":
					fmt.Fprint(w, `{"data":{"browse":{"sections":{"pagingInfo":{"nextOffset":null},"items":[{"sectionItems":{"totalCount":21,"pagingInfo":{"nextOffset":20},"items":[{}]}}]}}}}`)
				case "short item page":
					fmt.Fprint(w, `{"data":{"browse":{"sections":{"items":[{"sectionItems":{"pagingInfo":{"nextOffset":20},"items":[{}]}},{"sectionItems":{"pagingInfo":{"nextOffset":5},"items":[{}]}}]}}}}`)
				case "limit":
					fmt.Fprint(w, `{"data":{"browse":{"sections":{"pagingInfo":{"nextOffset":100},"items":[]}}}}`)
				default:
					fmt.Fprint(w, `{"data":{"browse":{"sections":{"totalCount":11,"pagingInfo":{"nextOffset":null},"items":[{"sectionItems":{"items":[]}}]}}}}`)
				}
			}))
			defer srv.Close()
			pf := pathfinderURL
			pathfinderURL = srv.URL
			SetIdentity(Identity{Token: "t", Expires: time.Now().Add(time.Hour)})
			defer func() { pathfinderURL = pf; SetIdentity(Identity{}) }()
			daily, err := madeForYouDaily()
			if (err != nil) != tc.wantError || (daily != "") != tc.wantMix {
				t.Fatalf("daily=%q err=%v offsets=%v", daily, err, offsets)
			}
			if tc.wantMix && daily != "spotify:playlist:37i9dQZF1E3later" {
				t.Fatalf("wrong mix: %s", daily)
			}
			want := [][2]int{{0, 0}}
			if tc.name == "sections" {
				want = append(want, [2]int{20, 0})
			}
			if tc.name == "items" {
				want = append(want, [2]int{0, 20})
			}
			if tc.name == "short item page" {
				want = append(want, [2]int{0, 5})
			}
			if fmt.Sprint(offsets) != fmt.Sprint(want) {
				t.Fatalf("offsets=%v want=%v", offsets, want)
			}
		})
	}
}
