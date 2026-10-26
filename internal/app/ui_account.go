package app

import (
	"context"
	"errors"
	"log"
	"runtime"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"vinyl/internal/spotify"
)

// signin is the web player's window. Hidden, it signs in as the listener
// last did and hands what it gets to package spotify, again whenever that
// runs out; shown, it is where the listener signs in.
type signin struct {
	win   *mygo.Window
	shown bool // the window shows, for the listener to sign in
	gen   int  // counts the times it showed, to stop the wait of one closed
}

// captureMu lets one capture use the window at a time.
var captureMu sync.Mutex

// boot signs in as the listener last did, if the web player still is, then
// loads the home page.
func (a *app) boot() {
	a.homeLoading = true
	a.signin.win = mygo.NewWindow(mygo.WindowOptions{
		Title: "登录 Spotify", Width: 480, Height: 720, Hidden: true,
		Page: mygo.PageOptions{PreloadScript: spotify.CaptureJS},
	})
	a.signin.win.OnClose(func(e *mygo.CloseEvent) {
		if a.quitting {
			return
		}
		// Closed, it hides, for the next time.
		e.PreventDefault()
		a.signin.win.Hide()
		a.signin.shown = false
		a.signin.gen++
	})
	spotify.Renew = a.renewIdentity
	go func() {
		id, err := a.renewIdentity()
		var prof *spotify.Profile
		if err == nil && !id.Anonymous {
			spotify.SetIdentity(id)
			if prof, err = spotify.Account(); err != nil {
				log.Printf("account: %v", err)
			}
		}
		a.post(func() { a.signedIn(prof) })
	}()
}

// renewIdentity loads the web player out of sight and waits for its token,
// anonymous if nobody is signed in. It blocks, so it runs on a goroutine.
func (a *app) renewIdentity() (spotify.Identity, error) {
	captureMu.Lock()
	defer captureMu.Unlock()
	w := a.signin.win
	if w == nil {
		return spotify.Identity{}, errors.New("no web player")
	}
	w.Page().LoadURL(spotify.WebPlayerURL)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	id, err := capture(ctx, w.Page(), false)
	// The web player has done its part: it would go on using the CPU.
	if !a.signin.shown {
		w.Page().LoadURL("about:blank")
	}
	return id, err
}

// capture waits for the web player on page to have a token, as CaptureJS
// notes it, and its client token, which comes a moment later. Unless
// anonymous will do, it waits on for the listener to sign in.
func capture(ctx context.Context, page *mygo.Page, signedIn bool) (spotify.Identity, error) {
	var since time.Time
	for {
		select {
		case <-ctx.Done():
			return spotify.Identity{}, ctx.Err()
		case <-time.After(400 * time.Millisecond):
		}
		v, err := page.EvalContext(ctx, "JSON.stringify(window.__vinyl || null)")
		s, _ := v.(string)
		if err != nil || s == "" || s == "null" {
			continue
		}
		id, ok := spotify.ParseIdentity(s)
		if !ok || id.Anonymous && signedIn {
			continue
		}
		if since.IsZero() {
			since = time.Now()
		}
		if id.ClientToken != "" || time.Since(since) > 8*time.Second {
			return id, nil
		}
	}
}

// openLogin shows the web player's sign-in page, and signs the app in once
// the listener has.
func (a *app) openLogin() {
	s := &a.signin
	if s.win == nil {
		return
	}
	s.gen++
	gen := s.gen
	s.shown = true
	s.win.Show()
	s.win.Focus()
	go func() {
		captureMu.Lock()
		defer captureMu.Unlock()
		s.win.Page().LoadURL(spotify.LoginURL)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		// Closing the window stops the wait.
		go func() {
			for ctx.Err() == nil {
				time.Sleep(time.Second)
				done := make(chan bool, 1)
				a.post(func() { done <- gen != s.gen })
				select {
				case closed := <-done:
					if closed {
						cancel()
					}
				case <-ctx.Done():
				}
			}
		}()
		id, err := capture(ctx, s.win.Page(), true)
		if err != nil {
			return
		}
		spotify.SetIdentity(id)
		prof, err := spotify.Account()
		a.post(func() {
			if gen == s.gen {
				s.shown = false
				s.win.Hide()
				s.win.Page().LoadURL("about:blank")
			}
			if err != nil {
				a.toast = "登录失败，请稍后再试"
				return
			}
			a.signedIn(prof)
		})
	}()
}

// signedIn shows what is personal to prof, or to nobody.
func (a *app) signedIn(prof *spotify.Profile) {
	a.profile = prof
	a.mine, a.saved, a.recPlaylists, a.liked = nil, nil, nil, nil
	// Who listens changed: load the lists again.
	a.playlists = map[string]*spotify.Playlist{}
	a.plErr = map[string]string{}
	if prof == nil {
		// Spotify plays nothing to nobody.
		a.homeLoading, a.homeErr = false, ""
		a.daily, a.featured, a.charts = nil, nil, nil
		return
	}
	// Playback picks up where it was left, once there is someone to play to.
	a.restoreSession()
	if a.route.page == pagePlaylist {
		a.loadPlaylist(a.route.playlist, false)
	}
	if a.searched != "" {
		a.runSearch(a.searched)
	}
	a.loadHome()
	a.loadLiked(prof.UserID)
	a.loadMine(prof.UserID)
}

func (a *app) signOut() {
	a.signin.gen++
	spotify.SetIdentity(spotify.Identity{})
	if a.pl.Playing() {
		a.pl.TogglePlay()
	}
	a.signedIn(nil)
	go func() {
		if err := mygo.Cookies.Clear(); err != nil {
			log.Printf("sign out: %v", err)
		}
	}()
}

// signedOutPage stands in for the pages while nobody is signed in.
func (a *app) signedOutPage(c *ui.Context) {
	p := a.pal
	top, bottom := a.insets(c)
	ui.Column(c).Grow(1).Padding(top, 0, bottom, 0).Center().Gap(10).Children(func() {
		ui.Text(c, "登录 Spotify 开始听歌").FontSize(20).FontWeight(700)
		ui.Text(c, "免费账号也能完整播放").FontSize(13).TextColor(p.muted).Margin(0, 0, 8, 0)
		if pillButton(c.Key("login"), iconUser, "登录 Spotify", p.accent, p.onAccent).Clicked() {
			a.openLogin()
		}
	})
}

func (a *app) profileName() string {
	if a.profile == nil {
		return ""
	}
	return a.profile.Nickname
}

// accountRow sits at the foot of the sidebar: who is signed in, with a
// dropdown to open the settings or sign out, or a way to sign in.
func (a *app) accountRow(c *ui.Context) {
	p := a.pal
	b := ui.ButtonBase(c.Key("account")).Height(40).PaddingX(8).Gap(10).Radius(10).AlignItems(ui.Center).Justify(ui.Start).FocusRing(false)
	if b.Hovered() || a.accountOpen && a.profile != nil {
		b.Background(p.hover)
	}
	if a.profile == nil {
		a.accountOpen = false
		b.Label("登录").Children(func() {
			ui.Box(c).Size(26, 26).Radius(13).Background(p.press).Center().Children(func() {
				ui.Icon(c, iconUser).Size(14, 14).TextColor(p.muted)
			})
			ui.Text(c, "登录 Spotify").FontSize(13).FontWeight(500).TextColor(p.muted)
		})
		if b.Clicked() {
			a.openLogin()
		}
		return
	}
	prof := a.profile
	b.Label(prof.Nickname).Expanded(a.accountOpen).Children(func() {
		a.avatar(c, prof.AvatarURL, 26, p.press)
		ui.Text(c, prof.Nickname).FontSize(13).FontWeight(600).SingleLine().Grow(1).Shrink(1).MinWidth(0)
		ui.Icon(c, iconUpDown).Size(13, 13).TextColor(p.faint)
	})
	if b.Clicked() {
		a.accountOpen = !a.accountOpen
	}
	a.accountMenu(c, b)
}

// accountMenu drops up from the account row, as wide as it, in the colors
// of the volume popover: the settings, then signing out.
func (a *app) accountMenu(c *ui.Context, row ui.Element) {
	pop := popoverColors(a.pal.dark)
	ui.PopoverBase(c.Key("account-menu"), row, &a.accountOpen, func(panel ui.Element) {
		panel.AttachTo(row, ui.AnchorTopStart, ui.AnchorBottomStart).Margin(0, 0, 6, 0).Padding(5).Gap(1).
			Radius(12).Background(pop.fill).Border(0.5, pop.edge).Shadow(0, 8, 24, -4, ui.RGBA(0, 0, 0, 0.2)).
			Role(ui.RoleMenu).FocusGroup(ui.Vertical).Label("账户").TransformOrigin(0.5, 1).
			Transition(ui.ElementTransition{Animation: &ui.Snappy, Enter: &ui.Motion{Scale: 0.96, Y: 4}, Exit: &ui.Motion{Scale: 0.98}})
		if w := row.Bounds().W; w > 0 {
			panel.Width(w)
		}
		shortcut := "Ctrl+,"
		if runtime.GOOS == "darwin" {
			shortcut = "⌘,"
		}
		if a.menuItem(c, iconSettings, "设置…", shortcut).AutoFocus().Clicked() {
			a.accountOpen = false
			a.openSettings()
		}
		ui.Box(c).Height(0.5).Margin(4, 8).Background(pop.edge)
		if a.menuItem(c, iconSignOut, "退出登录", "").Clicked() {
			a.accountOpen = false
			a.signOut()
		}
	})
}

// menuItem is an item of a dropdown: an icon, its label, and the shortcut
// that does the same, if any.
func (a *app) menuItem(c *ui.Context, icon *ui.SVG, label, shortcut string) ui.Element {
	p := a.pal
	b := ui.ButtonBase(c.Key(label)).Role(ui.RoleMenuItem).Label(label).Height(30).PaddingX(8).Gap(9).Radius(7).
		AlignItems(ui.Center).Justify(ui.Start).FocusRing(false)
	if b.Hovered() || b.FocusVisible() {
		b.Background(p.press)
	}
	b.Children(func() {
		ui.Icon(c, icon).Size(15, 15).TextColor(p.muted)
		ui.Text(c, label).FontSize(13).Grow(1)
		if shortcut != "" {
			ui.Text(c, shortcut).FontSize(12).TextColor(p.faint)
		}
	})
	return b
}

// avatar is a round picture of someone, or a circle of empty until it loads.
func (a *app) avatar(c *ui.Context, url string, size float32, empty ui.Color) {
	bmp := bitmap(a.art.get(spotify.Sized(url, int(size*3)), false))
	box := ui.Box(c).Size(size, size).Radius(size / 2).Shrink(0)
	in := artIn(box, bmp)
	if in < 1 {
		box.Background(empty)
	}
	box.Children(func() { artImage(c, bmp, size/2, in) })
}

// loadMine loads the listener's playlists, for the sidebar.
func (a *app) loadMine(uid string) {
	go func() {
		made, saved, err := spotify.UserPlaylists(uid)
		a.post(func() {
			if err == nil && a.profile != nil && a.profile.UserID == uid {
				a.mine, a.saved = made, saved
			}
		})
	}()
}

// myPlaylists lists the listener's playlists in the sidebar.
func (a *app) myPlaylists(c *ui.Context, title string, lists []spotify.Playlist) {
	if len(lists) == 0 {
		return
	}
	p := a.pal
	ui.Row(c.Key(title)).AlignItems(ui.Center).Margin(18, 4, 6, 10).Children(func() {
		ui.Text(c, title).FontSize(11.5).FontWeight(600).TextColor(p.faint).Grow(1)
	})
	for i := range lists {
		pl := &lists[i]
		selected := a.route.page == pagePlaylist && a.route.playlist == pl.ID && !a.nowPlaying
		b := ui.ButtonBase(c.Key(pl.ID)).Height(34).PaddingX(8).Gap(10).Radius(8).AlignItems(ui.Center).Justify(ui.Start).FocusRing(false).Label(pl.Name)
		switch {
		case selected:
			b.Background(p.press)
		case b.Hovered():
			b.Background(p.hover)
		}
		b.Children(func() {
			if pl.Liked() {
				ui.Box(c).Size(22, 22).Radius(5).Background(a.pal.accent).Center().Children(func() {
					ui.Icon(c, iconHeartFill).Size(12, 12).TextColor(a.pal.onAccent)
				})
			} else {
				a.cover(c, pl.Cover(), 80, 22, 5)
			}
			ui.Text(c, pl.Name).FontSize(13).SingleLine().Shrink(1).MinWidth(0)
		})
		if b.Clicked() {
			a.open(route{page: pagePlaylist, playlist: pl.ID})
		}
	}
}
