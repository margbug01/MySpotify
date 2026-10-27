package app

import (
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"vinyl/internal/spotify"
)

// The ways a music video shows in Now Playing: where the cover goes,
// beside the lyrics; filling the window, the controls over it; and filling
// the screen.
const (
	mvInCover = iota
	mvTheater
	mvFullScreen
)

// mvIdle is how long the pointer rests over a video filling Now Playing
// before the controls, and the pointer, go.
const mvIdle = 3 * time.Second

var black = ui.RGB(0, 0, 0)

// mvButton plays the song's music video instead of it, or the song again:
// shown for a song that has one. Once on, the songs after it that have one
// play theirs too, as in Spotify.
func (a *app) mvButton(c *ui.Context) {
	on := a.pl.VideoOn()
	label := "播放 MV"
	if on {
		label = "关闭 MV"
	}
	b := ui.ButtonBase(c.Key("mv")).Padding(2, 6, 2, 6).Radius(5).AlignItems(ui.Center).FocusRing(false).Label(label).Tooltip(label)
	edge, bg, fg := white.Alpha(0.45), ui.Transparent, white.Alpha(0.7)
	switch {
	case on:
		edge, bg, fg = white.Alpha(0.9), white.Alpha(0.9), ui.Hex("#0b0b0d")
	case b.Hovered():
		edge, fg = white.Alpha(0.8), white
	}
	b.Border(1, edge).Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond})).TextColor(fg)
	b.Children(func() {
		ui.Text(c, "MV").FontSize(11).FontWeight(800).LetterSpacing(0.4)
	})
	if b.Clicked() {
		a.pl.SetVideo(!on)
	}
}

// mvPage is the web view music videos play in, nil before one has played.
func (a *app) mvPage() *mygo.WebView {
	if a.pl.Video == nil {
		return nil
	}
	return a.pl.Video.View()
}

// theater reports whether the music video fills Now Playing: it plays, or
// the next one loads, and the listener chose a way that fills it.
func (a *app) theater() bool {
	pl := a.pl
	return a.mvMode != mvInCover && a.mvPage() != nil && (pl.ShowsVideo() || pl.VideoOn() && pl.Loading())
}

// showMV shows the music video the way m says, its controls with it.
func (a *app) showMV(m int) {
	a.mvMode, a.mvMoved = m, true
}

// toggleMVFullScreen has the video fill the screen, or show again as it
// did before it did.
func (a *app) toggleMVFullScreen() {
	if a.mvMode == mvFullScreen {
		a.showMV(a.mvFrom)
		return
	}
	a.mvFrom = a.mvMode
	a.showMV(mvFullScreen)
}

// npEscape steps back: from full screen to the way the video showed
// before, from the video filling Now Playing to its place by the lyrics,
// and from Now Playing to the page under it.
func (a *app) npEscape() {
	switch {
	case !a.theater():
		a.nowPlaying = false
	case a.mvMode == mvFullScreen:
		a.showMV(a.mvFrom)
	default:
		a.showMV(mvInCover)
	}
}

// syncFullScreen has the window fill the screen while the video does, and
// leave it once the video no longer does: closed, or a song without one
// playing. A window the listener put in full screen stays there.
func (a *app) syncFullScreen(npOpen bool) {
	want := npOpen && a.mvMode == mvFullScreen && a.theater()
	if !want && a.mvMode == mvFullScreen {
		a.mvMode = a.mvFrom
	}
	if a.win == nil || want == a.mvMadeFull {
		return
	}
	if !want {
		a.mvMadeFull = false
		a.win.SetFullScreen(false)
		return
	}
	if !a.win.IsFullScreen() {
		a.mvMadeFull = true
		a.win.SetFullScreen(true)
	}
}

// npVideo shows the music video where the cover goes: as wide as the
// column, as tall as the picture. The ways to fill the window or the
// screen show over it as the pointer does; double-clicked, it fills the
// screen.
func (a *app) npVideo(c *ui.Context, width, size float32) {
	h := width * 9 / 16
	ui.Column(c.Key("mv-place")).Size(width, size).Justify(ui.Center).AlignSelf(ui.Center).Children(func() {
		frame := ui.Box(c.Key("mv-frame")).Size(width, h).Radius(10).Background(black)
		frame.Children(func() {
			ui.WebView(c, a.mvPage()).Size(width, h).Radius(10).Label("MV")
			// Over the video, which would take the pointer itself.
			over := ui.Box(c.Key("mv-over")).Absolute().Top(0).Left(0).Right(0).Bottom(0).Radius(10).Label("MV 画面")
			if over.DoubleClicked() {
				a.toggleMVFullScreen()
			}
			ways := ui.Row(c.Key("mv-ways")).Absolute().Right(8).Bottom(8).Gap(2).Padding(3).Radius(8).Background(black.Alpha(0.45))
			reveal(ways, frame.Hovered()).Children(func() {
				if iconButton(c.Key("theater"), iconExpand, 17, white.Alpha(0.9), "铺满窗口").Clicked() {
					a.showMV(mvTheater)
				}
				if iconButton(c.Key("full"), iconFullScreen, 17, white.Alpha(0.9), "全屏").Clicked() {
					a.toggleMVFullScreen()
				}
			})
		})
	})
}

// npTheater fills Now Playing with the music video, the controls over it
// while the pointer moves or the video is paused. Still for a few seconds
// as it plays, they go, and so does the pointer. Double-clicked, the video
// fills the screen, or leaves it.
func (a *app) npTheater(c *ui.Context, tr *spotify.Track, art *artwork, accent ui.Color) {
	pl := a.pl
	now := c.Now()
	if !a.mvShown {
		// Come to fill it: the controls show at first.
		a.mvShown, a.mvMoved = true, true
	}
	if a.mvMoved {
		a.mvMoved, a.mvMovedAt = false, now
	}
	root := ui.Box(c.Key("theater")).Absolute().Top(0).Left(0).Right(0).Bottom(0).Background(black).TextColor(white)
	root.Children(func() {
		ui.Column(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Children(func() {
			ui.WebView(c, a.mvPage()).Grow(1).Label("MV")
		})
		// Over the video, which would take the pointer itself: here its
		// moves show the controls.
		over := ui.Box(c.Key("mv-over")).Absolute().Top(0).Left(0).Right(0).Bottom(0).Label("MV 画面")
		over.HandleInput(func(ev ui.InputEvent) bool {
			if ev.Kind != ui.InputPointerMove {
				return false
			}
			a.mvMoved = true
			return true
		})
		if over.DoubleClicked() {
			a.toggleMVFullScreen()
		}
		head := ui.Column(c.Key("mv-head")).Absolute().Top(0).Left(0).Right(0).Padding(0, 0, 28, 0).
			LinearGradient(ui.LinearGradient{Angle: 180, Stops: []ui.GradientStop{
				{Color: black.Alpha(0.6), At: 0},
				{Color: black.Alpha(0), At: 1},
			}})
		foot := ui.Column(c.Key("mv-foot")).Absolute().Left(0).Right(0).Bottom(0).Padding(56, 0, 0, 0).
			LinearGradient(ui.LinearGradient{Angle: 180, Stops: []ui.GradientStop{
				{Color: black.Alpha(0), At: 0},
				{Color: black.Alpha(0.7), At: 1},
			}})
		idle := now.Sub(a.mvMovedAt)
		held := head.Hovered() || foot.Hovered()
		shown := !pl.Playing() || held || idle < mvIdle
		if shown && pl.Playing() && !held {
			// Built again as they go.
			c.After(mvIdle - idle)
		}
		if !shown {
			// Gone, they let the pointer through to the video, whose moves
			// bring them back.
			over.Cursor(ui.CursorNone)
			head.PassThrough()
			foot.PassThrough()
		}
		reveal(head, shown).Children(func() {
			// The top strip drags the window; the ways to show the video sit
			// below it, clear of the window's buttons (Windows).
			ui.Row(c).Height(topBar(c)).Justify(ui.Center).AlignItems(ui.Center).DragWindow().Children(func() {
				if iconButton(c.Key("close"), iconDown, 22, white.Alpha(0.8), "关闭").Clicked() {
					a.nowPlaying = false
				}
			})
			ui.Row(c).Justify(ui.End).AlignItems(ui.Center).Gap(10).Padding(0, 28, 0, 28).Children(func() {
				a.mvButton(c)
				if iconButton(c.Key("shrink"), iconShrink, 20, white.Alpha(0.85), "还原").Clicked() {
					a.showMV(mvInCover)
				}
				icon, label := iconFullScreen, "全屏"
				if a.mvMode == mvFullScreen {
					icon, label = iconExitFullScreen, "退出全屏"
				}
				if iconButton(c.Key("full"), icon, 20, white.Alpha(0.85), label).Clicked() {
					a.toggleMVFullScreen()
				}
			})
		})
		reveal(foot, shown).Children(func() { a.stageBar(c, tr, art, accent) })
	})
}
