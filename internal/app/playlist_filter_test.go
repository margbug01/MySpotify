package app

import (
	"fmt"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"vinyl/internal/spotify"
)

// filterApp is a test app showing a playlist longer than a screen, whose
// first three songs differ in what a filter word can hit: a name, an
// artist, an album, in Chinese and in Latin letters.
func filterApp() *app {
	a := newTestApp()
	a.settings = defaultSettings()
	a.playlists["42"] = &spotify.Playlist{ID: "42", Name: "筛歌单", Tracks: []spotify.Track{
		{ID: "1", Name: "晴天", Artists: []spotify.Artist{{ID: "a", Name: "周杰伦"}},
			Album: spotify.Album{ID: "spotify:album:1", Name: "叶惠美"}, Duration: 200000},
		{ID: "2", Name: "富士山下", Artists: []spotify.Artist{{ID: "b", Name: "陳奕迅"}},
			Album: spotify.Album{ID: "spotify:album:2", Name: "What's Going On…?"}, Duration: 200000},
		{ID: "3", Name: "Under the Weather", Artists: []spotify.Artist{{ID: "c", Name: "Jay Chou"}},
			Album: spotify.Album{ID: "spotify:album:3", Name: "叶惠美"}, Duration: 200000},
	}}
	// The rest only make the list long enough for its filter to show.
	for i := 4; i <= filterMinTracks+1; i++ {
		a.playlists["42"].Tracks = append(a.playlists["42"].Tracks, spotify.Track{
			ID: fmt.Sprintf("%d", i), Name: fmt.Sprintf("Track %d", i),
			Artists: []spotify.Artist{{ID: "x", Name: "Nobody"}},
			Album:   spotify.Album{ID: "spotify:album:x", Name: "Nowhere"}, Duration: 200000,
		})
	}
	a.route = route{page: pagePlaylist, playlist: "42"}
	return a
}

// filterTracks matches a word wherever it appears in order in a track's
// name, an artist or the album, letters' case and words' order aside, and
// keeps only the tracks every word matches. A query with no matches is not
// the same as no query at all.
func TestFilterTracksMatching(t *testing.T) {
	a := filterApp()
	tracks := a.playlists["42"].Tracks
	for _, it := range []struct {
		query string
		want  []int
		whole bool // nil leaves the playlist whole, an empty set leaves no rows
	}{
		{"晴天", []int{0}, false},             // the name itself
		{"天晴", []int{}, false},              // the characters in order: 晴 before 天
		{"叶美", []int{0, 2}, false},          // a subsequence skips 惠, by the album
		{"weather", []int{2}, false},        // letters' case lets nothing differ
		{"JAY", []int{2}, false},            // an artist's name
		{"叶 周杰伦", []int{0}, false},          // two words, the album's and the artist's
		{"叶 富士", []int{}, false},            // no track answers to both words
		{"", nil, true}, {"   ", nil, true}, // empty of words, the playlist stays whole
	} {
		got := filterTracks(tracks, it.query)
		if !slices.Equal(got, it.want) {
			t.Errorf("filterTracks(%q) matched %v, want %v", it.query, got, it.want)
		}
		if (got == nil) != it.whole {
			t.Errorf("filterTracks(%q) left the playlist whole: %v, want %v", it.query, got == nil, it.whole)
		}
	}
}

// The filter's icon at the header's end opens its field in place, which
// narrows the rows to the matches; emptying it gives the whole playlist
// back, where a row click queues it.
func TestPlaylistFilterRows(t *testing.T) {
	a := filterApp()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("筛选"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	tt.Frame()
	if !tt.Focused("筛选") {
		t.Fatal("the filter's field did not take the focus")
	}
	tt.Type("晴天")
	tt.Frame()
	if err := tt.Click("富士山下"); err == nil {
		t.Error("the song the filter does not match still shows")
	}
	// Escape empties the field while it holds any, and the playlist shows
	// whole again, where a row click queues it.
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if err := tt.Click("晴天"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("富士山下"); err != nil {
		t.Fatal(err)
	}
	if got, want := len(a.pl.Queue()), len(a.playlists["42"].Tracks); got != want {
		t.Errorf("the click with the filter emptied queued %d songs, want the playlist's %d", got, want)
	}
	if a.plFilter != "" {
		t.Errorf("the filter kept the word %q", a.plFilter)
	}
}

// A list of a screen or less shows no filter at all: the icon is not
// there, and the shortcut leaves the search it usually opens instead.
func TestPlaylistFilterHiddenOnShortLists(t *testing.T) {
	a := filterApp()
	a.playlists["42"].Tracks = a.playlists["42"].Tracks[:filterMinTracks]
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("筛选"); err == nil {
		t.Error("a list of a screen or less shows the filter")
	}
	tt.Key(ui.Cmd, ui.KeyF)
	tt.Frame()
	tt.Frame()
	tt.Frame()
	if tt.Focused("筛选") {
		t.Error("the shortcut opened a filter a short list has no place for")
	}
}

// Left empty, the field closes as the focus leaves it, the header going back
// to its icon; a word in it keeps it open, so that the rows stay narrowed.
func TestPlaylistFilterClosesWhenLeftEmpty(t *testing.T) {
	a := filterApp()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("筛选"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	tt.Frame()
	if !tt.Focused("筛选") {
		t.Fatal("the filter's field did not take the focus")
	}
	// A click on the rows takes the focus, and with no word in the field
	// the filter closes rather than staying open.
	if err := tt.Click("晴天"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.filterOpen {
		t.Error("the empty filter stayed open after the focus left it")
	}
	if err := tt.Click("筛选"); err != nil {
		t.Fatal("the icon did not come back:", err)
	}
	// A word keeps the field open when the focus leaves it.
	tt.Frame()
	tt.Frame()
	tt.Frame()
	tt.Type("晴")
	tt.Frame()
	if err := tt.Click("晴天"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.filterOpen {
		t.Error("the filter closed with its word still in it")
	}
}

// A word no song answers to empties the rows and says so, rather than
// showing the playlist whole; Escape brings the songs back. The empty
// footer is built where the last row would be, which is no row at all.
func TestPlaylistFilterNoMatches(t *testing.T) {
	a := filterApp()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("筛选"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	tt.Frame()
	tt.Type("天晴")
	tt.Frame()
	if !tt.HasText("没有匹配的歌曲") {
		t.Error("the empty rows say nothing of why they are empty")
	}
	for _, name := range []string{"晴天", "富士山下", "Under the Weather", "Track 4"} {
		if err := tt.Click(name); err == nil {
			t.Errorf("%q shows though the filter matches nothing", name)
		}
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if err := tt.Click("富士山下"); err != nil {
		t.Fatal(err)
	}
}

// While the filter holds anything, the playlist's play button queues its
// matches, in the order the playlist has them.
func TestPlaylistFilterPlayQueuesMatches(t *testing.T) {
	a := filterApp()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("筛选"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	tt.Frame()
	tt.Type("叶")
	tt.Frame()
	if err := tt.Click("播放"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	queue := a.pl.Queue()
	if len(queue) != 2 || queue[0].ID != "1" || queue[1].ID != "3" {
		t.Fatalf("the play button queued %v, want the two matches in the playlist's order", ids(queue))
	}
	if cur := a.pl.Current(); cur == nil || cur.ID != "1" {
		t.Errorf("playing started at %v, want the first match", cur)
	}
}

// Leaving the page drops the filter: back to it, the songs are whole.
func TestPlaylistFilterResetsOnLeave(t *testing.T) {
	a := filterApp()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("筛选"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	tt.Frame()
	tt.Type("晴天")
	tt.Frame()
	a.open(route{page: pageHome})
	a.open(route{page: pagePlaylist, playlist: "42"})
	if a.plFilter != "" {
		t.Fatalf("the filter came along: word %q", a.plFilter)
	}
	tt.Frame()
	if err := tt.Click("富士山下"); err != nil {
		t.Fatal(err)
	}
}

func ids(tracks []spotify.Track) []string {
	var out []string
	for _, tr := range tracks {
		out = append(out, tr.ID)
	}
	return out
}
