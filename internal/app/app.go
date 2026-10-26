package app

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"vinyl/internal/player"
	"vinyl/internal/spotify"
)

type pageKind int

const (
	pageHome pageKind = iota
	pageDaily
	pageCharts
	pagePlaylist
	pageSearch
	pageArtist
	pageRecent
)

type route struct {
	page     pageKind
	playlist string
	artist   string // its URI
}

// app is the state both windows show. Everything in it is read and changed
// on the main thread: by the views, and by goroutines through post.
type app struct {
	win   *mygo.Window
	notch *notch
	art   *artCache
	pl    *player.Player
	pal   palette

	route   route
	history []route
	forward []route // the pages gone back from, the last first to go to

	profile                 *spotify.Profile
	mine, saved             []spotify.Playlist   // the listener's playlists: made, and saved
	followed                []spotify.ArtistCard // the artists the listener follows
	next                    *route               // the page to open once the frame is built (goTo)
	recPlaylists            []spotify.Playlist   // the day's playlists picked for the listener
	signin                  signin               // the web player's window, which signs the listener in
	accountOpen             bool                 // the dropdown of the account row shows
	liked                   map[string]bool      // the songs the listener likes
	toast                   string               // a message to show once
	naming                  nameDialog           // the name of a playlist asked for
	deleting                deleteDialog         // whether to delete a playlist, asked
	radioStarting           string               // the radio asked for its first songs, or ""
	played                  []spotify.Track      // the songs Vinyl played, the latest first
	recentLists             []spotify.Playlist   // what the listener played in Spotify's apps
	recentArtists           []spotify.ArtistCard
	recentLoading           bool
	foundAlbums, foundLists []spotify.Playlist // the albums and playlists a search found

	daily       []spotify.Track
	dailySpin   spinState // the record on the daily card
	dailyBars   barState  // the bars on its play button
	featured    []spotify.Playlist
	homeArtists []spotify.ArtistCard
	charts      []spotify.Playlist
	homeErr     string
	homeLoading bool

	playlists map[string]*spotify.Playlist
	loadingPl map[string]bool
	plErr     map[string]string

	artists    map[string]*spotify.ArtistPage
	loadingAr  map[string]bool
	arErr      map[string]string
	loadingRel map[string]bool // an artist's releases of a kind, all asked for
	trackList  ui.ListState
	// Where the page and the sidebar are scrolled, kept while Now Playing
	// covers them, unbuilt.
	pageScroll ui.ScrollState
	navScroll  ui.ScrollState

	query     string
	searched  string
	results   []spotify.Track
	found     []spotify.ArtistCard // the artists matching the search
	searching bool
	searchErr string
	searchBox ui.Handle

	nowPlaying  bool
	npOpen      bool      // Now Playing as it last showed: open or not
	npChangedAt time.Time // when it last opened or closed
	npPane      int       // paneLyrics, paneQueue or paneStage
	npPaneTop   float32   // how far the cover beside the pane starts below its top
	lyricScroll ui.ScrollState
	lyricGlide  lyricGlide
	lyricAt     int
	lyricTrack  string
	queueList   ui.ListState
	queueAt     int // the row of the queue pane last scrolled to, -1 while it is hidden

	scrub      float64
	scrubbing  bool
	volume     float64
	volumeOpen bool // the volume popover of the player bar shows
	mutedFrom  float64

	rowBars barState
	// underTop and underBottom say whether the page has content under its
	// bars, which show their scroll edges then.
	underTop, underBottom bool
	barScrub              scrubState
	npScrub               scrubState
	backdropCache         backdropCache // the backdrop of Now Playing, as last painted
	npArt                 *artwork      // the cover whose colors Now Playing shows
	npCoverState          coverState    // the cover of Now Playing, as it is tilted and flicked
	sig                   playerSig
	quitting              bool

	settings    settings
	settingsWin *mygo.Window

	restored     bool   // playback picked up where it was left
	savedSession string // the session as last kept
	sessionDue   bool   // a save of the session is due
}

// post runs fn on the main thread and redraws both windows. Without a
// window, as in tests, it drops fn.
func (a *app) post(fn func()) {
	if a.win == nil {
		return
	}
	a.win.Update(func() {
		fn()
		a.redraw()
	})
}

func (a *app) redraw() {
	if a.win != nil {
		a.win.Invalidate()
	}
	if a.notch != nil && a.notch.win != nil {
		a.notch.win.Invalidate()
	}
}

// shotHook, when set, drives the app to check its design.
var shotHook func(a *app)

func (a *app) showMain() {
	if a.win != nil {
		a.win.Show()
		a.win.Focus()
	}
}

// Run runs the app until it quits.
func Run() error {
	a := &app{
		playlists:  map[string]*spotify.Playlist{},
		loadingPl:  map[string]bool{},
		plErr:      map[string]string{},
		artists:    map[string]*spotify.ArtistPage{},
		loadingAr:  map[string]bool{},
		arErr:      map[string]string{},
		loadingRel: map[string]bool{},
		lyricAt:    -1,
		queueAt:    -1,
	}
	// Its data, the web player's sign-in among it, lives under this name,
	// whatever the executable is called.
	mygo.App.SetName("Vinyl")
	if dir, err := mygo.App.Path(mygo.PathUserData); err == nil {
		log.SetOutput(io.MultiWriter(&logFile{path: filepath.Join(dir, "vinyl.log")}, os.Stderr))
		spotify.OpsFile = filepath.Join(dir, "queries.json")
		spotify.LoadOps()
	}
	log.Printf("start, pid %d", os.Getpid())
	// The page that plays starts without a click; its GPU work, which
	// it hardly has, runs in the web view's main process rather than a
	// process of its own (50 MB).
	os.Setenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", strings.TrimSpace(os.Getenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS")+" --autoplay-policy=no-user-gesture-required --in-process-gpu"))
	// What decoding a page of covers takes goes back to the system after,
	// rather than staying in the heap (20 MB).
	if os.Getenv("GOMEMLIMIT") == "" {
		debug.SetMemoryLimit(64 << 20)
	}
	mygo.App.WhenReady(func() {
		a.settings = loadSettings()
		a.settings.applyAppearance()
		a.npPane = a.lyricsPaneChosen()
		// Before the window: on Windows it lays out its content as it is
		// made, and the view asks the player what plays.
		a.art = newArtCache(a.post)
		a.pl = player.New(a.post)
		if web, err := player.NewWeb(); err == nil {
			a.pl.Web = web
		} else {
			log.Printf("player: %v", err)
		}
		a.pl.Changed = func() {
			a.notePlayed()
			a.keepSession()
		}
		a.played = loadPlayed()
		a.volume = a.settings.Volume
		a.pl.SetVolume(a.volume)
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:         mygo.App.Name(),
			Width:         1180,
			Height:        780,
			MinWidth:      880,
			MinHeight:     580,
			StateKey:      "main",
			TitleBarStyle: mygo.TitleBarHidden,
			// As tall as the edge over a page (Linux, Windows).
			TitleBarHeight: buttonsHeight,
			// Inset from the corner, within the sidebar, as macOS 26's
			// apps with a sidebar place them.
			TrafficLightPosition: &mygo.Point{X: 20, Y: 20},
			Vibrancy:             sidebarVibrancy(),
			BackgroundColor:      "light-dark(#ffffff, #141416)",
			Content:              ui.View(a.view),
		})
		a.setMenu()
		a.boot()
		// The notch player is macOS's, over its menu bar: elsewhere it would
		// hang at the top of the screen as a black box.
		if runtime.GOOS == "darwin" {
			a.notch = newNotch(a)
		}
		if shotHook != nil {
			shotHook(a)
		}
		// On macOS closing the window hides it: music plays on, under the
		// notch, and the Dock icon brings it back. Elsewhere nothing would
		// bring it back, so closing it quits.
		a.win.OnClose(func(e *mygo.CloseEvent) {
			if a.quitting {
				return
			}
			e.PreventDefault()
			if runtime.GOOS == "darwin" {
				a.win.Hide()
			} else {
				mygo.App.Quit()
			}
		})
	})
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
		a.quitting = true
		a.settings.Volume = a.volume
		a.settings.save()
		a.saveSession()
	})
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.OnActivate(func(bool) { a.showMain() })
	return mygo.App.Run()
}

// sidebarVibrancy is the material behind the sidebar: macOS's. Windows'
// stays light under a dark theme, the sidebar's text unreadable on it, so
// the sidebar is painted there.
func sidebarVibrancy() mygo.Vibrancy {
	if runtime.GOOS == "darwin" {
		return mygo.VibrancySidebar
	}
	return mygo.VibrancyNone
}
