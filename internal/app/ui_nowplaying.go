package app

import (
	"math"
	"time"
	"unicode"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"vinyl/internal/lyrics"
	"vinyl/internal/spotify"
)

var white = ui.RGB(255, 255, 255)

var npSpring = ui.Animation{Spring: ui.SpringOptions{Duration: 450 * time.Millisecond, Bounce: 0.08}}

// npSettle is how long Now Playing takes to come or go, its spring at rest.
const npSettle = 600 * time.Millisecond

// npState tracks Now Playing opening and closing, and reports whether it is
// open and whether it has come to rest, open or closed. Call it once a
// build, before the window's content.
func (a *app) npState(c *ui.Context) (open, settled bool) {
	open = a.nowPlaying && a.pl.Current() != nil
	now := c.Now()
	if open != a.npOpen {
		a.npOpen, a.npChangedAt = open, now
		// A build once it is at rest: to stop painting what it covers, or
		// itself once gone.
		c.After(npSettle)
		// The window buttons light over its dark backdrop, and as the
		// theme over the pages (Windows).
		if a.win != nil {
			buttons := mygo.ThemeSystem
			if open {
				buttons = mygo.ThemeDark
			}
			a.win.SetWindowButtonsTheme(buttons)
		}
	}
	return open, now.Sub(a.npChangedAt) >= npSettle
}

// nowPlayingLayer holds Now Playing over the whole window, rising into
// view as it opens and sinking away as it closes. The layer moves itself,
// staying on top all the way, rather than leaving as an exit transition
// would, under the rest of the window: that would show it going over the
// sidebar's material while the page above it hid it at once. Its pose
// moves it in frames that only paint, building nothing.
func (a *app) nowPlayingLayer(c *ui.Context, open, settled bool) {
	layer := ui.Box(c.Key("now-playing-layer")).Absolute().Top(0).Left(0).Right(0).Bottom(0)
	pose := ui.Pose{Y: 60, Fade: 1}
	if open {
		pose = ui.Pose{}
	}
	layer.AnimatePose(pose, npSpring)
	if !open && settled || a.pl.Current() == nil {
		// Closed: nothing to paint or click.
		layer.Invisible()
		a.queueAt = -1
		a.mvShown = false
		return
	}
	if !open {
		// On its way out, the window under it takes the clicks.
		layer.PassThrough()
	}
	layer.Children(func() { a.nowPlayingView(c) })
}

// nowPlayingView fills the window with what plays: its cover over a slow
// drift of its own colors, the controls, and lyrics that follow the song.
func (a *app) nowPlayingView(c *ui.Context) {
	pl := a.pl
	tr := pl.Current()
	w, h := c.Size()
	art := a.art.get(spotify.Sized(tr.Album.PicURL, 600), true)
	// The colors stay the last song's while its cover loads, rather than
	// going gray between the two.
	shown := art
	if art == nil && tr.Album.PicURL != "" {
		shown = a.npArt
	}
	a.npArt = shown
	tint := ui.RGB(120, 120, 130)
	if shown != nil && !shown.failed {
		tint = shown.tint
	}
	accent := tint.Mix(white, 0.45)

	root := ui.Box(c.Key("now-playing")).Absolute().Top(0).Left(0).Right(0).Bottom(0).
		Background(ui.Hex("#0b0b0d")).TextColor(white)
	// The pointer is over the view whenever it is in the window.
	pointerIn := root.Hovered()
	root.Children(func() {
		if a.theater() {
			a.npTheater(c, tr, art, accent)
			return
		}
		a.mvShown = false
		a.backdrop(c, shown, tint)
		// Over the backdrop, which is positioned too: positioned elements
		// paint over those in the flow.
		ui.Column(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Children(func() { a.nowPlayingBody(c, tr, art, accent, w, h, pointerIn) })
	})
}

func (a *app) nowPlayingBody(c *ui.Context, tr *spotify.Track, art *artwork, accent ui.Color, w, h float32, pointerIn bool) {
	pl := a.pl
	if a.npPane != paneQueue {
		a.queueAt = -1
	}
	{
		// The top strip drags the window; the chevron closes the view,
		// showing only while the pointer is in the window, as the pane tabs do.
		ui.Row(c).Height(topBar(c)).Justify(ui.Center).AlignItems(ui.Center).DragWindow().Children(func() {
			reveal(ui.Box(c.Key("close-reveal")), pointerIn).Children(func() {
				if iconButton(c.Key("close"), iconDown, 22, white.Alpha(0.7), "关闭").Clicked() {
					a.nowPlaying = false
				}
			})
		})
		if a.npPane == paneStage {
			// The stage takes the whole view: the lyrics are the show, the
			// controls a slim bar at the foot.
			reveal(ui.Row(c.Key("tabs")).Absolute().Top(topBar(c)).Right(48).Gap(6), pointerIn).Children(func() { a.paneTabs(c) })
			a.stage(c, tr, accent)
			a.stageBar(c, tr, art, accent)
			return
		}

		side := float32(math.Min(float64(w)*0.36, 440))
		// The cover leaves room above it for the pane's tabs.
		coverSize := min(side, h-404)
		ui.Row(c).Grow(1).Padding(0, 72, 44, 72).Gap(72).AlignItems(ui.Stretch).Children(func() {
			var coverTop float32
			ui.Column(c).Width(side).Justify(ui.Center).Gap(0).Children(func() {
				coverTop = ui.Box(c.Key("cover-top")).Height(0).LayoutBounds().Y
				if pl.ShowsVideo() && a.mvPage() != nil {
					a.npVideo(c, side, coverSize)
				} else {
					a.npCover(c, tr, art, coverSize)
				}

				ui.Row(c).Gap(8).AlignItems(ui.Center).Margin(36, 0, 18, 0).Children(func() {
					ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(4).Children(func() {
						if r := pl.Radio(); r != "" {
							a.radioChip(c, r)
						}
						ui.Text(c, tr.Name).FontSize(22).FontWeight(700).LetterSpacing(-0.3).SingleLine()
						ui.Row(c).AlignItems(ui.Center).Children(func() {
							a.artistLinks(c.Key("by"), tr.Artists, 15, white.Alpha(0.6), white)
							if tr.Album.Name != "" {
								ui.Text(c, " — ").FontSize(15).TextColor(white.Alpha(0.6)).Shrink(0)
								if link(c.Key("album"), tr.Album.Name, 15, white.Alpha(0.6), white).Clicked() && tr.Album.ID != "" {
									a.goTo(route{page: pagePlaylist, playlist: tr.Album.ID})
								}
							}
						})
					})
					if pl.HasVideo() {
						a.mvButton(c)
					}
					a.likeButton(c.Key("like"), tr, 22, white.Alpha(0.7))
					a.songMenu(c.Key("more"), tr, 22, white.Alpha(0.7)).Margin(0, -8, 0, 0)
				})
				a.scrubber(c.Key("np-scrub"), &a.npScrub, white.Alpha(0.18), white.Alpha(0.9), 16, accent.Mix(white, 0.35))
				ui.Row(c).Children(func() {
					ui.Text(c, clock(pl.Position())).FontSize(11).FontFeatures("tnum").TextColor(white.Alpha(0.5))
					ui.Spacer(c)
					status := "-" + clock(pl.Duration()-pl.Position())
					if pl.Loading() {
						status = "正在缓冲…"
					}
					ui.Text(c, status).FontSize(11).FontFeatures("tnum").TextColor(white.Alpha(0.5))
				})
				ui.Box(c).Margin(14, 0, 18, 0).Children(func() {
					a.transport(c, white, white.Alpha(0.45), accent, 28)
				})
				ui.Row(c).Justify(ui.Center).Children(func() {
					a.volumeSlider(c, white.Alpha(0.7), white.Alpha(0.18), 180)
				})
			})

			pane := ui.Column(c.Key("pane")).Grow(1).Basis(0).MinWidth(0)
			pane.Children(func() {
				// The pane starts level with the cover beside it. Its tabs come
				// and go with the pointer, so they sit above it, out of the flow,
				// never over it.
				tabs := ui.Row(c.Key("tabs")).Absolute().Left(0).Gap(6)
				b, tb := pane.LayoutBounds(), tabs.LayoutBounds()
				if b.H > 0 && tb.H > 0 && coverTop > 0 {
					if top := max(coverTop-b.Y, tb.H+8); top != a.npPaneTop {
						a.npPaneTop = top
						c.AnimationFrame()
					}
				} else {
					c.AnimationFrame() // not laid out yet: the next frame places it
				}
				reveal(tabs.Top(a.npPaneTop-8-tb.H), pointerIn).Children(func() { a.paneTabs(c) })
				ui.Box(c).Height(a.npPaneTop)
				switch a.npPane {
				case paneLyrics:
					a.lyricsPane(c, tr, h)
				default:
					a.queuePane(c)
				}
			})
		})
	}
}

// reveal fades e in while the pointer is in the window, and out when it
// leaves.
func reveal(e ui.Element, shown bool) ui.Element {
	pose := ui.Pose{Fade: 1}
	if shown {
		pose = ui.Pose{}
	}
	return e.AnimatePose(pose, ui.Animation{Duration: 200 * time.Millisecond})
}

// lyricsPaneChosen is the pane of lyrics the listener chose last.
func (a *app) lyricsPaneChosen() int {
	if a.settings.LyricsStage {
		return paneStage
	}
	return paneLyrics
}

// The panes of Now Playing.
const (
	paneLyrics = iota
	paneQueue
	paneStage
)

func (a *app) paneTabs(c *ui.Context) {
	a.paneTab(c, paneLyrics, lyricsName(a.pl.Current()))
	a.paneTab(c, paneStage, "律动")
	a.paneTab(c, paneQueue, "待播清单")
}

// stageBar is the slim bar along the foot of the stage: what plays, where
// it is, and the controls.
func (a *app) stageBar(c *ui.Context, tr *spotify.Track, art *artwork, accent ui.Color) {
	pl := a.pl
	ui.Column(c).Padding(0, 48, 28, 48).Gap(10).Children(func() {
		a.scrubber(c.Key("stage-scrub"), &a.npScrub, white.Alpha(0.18), white.Alpha(0.9), 14, accent.Mix(white, 0.35))
		ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Grow(1).Basis(0).MinWidth(0).Gap(12).AlignItems(ui.Center).Children(func() {
				if bmp := bitmap(art); bmp != nil {
					cov := ui.Box(c.Key(tr.ID)).Size(44, 44)
					in := artIn(cov, bmp)
					cov.Children(func() { artImage(c, bmp, 8, in) })
				}
				ui.Column(c).Shrink(1).MinWidth(0).Gap(2).Children(func() {
					ui.Text(c, tr.Name).FontSize(14).FontWeight(700).SingleLine()
					a.artistLinks(c.Key("by"), tr.Artists, 12, white.Alpha(0.55), white)
				})
				a.likeButton(c.Key("like"), tr, 17, white.Alpha(0.6))
				a.songMenu(c.Key("more"), tr, 17, white.Alpha(0.6)).Margin(0, 0, 0, -10)
			})
			a.transport(c, white, white.Alpha(0.45), white, 22)
			ui.Row(c).Grow(1).Basis(0).MinWidth(0).Justify(ui.End).Children(func() {
				ui.Text(c, clock(pl.Position())+" / "+clock(pl.Duration())).FontSize(11.5).FontFeatures("tnum").TextColor(white.Alpha(0.5))
			})
		})
	})
}

func (a *app) paneTab(c *ui.Context, i int, label string) {
	b := ui.ButtonBase(c.Key(i)).Padding(6, 14).Radius(999).FocusRing(false).Label(label)
	bg, fg := ui.Transparent, white.Alpha(0.55)
	if a.npPane == i {
		bg, fg = white.Alpha(0.14), white
	} else if b.Hovered() {
		bg = white.Alpha(0.06)
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
	b.Children(func() {
		ui.Text(c, label).FontSize(13).FontWeight(600).TextColor(fg)
	})
	if b.Clicked() {
		a.npPane = i
		if (i == paneLyrics || i == paneStage) && a.settings.LyricsStage != (i == paneStage) {
			a.settings.LyricsStage = i == paneStage
			a.settings.save()
		}
	}
}

func (a *app) lyricsPane(c *ui.Context, tr *spotify.Track, h float32) {
	pl := a.pl
	lines, loaded := pl.Lyrics()
	if len(lines) == 0 {
		col := ui.Column(c).Grow(1)
		if loaded {
			col.Center()
		}
		col.Children(func() {
			if !loaded {
				lyricsSkeleton(c, npShimmer(c))
				return
			}
			ui.Text(c, "暂无"+lyricsName(tr)).FontSize(24).FontWeight(700).TextColor(white.Alpha(0.4))
		})
		return
	}
	if lines[0].Unsynced {
		if a.lyricTrack != tr.ID {
			a.lyricTrack, a.lyricAt = tr.ID, -2
			a.lyricScroll, a.lyricGlide = ui.ScrollState{}, lyricGlide{}
		}
		ui.Scroll(c.Key("plain-lyrics")).Grow(1).TrackScroll(&a.lyricScroll).Padding(0, 8, 24, 0).Children(func() {
			ui.Column(c).Gap(16).Padding(10, 12).Children(func() {
				ui.Text(c, "LRCLIB · 无时间轴歌词").FontSize(13).TextColor(white.Alpha(0.5))
				for _, line := range lines {
					ui.Text(c, line.Text).FontSize(30).FontWeight(800).LineHeight(1.25).TextColor(white.Alpha(0.85))
				}
			})
		})
		return
	}
	pos := pl.SongPosition()
	cur := lyrics.Index(lines, pos)
	// Redraw as the next line begins.
	if pl.Playing() && cur+1 < len(lines) {
		c.After(lines[cur+1].At - pos + 20*time.Millisecond)
	}
	if a.lyricTrack != tr.ID {
		a.lyricTrack, a.lyricAt = tr.ID, -2
		a.lyricScroll, a.lyricGlide = ui.ScrollState{}, lyricGlide{}
	}
	lag := a.lyricGlide.step(c, &a.lyricScroll)

	scroll := ui.Scroll(c.Key("lyrics")).Grow(1).TrackScroll(&a.lyricScroll).Padding(0, 8, h*0.5, 0)
	// Long lyrics, as an episode's transcript, thousands of lines, are built
	// only about what shows, and the lines above and below make room as
	// tall as they are (lyricRows).
	lo, hi := 0, len(lines)
	var rows *lyricRows
	box := scroll.Bounds()
	if len(lines) > lyricsAll {
		rows = &a.lyricRows
		rows.fit(tr.ID, lines, box.W)
		view := box.H
		if view <= 0 {
			view = h
		}
		shown := a.lyricScroll.Y + lag
		lo, hi = rows.span(shown-view, shown+2*view)
	}
	var active ui.Element
	// The lines, held back by how far the glide to the line sung has still
	// to go (lyricGlide).
	scroll.Children(func() {
		ui.Column(c).Gap(lyricGap).Translate(0, -lag).Children(func() {
			if lo > 0 {
				ui.Box(c.Key("above")).Height(rows.top(lo) - lyricGap)
			}
			for i := lo; i < hi; i++ {
				l := lines[i]
				line := ui.ButtonBase(c.Key(i)).Column().AlignItems(ui.Start).Padding(10, 12).Radius(12).Gap(6).FocusRing(false).Label(l.Text)
				on := i == cur
				alpha := float32(0.28)
				if on {
					alpha = 1
				} else if line.Hovered() {
					alpha = 0.55
				}
				if line.Hovered() {
					line.Background(white.Alpha(0.06))
				}
				scale := float32(0.94)
				if on {
					scale = 1
				}
				line.TransformOrigin(0, 0.5).AnimatePose(ui.Pose{Scale: scale}, ui.Animation{Spring: ui.SpringOptions{Duration: 600 * time.Millisecond, Bounce: 0.1}})
				line.Children(func() {
					// The line fades as a pose, in frames that only paint, rather
					// than building the view at the display's rate as it does.
					text := ui.Column(c).AlignItems(ui.Start).Gap(6)
					text.AnimatePose(ui.Pose{Fade: 1 - alpha}, ui.Animation{Duration: 380 * time.Millisecond})
					text.Children(func() {
						if on && len(l.Words) > 0 {
							// The line sung lights up word by word as it is sung.
							ui.RichText(c.Key("words")).FontSize(30).FontWeight(800).LineHeight(1.25).LetterSpacing(-0.4).Children(func() {
								for wi, wd := range l.Words {
									lit := clamp01(float32((pos - wd.Start).Seconds() / max(0.05, (wd.End-wd.Start).Seconds())))
									ui.Text(c.Key(wi), wd.Text).TextColor(white.Alpha(0.38 + 0.62*lit))
								}
							})
							if pl.Playing() {
								// Words brighten smoothly enough at 25 frames a second.
								c.After(40 * time.Millisecond)
							}
						} else {
							ui.Text(c, l.Text).FontSize(30).FontWeight(800).LineHeight(1.25).LetterSpacing(-0.4).TextColor(white)
						}
						if l.Translation != "" {
							ui.Text(c, l.Translation).FontSize(17).FontWeight(600).TextColor(white.Alpha(0.7))
						}
					})
				})
				if line.Clicked() {
					pl.Seek(l.At)
				}
				if on {
					active = line
				}
				if rows != nil {
					rows.measure(i, line)
				}
			}
			if hi < len(lines) {
				ui.Box(c.Key("below")).Height(rows.top(len(lines)) - rows.top(hi) - lyricGap)
			}
		})
	})
	// Bring the line sung to a third of the way down, once it changed.
	if cur != a.lyricAt {
		target := float32(0)
		if rows != nil {
			// Where the heights known put it: it may not be built yet.
			if box.H <= 0 {
				return // not laid out yet: the next frame scrolls
			}
			if cur >= 0 {
				target = rows.top(cur) - box.H*0.3
			}
		} else if active.Valid() {
			// Where the glide is going, without where it still is.
			if lb := active.LayoutBounds(); lb.H > 0 && box.H > 0 {
				target = lb.Y - box.Y + a.lyricScroll.Y - box.H*0.3
			} else {
				return // not laid out yet: the next frame scrolls
			}
		}
		a.lyricAt = cur
		a.lyricGlide.to(c, &a.lyricScroll, min(max(0, target), a.lyricScroll.MaxY))
	}
}

// lyricGap is the room between two lines of the lyrics.
const lyricGap = 6

// lyricsAll is how many lines of lyrics are built all at once, as a
// song's: more, as an episode's transcript, and only those about what
// shows are, a frame of thousands taking a tenth of a second.
const lyricsAll = 150

// lyricRows is what long lyrics know of how tall their lines are: as laid
// out, those built so far, the others as their text would wrap. A line
// built once and left keeps the room it took, so that what shows does not
// move as lines are built and let go about it.
type lyricRows struct {
	track string
	width float32
	lines []lyrics.Line
	h     []float32 // as laid out; 0 for a line never built
	guess []float32 // as its text would wrap; 0 until asked
}

// fit makes r track's lines', as wide as width: laid out again at another
// width, they are measured again.
func (r *lyricRows) fit(track string, lines []lyrics.Line, width float32) {
	if r.track != track || len(r.h) != len(lines) || math.Abs(float64(r.width-width)) > 0.5 {
		*r = lyricRows{track: track, width: width, lines: lines, h: make([]float32, len(lines)), guess: make([]float32, len(lines))}
	}
	r.lines = lines
}

// height is how tall line i is: as laid out, or as its text would wrap at
// 30 points, Chinese a character to the point size, Latin half that.
func (r *lyricRows) height(i int) float32 {
	if r.h[i] > 0 {
		return r.h[i]
	}
	if r.guess[i] > 0 {
		return r.guess[i]
	}
	width := float32(0)
	for _, ch := range r.lines[i].Text {
		if unicode.In(ch, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul) {
			width += 30
		} else {
			width += 15
		}
	}
	room := max(r.width-8-24, 120)
	h := float32(math.Ceil(float64(max(width, 1)/room)))*37.5 + 20
	if r.lines[i].Translation != "" {
		h += 6 + 22
	}
	r.guess[i] = h
	return h
}

// top is where line i begins, from the top of the lyrics.
func (r *lyricRows) top(i int) float32 {
	y := float32(0)
	for j := range i {
		y += r.height(j) + lyricGap
	}
	return y
}

// span returns the lines from lo up to hi that are between from and to.
func (r *lyricRows) span(from, to float32) (lo, hi int) {
	y := float32(0)
	lo, hi = len(r.h), len(r.h)
	for i := range r.h {
		bottom := y + r.height(i)
		if lo == len(r.h) && bottom >= from {
			lo = i
		}
		if y > to {
			hi = i
			break
		}
		y = bottom + lyricGap
	}
	return min(lo, hi), hi
}

// measure notes how tall line i, built as e, was laid out.
func (r *lyricRows) measure(i int, e ui.Element) {
	if b := e.LayoutBounds(); b.H > 0 {
		r.h[i] = b.H
	}
}

// lyricGlide scrolls the lyrics to the line sung, as a critically damped
// spring, as ScrollTo's would. ScrollTo moves the offset in every frame,
// and MyGo builds the view again in the frame after each that moved it,
// and its spring creeps over the last pixel for half a second: with a line
// following another every second or two, most of a song. So the offset
// goes to the line at once, in one frame, and the lines are held back by
// what the glide has still to go, its lag, which settles to nothing within
// half a pixel.
type lyricGlide struct {
	on bool
	// The lag t after start: (a + b·t)·e^(−ωt), in DIPs.
	a, b  float64
	start time.Time
	// The offset it goes to, set in the next frame (pending), with the lag
	// that holds the lines back from it: the layout of the frame that
	// chose it takes the offset too, the lines built without the lag.
	target  float32
	pending bool
	set     float32 // the offset it set
}

// lyricOmega is the spring's ω: a line or two away, it comes within half a
// pixel in about 800 ms.
const lyricOmega = 10

// at returns the lag at now, and how fast it changes, in DIPs a second.
func (g *lyricGlide) at(now time.Time) (lag, v float64) {
	if !g.on {
		return 0, 0
	}
	t := now.Sub(g.start).Seconds()
	e := math.Exp(-lyricOmega * t)
	return (g.a + g.b*t) * e, (g.b - lyricOmega*(g.a+g.b*t)) * e
}

// to scrolls s to target from the next frame, gliding from where it
// shows, at the speed it went if it was on its way.
func (g *lyricGlide) to(c *ui.Context, s *ui.ScrollState, target float32) {
	now := c.Now()
	lag, v := g.at(now)
	shown := float64(s.Y) + lag
	g.target, g.pending = target, true
	g.on = !c.Preferences().ReduceMotion
	g.start, g.a = now, shown-float64(target)
	g.b = v + lyricOmega*g.a
	c.AnimationFrame()
}

// step returns the lag of the lines in this frame, and asks for the next
// while there is one. The listener scrolling stops the glide where it
// shows.
func (g *lyricGlide) step(c *ui.Context, s *ui.ScrollState) float32 {
	if g.pending {
		s.Y, g.set, g.pending = g.target, g.target, false
	}
	if !g.on {
		return 0
	}
	lag, v := g.at(c.Now())
	if s.Y != g.set {
		// The listener took over: the offset takes what showed.
		s.Y, g.on = max(0, min(s.Y+float32(lag), s.MaxY)), false
		return 0
	}
	if math.Abs(lag) < 0.5 && math.Abs(v) < 20 {
		g.on = false
		return 0
	}
	c.AnimationFrame()
	return float32(lag)
}

// queuePane lists the whole queue in play order, the track playing marked
// among the others: a click plays one there, and the list stays as it is.
func (a *app) queuePane(c *ui.Context) {
	pl := a.pl
	order, at := pl.PlayOrder()
	// Opened, it shows the track playing at its top; after, it follows it
	// as little as keeps it in view.
	if at >= 0 && at != a.queueAt {
		if a.queueAt < 0 {
			a.queueList.ScrollTo(at, ui.Start)
		} else {
			a.queueList.ScrollIntoView(at)
		}
		a.queueAt = at
	}
	a.queueList.Key = func(row int) any { return order[row] }
	ui.List(c.Key("queue"), &a.queueList, len(order), func(r int) {
		i := order[r]
		tr := &pl.Queue()[i]
		playing := r == at
		row := ui.ButtonBase(c.Key(i)).Height(58).PaddingX(10).Gap(14).Radius(12).AlignItems(ui.Center).FocusRing(false).Label(tr.Name).
			ContextMenu(a.songItems(tr, ""))
		switch {
		case playing:
			row.Background(white.Alpha(0.12))
		case row.Hovered():
			row.Background(white.Alpha(0.08))
		}
		if !tr.Playable() {
			row.Opacity(0.4)
		}
		row.Children(func() {
			a.cover(c, tr.Album.PicURL, 96, 42, 7)
			ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
				ui.Text(c, tr.Name).FontSize(14).FontWeight(600).SingleLine()
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					if tr.Video {
						ui.Text(c, mvName(tr)).FontSize(9).FontWeight(800).TextColor(white.Alpha(0.6)).Padding(0, 4).Radius(3).Border(1, white.Alpha(0.35))
					}
					ui.Text(c, tr.ArtistNames()).FontSize(12.5).TextColor(white.Alpha(0.55)).SingleLine().Shrink(1)
				})
			})
			if at >= 0 && r == at+1 {
				ui.Text(c, "接下来").FontSize(10.5).FontWeight(700).TextColor(white.Alpha(0.85)).Padding(2, 7).Radius(5).Background(white.Alpha(0.14))
			}
			ui.Text(c, clock(tr.Length())).FontSize(12).FontFeatures("tnum").TextColor(white.Alpha(0.4))
		})
		if row.Clicked() && tr.Playable() && !playing {
			pl.Jump(i)
		}
	}).Grow(1).Padding(8, 14, 40, 0).Gap(2) // the right, the scrollbar's: it does not cover the rows
}
