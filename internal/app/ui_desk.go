package app

import (
	"runtime"
	"slices"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"vinyl/internal/l10n"
	"vinyl/internal/lyrics"
	"vinyl/internal/spotify"
)

// desk is the desktop lyrics: the line sung, large, floating over every
// app near the foot of the screen, its words taking the cover's color as
// they are sung, and under it its translation or the line coming next.
// Between lines, the song's name. The pointer over it shows the player's
// controls on a pane that moves it; locked, it lets clicks through to what
// is under it, but for the key that unlocks it.
type desk struct {
	a   *app
	win *mygo.Window

	hover   bool   // the pointer is over the window
	passing bool   // clicks pass through to what is under it
	unwatch func() // stops the pointer's moves polling, nil while they don't
	watched bool   // they poll
	polling bool   // a poll is due
}

const (
	deskW, deskH = 860, 156
	// The controls along the top, and the key that unlocks, in the middle.
	deskTop, deskTool = 8, 32
)

// deskHere says the desktop lyrics can be had here: Linux's windows cannot
// let clicks through, so locked they would be in the way.
var deskHere = runtime.GOOS != "linux"

func newDesk(a *app) *desk {
	d := &desk{a: a}
	wa := mygo.Screen.PrimaryDisplay().WorkArea
	d.win = mygo.NewWindow(mygo.WindowOptions{
		Title:         l10n.T("桌面歌词"),
		Panel:         true,
		Hidden:        true,
		Frameless:     true,
		Transparent:   true,
		DisableShadow: true,
		DisableResize: true,
		// Where the listener left it, at the next launch too.
		StateKey: "desk-lyrics",
		X:        wa.X + (wa.Width-deskW)/2,
		Y:        wa.Y + wa.Height - deskH - 24,
		Width:    deskW,
		Height:   deskH,
		Content:  ui.View(d.view),
	})
	// Escape and the like close a panel: this one hides with its setting.
	d.win.OnClose(func(e *mygo.CloseEvent) {
		if !a.quitting {
			e.PreventDefault()
		}
	})
	d.setEnabled(a.settings.DeskLyrics)
	return d
}

// setDesk shows the desktop lyrics or hides them, the next launch too.
func (a *app) setDesk(on bool) {
	a.settings.DeskLyrics = on
	a.settings.save()
	if a.desk != nil {
		a.desk.setEnabled(on)
	}
}

// setDeskLocked locks the desktop lyrics, letting clicks through, or
// unlocks them.
func (a *app) setDeskLocked(locked bool) {
	a.settings.DeskLocked = locked
	a.settings.save()
	if a.desk != nil && a.desk.win != nil {
		a.desk.poll()
	}
}

// setEnabled shows the window or hides it.
func (d *desk) setEnabled(on bool) {
	if d.win == nil {
		return // as in tests
	}
	d.hover = false
	if on {
		d.win.ShowInactive()
		d.watch(true)
		d.poll()
		return
	}
	d.win.Hide()
	d.watch(false)
}

// watch follows the pointer's moves while the lyrics show, where the
// system tells of them (macOS): elsewhere the window polls.
func (d *desk) watch(on bool) {
	switch {
	case on && d.unwatch == nil:
		d.unwatch, d.watched = watchPointer(d.poll)
	case !on && d.unwatch != nil:
		d.unwatch()
		d.unwatch, d.watched = nil, false
	}
}

// poll finds whether the pointer is over the window, which, letting clicks
// through, cannot see it: over it, the window shows its controls, or
// locked, takes clicks on the key only.
func (d *desk) poll() {
	if d.win == nil || !d.a.settings.DeskLyrics {
		return
	}
	pt := mygo.Screen.CursorScreenPoint()
	b := d.win.Bounds()
	x, y := float32(pt.X-b.X), float32(pt.Y-b.Y)
	inside := x >= 0 && y >= 0 && x < float32(b.Width) && y < float32(b.Height)
	if inside != d.hover {
		d.hover = inside
		d.win.Invalidate()
	}
	const half = (deskTool + 16) / 2 // the key's button and a little more
	onKey := inside && abs32(x-float32(b.Width)/2) <= half && abs32(y-deskTop-deskTool/2) <= half
	if pass := d.a.settings.DeskLocked && !onKey; pass != d.passing {
		d.passing = pass
		d.win.SetIgnoreMouseEvents(pass)
	}
	// Over it, poll for the pointer leaving; without the system telling of
	// its moves, poll all along.
	if (inside || !d.watched) && !d.polling {
		wait := 150 * time.Millisecond
		if inside {
			wait = 50 * time.Millisecond
		}
		d.polling = true
		time.AfterFunc(wait, func() {
			mygo.RunOnMain(func() {
				d.polling = false
				d.poll()
			})
		})
	}
}

func abs32(v float32) float32 { return max(v, -v) }

func (d *desk) view(c *ui.Context) {
	a := d.a
	c.Root().Background(ui.Transparent)
	locked := a.settings.DeskLocked
	quick := ui.Animation{Duration: 180 * time.Millisecond}
	stage := ui.Box(c).Fill()
	pane := stage.AnimateTo("pane", b2f(d.hover && !locked), quick)
	key := stage.AnimateTo("key", b2f(d.hover && locked), quick)
	if !locked {
		stage.DragWindow()
	}
	stage.Children(func() {
		if pane > 0.01 {
			ui.Box(c.Key("pane")).Absolute().Top(0).Left(0).Right(0).Bottom(0).Radius(18).
				Background(ui.RGBA(18, 18, 20, 0.55*pane)).PassThrough()
		}
		ui.Row(c.Key("tools")).Absolute().Top(deskTop).Left(0).Right(0).Height(deskTool).Justify(ui.Center).AlignItems(ui.Center).Gap(4).Children(func() {
			if pane > 0.01 {
				d.tools(c, pane)
			}
			if key > 0.01 && pane <= 0.01 {
				// On a dark disc, to be found on a light desktop too.
				if iconButton(c.Key("unlock"), iconUnlock, 16, white, l10n.T("解锁桌面歌词")).Background(ui.RGBA(18, 18, 20, 0.6)).Opacity(key).Clicked() {
					a.setDeskLocked(false)
				}
			}
		})
		d.lyrics(c)
	})
	a.syncWindows()
}

// tools are the controls the pointer brings up: the player's, the lock,
// and a way to close the lyrics.
func (d *desk) tools(c *ui.Context, shown float32) {
	a := d.a
	pl := a.pl
	ui.Row(c).Gap(2).AlignItems(ui.Center).Opacity(shown).Children(func() {
		if iconButton(c.Key("prev"), iconPrev, 16, white, l10n.T("上一首")).Clicked() {
			pl.Prev()
		}
		if playButton(c.Key("play"), pl.Playing(), 16, white).Clicked() {
			pl.TogglePlay()
		}
		if iconButton(c.Key("next"), iconNext, 16, white, l10n.T("下一首")).Clicked() {
			pl.Next()
		}
		ui.Box(c).Size(1, 16).Margin(0, 8).Background(white.Alpha(0.25))
		if iconButton(c.Key("lock"), iconLock, 16, white, l10n.T("锁定桌面歌词")).Clicked() {
			a.setDeskLocked(true)
		}
		if iconButton(c.Key("close"), iconX, 16, white, l10n.T("关闭桌面歌词")).Clicked() {
			a.setDesk(false)
		}
	})
}

// lyrics shows the line sung and the one under it, the line rising in as
// it begins; else the song's name and artists, or nothing playing, the
// app's name.
func (d *desk) lyrics(c *ui.Context) {
	a := d.a
	pl := a.pl
	tr := pl.Current()
	lines, _ := pl.Lyrics()
	pos := pl.SongPosition()
	playing := pl.Playing()
	cur := -1
	if tr != nil && timed(lines) {
		cur = sungLine(lines, pos)
		if playing {
			if wait := lyricChange(lines, lyrics.Index(lines, pos), pos); wait > 0 {
				c.After(time.Duration(float64(wait)/pl.Rate()) + 20*time.Millisecond)
			}
		}
	}
	text, under, size := appName, "", float32(26)
	switch {
	case cur >= 0:
		text, under, size = lines[cur].Text, deskUnder(lines, cur, a.settings.DeskUnder), 36
	case tr != nil:
		text, under = tr.Name, tr.ArtistNames()
	}
	rise := float32(1)
	if cur >= 0 && playing {
		if rise = clamp01(float32((pos - lines[cur].At).Seconds() / 0.3)); rise < 1 {
			c.AnimationFrame()
		}
	}
	rise = easeOutCubic(rise)
	w, _ := c.Size()
	scale := a.settings.deskScale()
	size = fitFont(c, text, size*scale, 800, w-64)
	underSize := fitFont(c, under, 18*scale, 600, w-64)

	ui.Column(c.Key(cur)).Absolute().Top(deskTop+deskTool).Left(32).Right(32).Bottom(12).Center().Gap(6).
		Opacity(rise).Translate(0, (1-rise)*10).PassThrough().Children(func() {
		if cur >= 0 && len(lines[cur].Words) > 0 {
			d.words(c, &lines[cur], pos, size, playing)
		} else {
			deskShadow(ui.Text(c.Key("line"), text).FontSize(size).FontWeight(800).TextColor(white).SingleLine().PassThrough())
		}
		if under != "" {
			deskShadow(ui.Text(c.Key("under"), under).FontSize(underSize).FontWeight(600).TextColor(white.Alpha(0.78)).SingleLine().PassThrough())
		}
	})
}

// words shows line l with its words sung by pos in the color chosen.
func (d *desk) words(c *ui.Context, l *lyrics.Line, pos time.Duration, size float32, playing bool) {
	tint := d.tint()
	// White words sung stand out from those to come only as these dim.
	ahead := white
	if tint == white {
		ahead = white.Alpha(0.5)
	}
	ui.RichText(c.Key("words")).FontSize(size).FontWeight(800).SingleLine().PassThrough().Children(func() {
		for i, wd := range l.Words {
			lit := clamp01(float32((pos - wd.Start).Seconds() / max(0.05, (wd.End-wd.Start).Seconds())))
			deskShadow(ui.Text(c.Key(i), wd.Text).TextColor(ahead.Mix(tint, lit)))
		}
	})
	// Words light up smoothly enough at 25 frames a second; sung, they rest.
	if playing && pos < l.Words[len(l.Words)-1].End {
		c.After(40 * time.Millisecond)
	}
}

// tint is the color of words sung: the one chosen, or the cover's, light
// enough to read on any desktop, or the app's own.
func (d *desk) tint() ui.Color {
	a := d.a
	for _, k := range deskColors {
		if k.id == a.settings.DeskColor {
			return k.color
		}
	}
	if tr := a.pl.Current(); tr != nil {
		if art := a.art.get(spotify.Sized(tr.Album.PicURL, 96), false); art != nil && !art.failed {
			return art.tint.Mix(white, 0.3)
		}
	}
	return a.pal.accent
}

// deskSettings are the desktop lyrics' part of the settings: showing them,
// their size, the color of words sung and what shows under the line, the
// lyrics changing as they are chosen.
func (a *app) deskSettings(c *ui.Context, group func(string, func()), row func(string, string, func())) {
	p := a.pal
	s := &a.settings
	group(l10n.T("桌面歌词"), func() {
		row(l10n.T("显示桌面歌词"), l10n.T("正在唱的那句浮在所有窗口上面；锁定后鼠标可以直接穿过"), func() {
			if ui.Switch(c.Key("desk"), &s.DeskLyrics).Label(l10n.T("显示桌面歌词")).Changed() {
				a.setDesk(s.DeskLyrics)
			}
		})
		row(l10n.T("字号"), "", func() {
			sizes := []string{"small", "", "large"}
			i := slices.Index(sizes, s.DeskSize)
			if i < 0 {
				i = 1
			}
			if segmented(c.Key("desk-size"), &i, l10n.T("小"), l10n.T("中"), l10n.T("大")) {
				s.DeskSize = sizes[i]
				s.save()
				a.redraw()
			}
		})
		row(l10n.T("唱到的字的颜色"), "", func() {
			ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
				swatch := func(id, label string, dot func()) {
					ring := ui.Transparent
					if s.DeskColor == id {
						ring = p.accent
					}
					b := ui.ButtonBase(c.Key("desk-color-"+id)).Size(28, 28).Radius(999).Center().Border(2, ring).Label(label).Tooltip(label)
					b.Children(dot)
					if b.Clicked() {
						s.DeskColor = id
						s.save()
						a.redraw()
					}
				}
				swatch("", l10n.T("跟随封面"), func() {
					if tr := a.pl.Current(); tr != nil {
						a.cover(c, tr.Album.PicURL, 96, 20, 10)
						return
					}
					ui.Box(c).Size(20, 20).Radius(999).Background(p.hover).Center().Children(func() {
						ui.Icon(c, iconNote).Size(11, 11).TextColor(p.muted)
					})
				})
				for _, k := range deskColors {
					swatch(k.id, l10n.T(k.label), func() {
						ui.Box(c).Size(20, 20).Radius(999).Background(k.color).Border(1, p.hairline)
					})
				}
			})
		})
		under := ""
		if s.DeskUnder == "" {
			under = l10n.T("没有翻译的歌，第二行显示下一句")
		}
		row(l10n.T("第二行"), under, func() {
			i := max(0, slices.Index(deskUnders, s.DeskUnder))
			if segmented(c.Key("desk-under"), &i, l10n.T("翻译"), l10n.T("下一句"), l10n.T("不显示")) {
				s.DeskUnder = deskUnders[i]
				s.save()
				a.redraw()
			}
		})
	})
}

// deskColors are the colors words sung can take, rather than the cover's.
var deskColors = []struct {
	id, label string
	color     ui.Color
}{
	{"white", "白色", white},
	{"red", "红色", ui.RGB(255, 92, 108)},
	{"pink", "粉色", ui.RGB(255, 143, 184)},
	{"gold", "金色", ui.RGB(255, 205, 96)},
	{"green", "绿色", ui.RGB(110, 226, 158)},
	{"blue", "蓝色", ui.RGB(116, 184, 255)},
	{"purple", "紫色", ui.RGB(186, 150, 255)},
}

// deskScale is how much larger than the middle size the lyrics are.
func (s settings) deskScale() float32 {
	switch s.DeskSize {
	case "small":
		return 0.8
	case "large":
		return 1.25
	}
	return 1
}

// deskUnders are what can show under the line: its translation, or the
// next line without one; always the next line; nothing.
var deskUnders = []string{"", "next", "none"}

// deskUnder is what shows under line cur, as mode, one of deskUnders, says.
func deskUnder(lines []lyrics.Line, cur int, mode string) string {
	if mode == "none" {
		return ""
	}
	if t := lines[cur].Translation; t != "" && mode == "" {
		return t
	}
	if cur+1 < len(lines) {
		return lines[cur+1].Text
	}
	return ""
}

// deskShadow sets text off from whatever the desktop is behind it.
func deskShadow(e ui.Element) ui.Element {
	return e.TextShadow(0, 1, 3, black.Alpha(0.85))
}

// fitFont returns size, or smaller, down to two thirds of it, for text
// of weight to fit in width on one line.
func fitFont(c *ui.Context, text string, size float32, weight int, width float32) float32 {
	if tw, _ := c.MeasureText(0, ui.Span{Text: text, Size: size, Weight: weight}); tw > width {
		return max(size*2/3, size*width/tw)
	}
	return size
}
