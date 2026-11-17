package app

import (
	"cmp"
	"math"
	"strconv"
	"time"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"

	"vinyl/internal/l10n"
	"vinyl/internal/player"
)

// scrubState is a progress bar's own: the value dragged, while it is, and
// the levels of the music its smoke follows.
type scrubState struct {
	v float64
	// held is set while the pointer pressed on the bar is down, wherever
	// it has gone since: Pressed is false off the bar, which is thin.
	held   bool
	active bool // dragged since the last frame, to seek once let go
	level  barState
}

func secs(s float64) time.Duration { return time.Duration(s * float64(time.Second)) }

// scrubber shows how far the track is, and seeks where it is dragged or
// clicked: the bar thickens under the pointer, and its knob shows. With a
// smoke color, the bar is a comet while music plays: what has played is a
// streak brightening to the playhead, which glows and trails smoke that
// rises and fades, thicker as the music is louder.
func (a *app) scrubber(c *ui.Context, st *scrubState, track, fill ui.Color, height float32, smoke ui.Color) ui.Element {
	pl := a.pl
	dur := pl.Duration().Seconds()
	if !st.active && !st.held {
		st.v = pl.Position().Seconds()
	}
	s := ui.SliderBase(c, &st.v, 0, max(dur, 1)).Height(height).FocusRing(false).Label(l10n.T("播放进度")).Cursor(ui.CursorPointer)
	// The press and its release come to the bar wherever the pointer is,
	// and go on to the slider, which follows the pointer.
	s.HandleInput(func(ev ui.InputEvent) bool {
		switch ev.Kind {
		case ui.InputPointerDown:
			st.held = ev.Button == 0
		case ui.InputPointerUp:
			st.held = false
		}
		return false
	})
	pressed := st.held
	switch {
	case pressed:
		st.active = true
	case st.active:
		// Let go: there, but not right at the end, which would end the
		// song as it begins to play.
		st.active = false
		pl.Seek(secs(min(st.v, max(0, dur-1))))
	case s.Changed():
		pl.Seek(secs(min(st.v, max(0, dur-1))))
	}
	hot := s.Hovered() || pressed
	thick := float32(4)
	if hot {
		thick = 6
	}
	thick = s.AnimateTo("thick", thick, ui.Snappy)
	knob := float32(0)
	if hot {
		knob = 1
	}
	knob = s.AnimateTo("knob", knob, ui.Snappy)
	// The smoke settles away as the music pauses, and while dragging.
	smoking := float32(0)
	if smoke.A > 0 && a.settings.Smoke && pl.Playing() && !pl.Loading() && !st.active {
		smoking = 1
	}
	smoking = s.AnimateTo("smoke", smoking, ui.Animation{Duration: 600 * time.Millisecond})
	s.Draw(func(p *ui.Painter, r ui.Rect) {
		at := st.v
		if !st.active {
			at = pl.Position().Seconds()
		}
		frac := float32(0)
		if dur > 0 {
			frac = float32(min(1, max(0, at/dur)))
		}
		y := r.Y + (r.H-thick)/2
		head, cy := r.X+r.W*frac, r.Y+r.H/2
		p.Fill(ui.Rect{X: r.X, Y: y, W: r.W, H: thick}, track, thick/2)
		played := ui.Rect{X: r.X, Y: y, W: max(thick, head-r.X), H: thick}
		if smoking > 0.01 {
			// A streak of light, brightest at the head.
			p.FillGradient(played, ui.LinearGradient{Angle: 90, Oklab: true,
				From: fill.Mix(smoke, smoking).Alpha(0.3), To: fill.Mix(smoke, 0.8*smoking)}, thick/2)
			lv, live := pl.Levels()
			st.level.step(p.Now(), lv, live)
			bass := st.level.group(0, 6)
			drawSmoke(p, head, cy, p.Now(), bass, smoke, smoking)
			// The glow of the head, swelling with the bass, and its core.
			glow := 9 + 8*bass
			p.Shadow(ui.Rect{X: head, Y: cy}, 0, 0, 0, glow, 2.5+2.5*bass, smoke.Alpha(0.95*smoking))
			core := thick * (1 + 0.25*bass)
			p.Fill(ui.Rect{X: head - core/2, Y: cy - core/2, W: core, H: core}, white.Mix(smoke, 0.15).Alpha(smoking), core/2)
		} else {
			p.Fill(played, fill, thick/2)
		}
		if knob > 0.01 {
			k := 12 * knob
			p.Shadow(ui.Rect{X: head - k/2, Y: r.Y + (r.H-k)/2, W: k, H: k}, k/2, 0, 1, 3, 0, ui.RGBA(0, 0, 0, 0.3))
			p.Fill(ui.Rect{X: head - k/2, Y: r.Y + (r.H-k)/2, W: k, H: k}, fill, k/2)
		}
		switch {
		case smoking > 0.01 && (pl.Playing() || smoking < 1):
			// The smoke drifts slowly, under a point a frame at 30 frames
			// a second, which show it smooth for a quarter of what a 120 Hz
			// display would draw.
			p.After(time.Second / 30)
		case pl.Playing() && !st.active && dur > 0:
			// Again when the fill has moved a pixel of the screen, which
			// takes a good part of a second, rather than every frame.
			perPixel := dur / float64(r.W*p.Scale()) / pl.Rate()
			p.After(max(16*time.Millisecond, secs(perPixel)))
		}
	})
	return s
}

// Smoke from the playhead: wisps shed one after another, each drifting
// back along the bar and rising, growing and fading over its life. Where a
// wisp is follows from the time alone, so nothing is kept between frames.
const (
	smokeLife  = 1.4 // seconds
	smokeWisps = 36  // alive at once, at most
)

// drawSmoke draws the smoke of a playhead at (x, y) at now, thicker as
// loud (0 to 1) is higher, at opacity shown.
func drawSmoke(p *ui.Painter, x, y float32, now time.Time, loud float32, color ui.Color, shown float32) {
	t := float64(now.UnixMilli()) / 1000
	interval := smokeLife / smokeWisps
	first := math.Floor(t / interval)
	for j := range smokeWisps {
		k := first - float64(j)
		age := float32(t - k*interval)
		if age < 0 || age > smokeLife {
			continue
		}
		// A wisp of its own: how fast it drifts and rises, how it curls,
		// how large it grows. Quiet music sheds fewer of them.
		r := func(o float64) float32 { return lineRandom(k, o) }
		if r(5) > 0.6+0.4*loud {
			continue
		}
		f := age / smokeLife
		wx := x - age*(12+22*r(1))
		wy := y - age*(12+18*r(2)) + float32(math.Sin(float64(age)*(2.5+2*float64(r(3)))+float64(r(4))*2*math.Pi))*6*f
		size := 3 + f*(12+10*r(4))
		alpha := 0.5 * float32(math.Pow(float64(1-f), 1.4)) * shown * (0.75 + 0.5*loud)
		p.Shadow(ui.Rect{X: wx, Y: wy}, 0, 0, 0, size, size*0.45, color.Alpha(alpha))
	}
}

// tickTimes redraws the view as the next second of the track begins, for
// the times shown.
func (a *app) tickTimes(c *ui.Context) {
	if a.pl.Playing() && a.pl.Ready() {
		pos := a.pl.Position()
		c.After(time.Duration(float64(time.Second-pos%time.Second)/a.pl.Rate()) + 10*time.Millisecond)
	}
}

// volumeButton is the speaker, which opens the volume in a popover above
// it, as an AppKit popover: a native slider standing up, the volume in
// percent over it. The wheel over the speaker turns it up and down
// without opening it.
func (a *app) volumeButton(c *ui.Context, color ui.Color) {
	b := iconButton(c.Key("volume-button"), volumeIcon(a.volume), 16, color, l10n.T("音量"))
	if b.Clicked() {
		a.volumeOpen = !a.volumeOpen
	}
	b.HandleInput(func(ev ui.InputEvent) bool { return a.scrollVolume(ev) })
	pop := popoverColors(a.pal.dark)
	ui.PopoverBase(c.Key("volume-popover"), b, &a.volumeOpen, func(panel ui.Element) {
		const arrow = 7
		panel.AttachTo(b, ui.AnchorTop, ui.AnchorBottom).Margin(0, 0, arrow+3, 0).Width(52).Padding(11, 0, 12, 0).
			Radius(12).Background(pop.fill).Border(0.5, pop.edge).Shadow(0, 8, 24, -4, ui.RGBA(0, 0, 0, 0.2)).
			AlignItems(ui.Center).Gap(8).Label(l10n.T("音量"))
		panel.Draw(func(p *ui.Painter, r ui.Rect) {
			// The arrow pointing at the speaker, its sides edged as the
			// panel is, over the panel's border.
			cx, foot := r.X+r.W/2, r.Y+r.H
			tip := (&ui.Path{}).MoveTo(cx-arrow-1, foot-0.75).LineTo(cx-1.2, foot+arrow-1.2).
				QuadTo(cx, foot+arrow, cx+1.2, foot+arrow-1.2).LineTo(cx+arrow+1, foot-0.75).Close()
			p.FillPath(tip, pop.fill)
			edge := (&ui.Path{}).MoveTo(cx-arrow-1, foot).LineTo(cx-1.2, foot+arrow-1.2).
				QuadTo(cx, foot+arrow, cx+1.2, foot+arrow-1.2).LineTo(cx+arrow+1, foot)
			p.StrokePath(edge, 0.5, pop.edge)
		})
		ui.Textf(c, "%d%%", int(a.volume*100+0.5)).FontSize(11).FontWeight(500).FontFeatures("tnum").TextColor(a.pal.muted)
		a.nativeVolumeSlider(c, pop)
	})
}

// popover holds the colors of a popover as AppKit draws them.
type popover struct {
	fill, edge, track, knob ui.Color
}

func popoverColors(dark bool) popover {
	if dark {
		return popover{fill: ui.Hex("#2b2b2e"), edge: ui.RGBA(255, 255, 255, 0.14), track: ui.RGBA(255, 255, 255, 0.16), knob: ui.Hex("#ececec")}
	}
	return popover{fill: ui.Hex("#f6f6f6"), edge: ui.RGBA(0, 0, 0, 0.12), track: ui.RGBA(0, 0, 0, 0.12), knob: ui.Hex("#ffffff")}
}

// nativeVolumeSlider is a vertical slider as AppKit draws one: a thin
// track, filled with the accent from the foot to a round knob.
func (a *app) nativeVolumeSlider(c *ui.Context, pop popover) {
	const knob, track = 20, 4
	s := ui.SliderBase(c.Key("volume"), &a.volume, 0, 1).Vertical().Size(knob+8, 124).PaddingY(knob / 2).
		Step(0.01).AutoFocus().FocusRing(false).Label(l10n.T("音量"))
	if s.Changed() {
		a.setVolume(a.volume)
	}
	s.HandleInput(func(ev ui.InputEvent) bool { return a.scrollVolume(ev) })
	pressed := s.Pressed()
	v := float32(a.volume)
	accent := a.pal.accent
	s.Draw(func(p *ui.Painter, r ui.Rect) {
		top, foot := r.Y+knob/2, r.Y+r.H-knob/2
		x := r.X + (r.W-track)/2
		y := foot - (foot-top)*v
		p.Fill(ui.Rect{X: x, Y: top - track/2, W: track, H: foot - top + track}, pop.track, track/2)
		p.Fill(ui.Rect{X: x, Y: y, W: track, H: foot - y + track/2}, accent, track/2)
		k := ui.Rect{X: r.X + (r.W-knob)/2, Y: y - knob/2, W: knob, H: knob}
		p.Shadow(k, knob/2, 0, 0.5, 1.5, 0, ui.RGBA(0, 0, 0, 0.3))
		p.Shadow(k, knob/2, 0, 1, 4, 0, ui.RGBA(0, 0, 0, 0.12))
		fill := pop.knob
		if pressed {
			fill = fill.Mix(ui.RGB(0, 0, 0), 0.06)
		}
		p.Fill(k, fill, knob/2)
		p.Stroke(k, ui.RGBA(0, 0, 0, 0.08), knob/2, 0.5)
	})
}

// scrollVolume turns the volume up as the wheel or the fingers scroll up:
// the fingers by how far they go, a wheel by 5% a notch. A notch is as far
// as the system scrolls a page for it, on Windows 33 a line of its "lines
// to scroll", which at 9 turned the volume 74% a notch (#42).
func (a *app) scrollVolume(ev ui.InputEvent) bool {
	if ev.Kind != ui.InputScroll {
		return false
	}
	v := a.volume - float64(ev.DY)/400
	if !ev.Precise {
		// To the percent shown, so that turning down comes to 0, silent.
		v = math.Round((a.volume-0.05*float64(cmp.Compare(ev.DY, 0)))*100) / 100
	}
	a.setVolume(min(1, max(0, v)))
	return true
}

// volumeIcon is the speaker for volume v: silent, or with one or two waves.
func volumeIcon(v float64) *ui.SVG {
	switch {
	case v == 0:
		return iconMute
	case v < 0.4:
		return iconVolumeLow
	default:
		return iconVolume
	}
}

// muteButton is the speaker, which mutes and unmutes.
func (a *app) muteButton(c *ui.Context, color ui.Color) {
	icon := iconVolume
	if a.volume == 0 {
		icon = iconMute
	}
	if iconButton(c.Key("mute"), icon, 16, color, l10n.T("静音")).Clicked() {
		v := 0.0
		if a.volume > 0 {
			a.mutedFrom = a.volume
		} else {
			v = max(a.mutedFrom, 0.5)
		}
		a.setVolume(v)
	}
}

// volumeSlider is a slim slider with the speaker before it, which mutes.
func (a *app) volumeSlider(c *ui.Context, color, track ui.Color, width float32) {
	ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
		a.muteButton(c, color)
		s := ui.SliderBase(c.Key("volume"), &a.volume, 0, 1).Width(width).Height(16).FocusRing(false).Label(l10n.T("音量"))
		if s.Changed() {
			a.setVolume(a.volume)
		}
		hot := s.Hovered() || s.Pressed()
		thick := float32(4)
		if hot {
			thick = 5
		}
		thick = s.AnimateTo("thick", thick, ui.Snappy)
		v := float32(a.volume)
		s.Draw(func(p *ui.Painter, r ui.Rect) {
			y := r.Y + (r.H-thick)/2
			p.Fill(ui.Rect{X: r.X, Y: y, W: r.W, H: thick}, track, thick/2)
			p.Fill(ui.Rect{X: r.X, Y: y, W: max(thick, r.W*v), H: thick}, color, thick/2)
		})
	})
}

// transport is the row of controls: shuffle, previous, play, next, repeat.
// An episode playing here has its speed where shuffle goes, as podcasts
// are listened to in order.
func (a *app) transport(c *ui.Context, color, dim, on ui.Color, size float32) {
	pl := a.pl
	ui.Row(c).Gap(size * 0.5).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		if tr := pl.Current(); tr != nil && tr.Episode && !a.isRemote() {
			a.rateButton(c.Key("rate"), dim, on, size)
		} else {
			shuffle := dim
			if pl.Shuffle() {
				shuffle = on
			}
			if iconButton(c.Key("shuffle"), iconShuffle, size*0.62, shuffle, l10n.T("随机播放")).Clicked() {
				pl.ToggleShuffle()
			}
		}
		if iconButton(c.Key("prev"), iconPrev, size*0.85, color, l10n.T("上一首")).Clicked() {
			pl.Prev()
		}
		if playButton(c.Key("play"), pl.Playing(), size*1.25, color).Clicked() {
			pl.TogglePlay()
		}
		if iconButton(c.Key("next"), iconNext, size*0.85, color, l10n.T("下一首")).Clicked() {
			pl.Next()
		}
		repeat, ricon := dim, iconRepeat
		switch pl.Repeat() {
		case player.RepeatAll:
			repeat = on
		case player.RepeatOne:
			repeat, ricon = on, iconRepeat1
		}
		if iconButton(c.Key("repeat"), ricon, size*0.62, repeat, l10n.T("循环")).Clicked() {
			pl.CycleRepeat()
		}
	})
}

// speeds are the rates an episode plays at, in the rate button's menu.
var speeds = []float64{0.5, 0.75, 1, 1.25, 1.5, 1.75, 2, 2.5, 3}

func speedLabel(r float64) string { return strconv.FormatFloat(r, 'f', -1, 64) + "×" }

// rateButton shows how fast the episode plays, in the color on unless at
// its own pace, and opens a menu of speeds that sets it.
func (a *app) rateButton(c *ui.Context, dim, on ui.Color, size float32) {
	rate := a.pl.Rate()
	color := dim
	if rate != 1 {
		color = on
	}
	side := size*0.62 + 16 // shuffle's, whose place it takes
	b := ui.ButtonBase(c).Height(side).MinWidth(side).PaddingX(4).Shrink(0).Center().Radius(999).
		Label(l10n.T("播放速度")).TextColor(color)
	bg := ui.Transparent
	if b.Hovered() {
		bg = ui.RGBA(128, 128, 128, 0.14)
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
	b.Children(func() {
		ui.Text(c, speedLabel(rate)).FontSize(max(10, size*0.5)).FontWeight(600).FontFeatures("tnum").SingleLine()
	})
	b.Menu(func(m *ui.Menu) {
		for _, r := range speeds {
			if m.Item(speedLabel(r)).Radio(r == rate).Chosen() {
				a.setRate(r)
			}
		}
	})
}

// setRate has episodes play at r from now on.
func (a *app) setRate(r float64) {
	a.pl.SetRate(r)
	a.settings.PodcastRate = r
	a.settings.save()
}

// playerBar runs along the foot of the content: what plays, its controls
// and its progress, and the volume.
func (a *app) playerBar(c *ui.Context) {
	p := a.pal
	pl := a.pl
	tr := pl.Current()
	a.tickTimes(c)
	// Over the foot of the page: while content is under it, its edge frosts
	// it, with a hairline along the top.
	edge := ui.Box(c.Key("bottom-edge")).Absolute().Left(0).Right(0).Bottom(0).Height(playerBarHeight).
		Material(glass.ScrollEdge{Hard: true, Bottom: true, Background: p.bg})
	edge.Opacity(edgeShown(edge, a.underBottom))
	// The controls keep the middle, between sides that share what is left:
	// narrower, the middle gives up to 140 DIPs, and what plays keeps room
	// for its cover, some of its title and its heart.
	const controls = 4*32 + 3*4 // lyrics, desktop lyrics, queue and the speaker
	// Before them, where the window has room beyond what the rest keeps, the
	// music video's button, then the badge of the bitrate ("256k", about
	// 40): in a narrow window they pushed the speaker out of it.
	w, _ := c.Size()
	room := w - sidebarWidth - 1 - 2*20 - 2*20 - 150 - 300 - controls
	side := float32(controls)
	mv := tr != nil && pl.HasVideo() && room >= 32+4
	if mv {
		side += 32 + 4
	}
	badge := tr != nil && pl.Bitrate() > 0 && room-(side-controls) >= 44+4
	if badge {
		side += 44 + 4
	}
	ui.Row(c).Absolute().Left(0).Right(0).Bottom(0).Height(playerBarHeight).PaddingX(20).Gap(20).AlignItems(ui.Center).Children(func() {
		// What plays.
		ui.Row(c).Grow(1).Basis(0).MinWidth(150).Children(func() {
			if tr == nil {
				ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
					ui.Box(c).Size(48, 48).Radius(8).Background(p.hover).Center().Children(func() {
						ui.Icon(c, iconNote).Size(18, 18).TextColor(p.faint)
					})
					ui.Text(c, l10n.T("未在播放")).FontSize(13).TextColor(p.muted)
				})
				return
			}
			info := ui.ButtonBase(c.Key("info")).Gap(12).AlignItems(ui.Center).FocusRing(false).Shrink(1).MinWidth(0).Label(l10n.T("打开正在播放"))
			hovered := info.Hovered()
			info.Children(func() {
				ui.Box(c).Size(48, 48).Shrink(0).Children(func() {
					a.cover(c, tr.Album.PicURL, 120, 48, 8).Shadow(0, 2, 8, 0, p.cardShade)
					if hovered {
						ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Radius(8).Center().
							Background(ui.RGBA(0, 0, 0, 0.35)).TextColor(ui.RGB(255, 255, 255)).Children(func() {
							ui.Icon(c, iconDown).Size(18, 18).Rotate(180)
						})
					}
				})
				ui.Column(c).Shrink(1).MinWidth(0).Gap(3).Children(func() {
					ui.Text(c, tr.Name).FontSize(13.5).FontWeight(600).SingleLine()
					switch {
					case pl.Loading():
						ui.Text(c, l10n.T("正在缓冲…")).FontSize(12).TextColor(p.muted).SingleLine()
					case pl.Err() != "":
						ui.Text(c, pl.Err()).FontSize(12).TextColor(p.accent).SingleLine()
					default:
						a.artistLinks(c.Key("by"), tr.Artists, 12, p.muted, p.text)
					}
				})
			})
			if info.Clicked() {
				a.nowPlaying = true
			}
			a.likeButton(c.Key("like"), tr, 16, p.muted).Margin(0, 0, 0, 4).Shrink(0)
		})

		// Controls over the progress.
		ui.Column(c).Width(440).Shrink(1).MinWidth(300).Gap(0).AlignItems(ui.Stretch).Children(func() {
			a.transport(c, p.text, p.muted, p.accent, 18)
			ui.Row(c).Gap(10).AlignItems(ui.Center).Margin(-4, 0, 0, 0).Children(func() {
				ui.Text(c, clock(pl.Position())).FontSize(10.5).FontFeatures("tnum").TextColor(p.faint).Width(34).TextAlign(ui.End)
				a.scrubber(c.Key("bar-scrub"), &a.barScrub, p.press, p.text.Alpha(0.75), 14, p.accent).Grow(1)
				ui.Text(c, clock(pl.Duration())).FontSize(10.5).FontFeatures("tnum").TextColor(p.faint).Width(34)
			})
		})

		// Lyrics and volume.
		ui.Row(c).Grow(1).Basis(0).MinWidth(side).Justify(ui.End).AlignItems(ui.Center).Gap(4).Children(func() {
			if badge {
				a.qualityBadge(c, p.press, p.faint)
			}
			if mv {
				a.barMVButton(c)
			}
			lyr := p.muted
			if a.nowPlaying {
				lyr = p.accent
			}
			if iconButton(c.Key("lyrics"), iconLyrics, 16, lyr, lyricsName(tr)).Clicked() && tr != nil {
				a.nowPlaying, a.npPane = true, a.lyricsPaneChosen()
			}
			if deskHere {
				on := p.muted
				if a.settings.DeskLyrics {
					on = p.accent
				}
				if iconButton(c.Key("desk"), iconDesk, 16, on, l10n.T("桌面歌词")).Clicked() {
					a.setDesk(!a.settings.DeskLyrics)
				}
			}
			if iconButton(c.Key("queue"), iconQueue, 16, p.muted, l10n.T("播放队列")).Clicked() && tr != nil {
				a.nowPlaying, a.npPane = true, paneQueue
			}
			a.connectButton(c)
			a.volumeButton(c, p.text.Alpha(0.75))
		})
	})
}
