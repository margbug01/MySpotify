package app

import (
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"vinyl/internal/spotify"
)

// Headless testers have no application locale event loop.
func testLanguage(t *testing.T, tag string) {
	t.Helper()
	previous := i18n.Locale()
	t.Setenv("MYGO_LOCALE", tag)
	i18n.SetLocaleFunc(func() string {
		if locale := os.Getenv("MYGO_LOCALE"); locale != "" {
			return locale
		}
		return "zh-Hans"
	})
	t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return previous }) })
}

// Selecting a language applies its tag and persists it without changing other choices.
func TestLanguageSelection(t *testing.T) {
	testLanguage(t, "zh-Hans")
	old := setLocale
	defer func() { setLocale = old }()
	var tags []string
	setLocale = func(tag string) { tags = append(tags, tag); t.Setenv("MYGO_LOCALE", tag) }
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	a.settings.Volume = 0.37
	a.open(route{page: pageSettings})
	tt := ui.NewTester(a.view, 1180, 1000)
	tt.Frame()
	if err := tt.Click("界面语言"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(300 * time.Millisecond)
	tt.Frame()
	if err := tt.Click("English"); err != nil {
		t.Fatalf("%v; shows %q", err, tt.Texts())
	}
	tt.Frame()
	tt.Frame()
	if a.settings.Language != "en" || len(tags) != 1 || tags[0] != "en" {
		t.Fatalf("language %q, applied %v", a.settings.Language, tags)
	}
	b, err := os.ReadFile(a.settings.path)
	if err != nil {
		t.Fatal(err)
	}
	var saved settings
	if err := json.Unmarshal(b, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Language != "en" || saved.Volume != 0.37 {
		t.Fatalf("saved language %q, volume %v", saved.Language, saved.Volume)
	}
	if err := tt.Click("Interface language"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(300 * time.Millisecond)
	tt.Frame()
	tt.Key(0, ui.KeyHome)
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	tt.Frame()
	if a.settings.Language != "" || len(tags) != 2 || tags[1] != "" {
		t.Fatalf("system choice %q, applied %v", a.settings.Language, tags)
	}
}

// English dates and compact counts keep their values, and media actions keep their names.
func TestEnglishFormatting(t *testing.T) {
	testLanguage(t, "en")
	day := time.Date(2026, time.November, 10, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct{ got, want string }{
		{shortDate(day, false), "Nov 10"}, {shortDate(day, true), "Nov 10, 2026"},
		{dailyMonth(day), "Nov · Tue"}, {dailyDate(day), "Tue, Nov 10"},
		{plays(12000), "12K"}, {plays(120000000), "120M"},
		{mvAction("Show", "Lyrics"), "Show Lyrics"}, {mvAction("Play", "Video"), "Play Video"},
	} {
		if tc.got != tc.want {
			t.Errorf("got %q, want %q", tc.got, tc.want)
		}
	}
}

// A cached home in another language keeps account data but cannot restore its old headings.
func TestHomeCacheLanguage(t *testing.T) {
	testLanguage(t, "zh-Hans")
	a := newTestApp()
	a.homeFile = filepath.Join(t.TempDir(), "home.json")
	a.homeSections = testSections(1)
	a.mine = []spotify.Playlist{{ID: "spotify:playlist:mine", Name: "我的原名"}}
	a.keepHome()
	testLanguage(t, "en")
	b := newTestApp()
	b.homeFile = a.homeFile
	b.restoreHome()
	if b.profile == nil || len(b.mine) != 1 || b.mine[0].Name != "我的原名" {
		t.Fatal("account data was lost or renamed")
	}
	if len(b.homeSections) != 0 {
		t.Fatal("restored headings from the previous language")
	}
}

// Switching languages reloads a selected home facet instead of leaving its skeleton forever.
func TestLanguageRefreshKeepsChosenFacet(t *testing.T) {
	testLanguage(t, "en")
	oldFeed := homeFeed
	defer func() { homeFeed = oldFeed }()
	asked := make(chan string, 1)
	homeFeed = func(id string) (*spotify.Home, error) {
		asked <- id
		return &spotify.Home{}, nil
	}
	a := newTestApp()
	a.route = route{page: pageSettings}
	a.homeFacet = "music-chip"
	a.facets = map[string][]spotify.HomeSection{"music-chip": testSections(1)}
	a.loading = map[string]bool{"home:music-chip": true}
	a.languageGen = 4
	a.reloadLocalizedPages()
	select {
	case id := <-asked:
		if id != "music-chip" {
			t.Fatalf("reloaded facet %q", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("selected facet was not reloaded")
	}
	if a.homeFacet != "music-chip" || a.languageGen != 5 || a.facets != nil || a.profile == nil {
		t.Fatal("language refresh lost the selection or kept the old localized data")
	}
}

// With NM_FRAMES set, save English pages at normal and minimum widths for visual review.
func TestEnglishFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	testLanguage(t, "en")
	for _, width := range []int{880, 1180} {
		a := editTestApp()
		a.profile.Nickname = "Alex"
		a.settings = defaultSettings()
		a.settings.Language = "en"
		tt := ui.NewTester(a.view, width, 780)
		save := func(name string) {
			t.Helper()
			tt.Frame()
			tt.Advance(time.Second)
			tt.Frame()
			file, err := os.Create(filepath.Join(dir, fmt.Sprintf("english-%s-%d.png", name, width)))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(file, tt.Image())
			file.Close()
			if err != nil {
				t.Fatal(err)
			}
		}
		for _, page := range []struct {
			name  string
			route route
		}{
			{"playlist", a.route}, {"settings", route{page: pageSettings}},
		} {
			a.route = page.route
			a.pageScroll = ui.ScrollState{}
			save(page.name)
		}
		tt.SetDark(true)
		save("settings-dark")
		a.pl.Cue([]spotify.Track{{ID: "preview", Name: "A song with a longer title", Artists: []spotify.Artist{{Name: "Original Artist"}}, Duration: 200000}})
		a.nowPlaying, a.npPane = true, paneLyrics
		tt.Move(float32(width)/2, 300)
		save("now-playing")
	}
}
