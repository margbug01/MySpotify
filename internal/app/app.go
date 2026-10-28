package app

import (
	"io"
	"log"
	"net/http"
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
	pageTrack
	pageBrowse
	pageCategory
	pageUser
	pageSection
)

type route struct {
	page     pageKind
	playlist string
	artist   string // its URI
	track    string // its ID
	category string // its page's URI
	user     string // their username
	section  string // a home section's key (sectionKey)
}

// app is the state both windows show. Everything in it is read and changed
// on the main thread: by the views, and by goroutines through post.
type app struct {
	win   *mygo.Window
	notch *notch
	tray  *mygo.Tray // the icon in the notification area, where there is one
	art   *artCache
	pl    *player.Player
	pal   palette

	route   route
	history []route
	forward []route // the pages gone back from, the last first to go to

	connect     *connectState
	mineLoading bool
	mineAgain   bool
	mineGen     int
	mineChecked time.Time
	mineTimer   *time.Timer

	profile                             *spotify.Profile
	mine, saved                         []spotify.Playlist   // the listener's playlists: made, and saved
	followed                            []spotify.ArtistCard // the artists the listener follows
	next                                *route               // the page to open once the frame is built (goTo)
	signin                              signin               // the web player's window, which signs the listener in
	accountOpen                         bool                 // the dropdown of the account row shows
	liked                               map[string]bool      // the songs the listener likes
	toast                               string               // a message to show once
	naming                              nameDialog           // the name of a playlist asked for
	deleting                            deleteDialog         // whether to delete a playlist, asked
	radioStarting                       string               // the radio asked for its first songs, or ""
	played                              []spotify.Track      // the songs Vinyl played, the latest first
	playedFrom                          []spotify.HomeItem   // the lists, artists and podcasts Vinyl played them from
	queuedFrom                          *queued              // what the queue was played from, until it plays
	recentLists                         []spotify.Playlist   // what the listener played in Spotify's apps
	recentArtists                       []spotify.ArtistCard
	recentLoading                       bool
	foundAlbums, foundLists, foundShows []spotify.Playlist // the albums, playlists and podcasts a search found

	daily       []spotify.Track
	dailySpin   spinState // the record on the daily card
	dailyBars   barState  // the bars on its play button
	charts      []spotify.Playlist
	homeErr     string
	homeLoading bool

	homeShelves   map[string]*ui.ScrollState
	homeSections  []spotify.HomeSection            // what the home page shows: Spotify's sections
	homeShortcuts []spotify.HomeItem               // what the listener played lately, for the grid over them
	homeFacet     string                           // the chip chosen over the home page, "" for all
	facets        map[string][]spotify.HomeSection // the home's sections with a chip chosen, by chip
	sectionAll    map[string][]spotify.HomeItem    // all of a section's cards, by its URI
	homeTint      ui.Color                         // the color over the top of the home page

	playlists map[string]*spotify.Playlist
	loadingPl map[string]bool
	plErr     map[string]string

	artists    map[string]*spotify.ArtistPage
	loadingAr  map[string]bool
	arErr      map[string]string
	loadingRel map[string]bool // an artist's releases of a kind, all asked for

	tracks    map[string]*spotify.TrackPage
	loadingTr map[string]bool
	trErr     map[string]string

	cats      []spotify.Category               // the tiles of the browse page
	catPages  map[string]*spotify.CategoryPage // by URI
	users     map[string]*spotify.UserPage     // profiles, by username
	shelfOpen map[string]bool                  // a category's rows shown whole
	loading   map[string]bool                  // what fetch loads, by key
	loadErr   map[string]string                // why it did not, by key
	trackList ui.ListState
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

	// How the music video shows (mvInCover, mvTheater, mvFullScreen), and
	// the way it showed before full screen, which leaving it goes back to.
	mvMode, mvFrom int
	mvMadeFull     bool      // the window went full screen for the video, to leave with it
	mvShown        bool      // the video filled Now Playing in the last frame
	mvMoved        bool      // the pointer moved over the video since the last frame
	mvMovedAt      time.Time // when the pointer last moved over the video
	lyricsToast    string    // how far the lyrics over a video were moved, to show

	settings    settings
	settingsWin *mygo.Window
	cacheUsed   int64   // the bytes the cache of tracks takes, -1 until measured
	upd         updates // a newer version, looked for and installed

	restored     bool   // playback picked up where it was left
	accountGen   int    // signed in or out how many times: a sign-in asked before is no one's
	savedSession string // the session as last kept
	sessionDue   bool   // a save of the session is due
	homeFile     string // where what the home page shows is kept, "" to keep none
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
		if a.win.IsMinimized() {
			a.win.Restore()
		}
		a.win.Focus()
	}
}

// Run runs the app until it quits.
// appName is the name the app shows.
const appName = "MySpotify"

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
		cacheUsed:  -1,
		queueAt:    -1,
	}
	// Its data, the web player's sign-in among it, lives under this name,
	// whatever the executable is called.
	// Vinyl names where the app keeps its data (settings, the log, the
	// sign-in in WebView2), as before it was MySpotify: appName is what it
	// shows.
	mygo.App.SetName("Vinyl")
	// One runs: launched again, it shows the window of the one running,
	// hidden in the notification area perhaps. The scenes run beside it.
	if shotHook == nil && !mygo.App.RequestSingleInstanceLock() {
		return nil
	}
	if dir, err := mygo.App.Path(mygo.PathUserData); err == nil {
		log.SetOutput(io.MultiWriter(&logFile{path: filepath.Join(dir, "vinyl.log")}, os.Stderr))
		spotify.OpsFile = filepath.Join(dir, "queries.json")
		spotify.LoadOps()
	}
	log.Printf("start, pid %d", os.Getpid())
	// MyGo's updater asks GitHub with http.DefaultClient: through the
	// system's proxy, as the requests to Spotify go.
	if t, ok := http.DefaultTransport.(*http.Transport); ok {
		t.Proxy = spotify.SystemProxy
	}
	os.Setenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", strings.TrimSpace(os.Getenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS")+" --autoplay-policy=no-user-gesture-required --in-process-gpu --disable-gpu"))
	// What decoding a page of covers takes goes back to the system after,
	// rather than staying in the heap (20 MB).
	if os.Getenv("GOMEMLIMIT") == "" {
		debug.SetMemoryLimit(64 << 20)
	}
	mygo.App.WhenReady(func() {
		a.settings = loadSettings()
		a.settings.applyAppearance()
		a.npPane = a.lyricsPaneChosen()
		a.mvMode = a.mvModeChosen()
		// Before the window: on Windows it lays out its content as it is
		// made, and the view asks the player what plays.
		a.art = newArtCache(a.post)
		if dir, err := mygo.App.Path(mygo.PathCache); err == nil {
			a.art.dir = filepath.Join(dir, "covers")
			go pruneCovers(a.art.dir, coversMax)
		}
		a.pl = player.New(a.post)
		a.pl.Leads = a.mvLead
		a.pl.Cache = newPartCache(a.settings.CacheMB)
		a.pl.NewWeb = func() (*player.Web, error) {
			web, err := player.NewWeb()
			if err == nil {
				web.Cache = a.pl.Cache
			}
			return web, err
		}
		a.pl.Changed = func() {
			a.notePlayed()
			a.keepSession()
			a.reportConnect()
		}
		a.played, a.playedFrom = loadPlayed()
		a.volume = a.settings.Volume
		a.pl.SetVolume(a.volume)
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:         appName,
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
		// Music videos play in a web view of the window, which Now Playing
		// shows where the cover goes.
		a.pl.NewVideo = func() (*player.Web, error) { return player.NewVideoWeb(a.win) }
		// Full screen left with the system's own button or keys: the video
		// shows as it did before.
		a.win.OnLeaveFullScreen(func() {
			a.post(func() {
				if a.mvMadeFull {
					a.mvMadeFull = false
					if a.mvMode == mvFullScreen {
						a.mvMode = a.mvFrom
					}
				}
			})
		})
		// What the home page and the sidebar showed last time shows at
		// once, while the app signs in and loads them again. Not before the
		// window: their covers, from disk, would load as it lays out its
		// content, before a.win is set, and post would drop them. The
		// scenes show what loads.
		if dir, err := mygo.App.Path(mygo.PathUserData); err == nil && shotHook == nil {
			a.homeFile = filepath.Join(dir, "home.json")
			a.restoreHome()
			a.redraw()
		}
		a.win.OnFocus(func() { a.refreshMine(time.Now()) })
		a.scheduleMine()
		a.upd = myGoUpdates(a.postSettings)
		a.setMenu()
		a.boot()
		if shotHook == nil {
			a.watchUpdates()
		}
		// The notch player is macOS's, over its menu bar: elsewhere it would
		// hang at the top of the screen as a black box, and the app sits in
		// the notification area instead.
		if runtime.GOOS == "darwin" {
			a.notch = newNotch(a)
		} else {
			a.tray = a.newTray()
		}
		if shotHook != nil {
			shotHook(a)
		}
		// Closing the window hides it: music plays on, and the Dock icon
		// (macOS) or the one in the notification area brings it back.
		// Without either, nothing would bring it back, so closing it quits.
		a.win.OnClose(func(e *mygo.CloseEvent) {
			if a.quitting {
				return
			}
			e.PreventDefault()
			if runtime.GOOS == "darwin" || a.tray != nil {
				a.win.Hide()
			} else {
				mygo.App.Quit()
			}
		})
	})
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
		a.quitting = true
		a.stopConnect()
		if a.mineTimer != nil {
			a.mineTimer.Stop()
		}
		a.settings.Volume = a.volume
		a.settings.save()
		a.saveSession()
		if a.pl != nil {
			a.pl.CloseWeb()
		}
	})
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.OnActivate(func(bool) { a.showMain() })
	mygo.App.OnSecondInstance(func([]string, string) { a.showMain() })
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
