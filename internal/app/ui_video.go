package app

import (
	"fmt"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"vinyl/internal/lyrics"
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
	if badge(c.Key("mv"), "MV", label, on, white, ink) {
		a.pl.SetVideo(!on)
	}
}

// ink is the text of a badge filled white.
var ink = ui.Hex("#0b0b0d")

// badge is a little button of text in color, filled with it, its text
// in ink, while what it turns on is; it reports whether it was clicked.
func badge(c *ui.Context, text, label string, on bool, color, ink ui.Color) bool {
	b := ui.ButtonBase(c).Padding(2, 6, 2, 6).Radius(5).AlignItems(ui.Center).FocusRing(false).Label(label).Tooltip(label)
	edge, bg, fg := color.Alpha(0.45), ui.Transparent, color.Alpha(0.7)
	switch {
	case on:
		edge, bg, fg = color.Alpha(0.9), color.Alpha(0.9), ink
	case b.Hovered():
		edge, fg = color.Alpha(0.8), color
	}
	b.Border(1, edge).Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond})).TextColor(fg)
	b.Children(func() {
		ui.Text(c, text).FontSize(11).FontWeight(800).LetterSpacing(0.4)
	})
	return b.Clicked()
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

// mvModeChosen is the way the listener last had music videos show: full
// screen is not kept, which a video would go into by itself next time.
func (a *app) mvModeChosen() int {
	if a.settings.MVFill {
		return mvTheater
	}
	return mvInCover
}

// showMV shows the music video the way m says, its controls with it, and
// keeps whether it fills Now Playing for the next time.
func (a *app) showMV(m int) {
	a.mvMode, a.mvMoved = m, true
	if m == mvFullScreen {
		return
	}
	if fill := m == mvTheater; a.settings.MVFill != fill {
		a.settings.MVFill = fill
		a.settings.save()
	}
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
		if a.settings.MVLyrics {
			a.mvLyrics(c, shown)
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
				if lines, _ := pl.Lyrics(); timed(lines) {
					label := "显示歌词"
					if a.settings.MVLyrics {
						label = "隐藏歌词"
					}
					// The lyrics, and on either side, earlier and later.
					ui.Row(c.Key("lyrics")).Gap(2).AlignItems(ui.Center).Margin(0, 6, 0, 0).Children(func() {
						if a.settings.MVLyrics {
							if iconButton(c.Key("earlier"), iconMinus, 15, white.Alpha(0.75), "歌词提前 0.5 秒 ([)").Clicked() {
								a.nudgeLyrics(-lyricsNudge)
							}
						}
						if badge(c.Key("on"), "歌词", label, a.settings.MVLyrics, white, ink) {
							a.settings.MVLyrics = !a.settings.MVLyrics
							a.settings.save()
						}
						if a.settings.MVLyrics {
							if iconButton(c.Key("later"), iconPlus, 15, white.Alpha(0.75), "歌词延后 0.5 秒 (])").Clicked() {
								a.nudgeLyrics(lyricsNudge)
							}
						}
					})
				}
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

// timed reports whether lines are lyrics with their times, which can show
// as they are sung.
func timed(lines []lyrics.Line) bool {
	return len(lines) > 0 && !lines[0].Unsynced
}

// mvLyrics shows the line sung over the music video, its translation under
// it, as subtitles: above the controls while they show, lower once they
// go. Nothing shows before the first line, in a long break between lines
// or after the last.
func (a *app) mvLyrics(c *ui.Context, controls bool) {
	pl := a.pl
	lines, _ := pl.Lyrics()
	if !timed(lines) {
		return
	}
	pos := pl.SongPosition()
	cur := lyrics.Index(lines, pos)
	if pl.Playing() {
		if wait := lyricChange(lines, cur, pos); wait > 0 {
			c.After(wait)
		}
	}
	if cur < 0 || lines[cur].Text == "" {
		return
	}
	l := &lines[cur]
	if pos > l.End+time.Second && (cur+1 == len(lines) || lines[cur+1].At-l.End > 4*time.Second) {
		return
	}
	w, _ := c.Size()
	f := min(34, max(20, w*0.022))
	pose := ui.Pose{}
	if controls {
		pose.Y = -64 // clear of the bar at the foot
	}
	// Under the pointer, it lets it through to the video, which watches
	// its moves.
	ui.Column(c.Key("mv-lyrics")).Absolute().Left(48).Right(48).Bottom(48).AlignItems(ui.Center).Gap(6).PassThrough().
		AnimatePose(pose, ui.Animation{Duration: 200 * time.Millisecond}).Children(func() {
		caption := func(text string, size float32, color ui.Color) {
			ui.Box(c).Padding(4, 12).Radius(8).Background(black.Alpha(0.55)).PassThrough().Children(func() {
				ui.Text(c, text).FontSize(size).FontWeight(700).TextColor(color).TextAlign(ui.Center).PassThrough()
			})
		}
		caption(l.Text, f, white)
		if l.Translation != "" {
			caption(l.Translation, f*0.62, white.Alpha(0.85))
		}
	})
}

// lyricChange returns how long from pos the subtitles change: the next
// line comes, or a break begins. 0 when they no longer do.
func lyricChange(lines []lyrics.Line, cur int, pos time.Duration) time.Duration {
	next := time.Duration(0)
	at := func(t time.Duration) {
		if t > pos && (next == 0 || t-pos < next) {
			next = t - pos
		}
	}
	if cur+1 < len(lines) {
		at(lines[cur+1].At)
	}
	if cur >= 0 {
		at(lines[cur].End + time.Second)
	}
	return next
}

// lyricsNudge is how far a press moves the lyrics over a music video.
const lyricsNudge = 500 * time.Millisecond

// mvLead is how far the listener has the music video of the song id run
// ahead of it, if they set it.
func (a *app) mvLead(id string) (time.Duration, bool) {
	ms, ok := a.settings.MVLeads[id]
	return time.Duration(ms) * time.Millisecond, ok
}

// nudgeLyrics has the lyrics of the song whose music video plays come d
// later, as its video runs further ahead of it, for this song from now on.
func (a *app) nudgeLyrics(d time.Duration) {
	pl := a.pl
	tr := pl.Current()
	if tr == nil || !pl.ShowsVideo() {
		return
	}
	lead := pl.VideoLead() + d
	if a.settings.MVLeads == nil {
		a.settings.MVLeads = map[string]int64{}
	}
	a.settings.MVLeads[tr.ID] = lead.Milliseconds()
	a.settings.save()
	way := "延后"
	if d < 0 {
		way, d = "提前", -d
	}
	a.toast = fmt.Sprintf("歌词%s %.1f 秒（偏移 %+.1f 秒）", way, d.Seconds(), lead.Seconds())
	a.mvMoved = true
}

// barMVButton plays the song's music video in Now Playing from the player
// bar, or the song again: shown for a song that has one.
func (a *app) barMVButton(c *ui.Context) {
	pl := a.pl
	on := pl.VideoOn()
	label := "播放 MV"
	if on {
		label = "关闭 MV"
	}
	if badge(c.Key("mv"), "MV", label, on, a.pal.text, a.pal.bg) {
		pl.SetVideo(!on)
		if !on {
			a.nowPlaying = true
		}
	}
}

// mvTag marks a track of a list that has a music video, and plays it.
func (a *app) mvTag(c *ui.Context) ui.Element {
	p := a.pal
	b := ui.ButtonBase(c).Padding(0, 4).Radius(3).FocusRing(false).Label("播放 MV").Tooltip("播放 MV")
	color := p.muted
	if b.Hovered() {
		color = p.text
	}
	b.Border(1, color.Alpha(0.6)).Children(func() {
		ui.Text(c, "MV").FontSize(9).FontWeight(800).TextColor(color)
	})
	return b
}

// playMV plays tracks from i as a list does, the music video of the one at
// i rather than it, and shows it in Now Playing.
func (a *app) playMV(from spotify.HomeItem, tracks []spotify.Track, i int) {
	if cur := a.pl.Current(); cur != nil && cur.ID == tracks[i].ID {
		a.pl.SetVideo(true)
	} else {
		a.pl.VideoNext()
		a.playFrom(from, tracks, i)
	}
	a.nowPlaying = true
}
