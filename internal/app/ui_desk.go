package app

import (
	"runtime"
	"slices"
	"strings"
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
	undecorate(d.win.NativeHandle())
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
				// On a dark disc, to be found on a light desktop too. The
				// lock shows how they are: closed, locked.
				if iconButton(c.Key("unlock"), iconLock, 16, white, l10n.T("解锁桌面歌词")).Background(ui.RGBA(18, 18, 20, 0.6)).Opacity(key).Clicked() {
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
		if iconButton(c.Key("lock"), iconUnlock, 16, white, l10n.T("锁定桌面歌词")).Clicked() {
			a.setDeskLocked(true)
		}
		if iconButton(c.Key("close"), iconX, 16, white, l10n.T("关闭桌面歌词")).Clicked() {
			a.setDesk(false)
		}
	})
}

// lyrics shows the line sung and the one under it, or dimmed, the line
// coming through a break or the last after it; without timed lyrics the
// song's name and artists, or nothing playing, the app's name. A line sung
// comes in word by word out of a blur, and as its time runs out dissolves
// word by word, the next coming in where it was (deskFade).
func (d *desk) lyrics(c *ui.Context) {
	a := d.a
	pl := a.pl
	tr := pl.Current()
	lines, _ := pl.Lyrics()
	pos := pl.SongPosition()
	playing := pl.Playing()
	cur, resting := -1, false
	fade := deskFade{pos: pos}
	if tr != nil && timed(lines) {
		cur, resting = deskLine(lines, pos)
		if playing {
			if wait := lyricChange(lines, lyrics.Index(lines, pos), pos); wait > 0 {
				c.After(time.Duration(float64(wait)/pl.Rate()) + 20*time.Millisecond)
				if cur >= 0 && !resting {
					fade.change = deskGoes(lines, cur, pos, wait)
				}
			}
		}
		// A line comes in as it follows another; one shown dimmed through the
		// break before it only brightens.
		if cur >= 0 && !resting {
			fade.at = lines[cur].At
			fade.enters = sungLine(lines, lines[cur].At-time.Millisecond) >= 0
		}
	}
	text, under, size := appName, "", float32(26)
	switch {
	case cur >= 0:
		text, under, size = lines[cur].Text, deskUnder(lines, cur, a.settings.DeskUnder), 36
	case tr != nil:
		text, under = tr.Name, tr.ArtistNames()
	}
	w, _ := c.Size()
	scale := a.settings.deskScale()
	size = fitFont(c, text, size*scale, 800, w-64)
	underSize := fitFont(c, under, 18*scale, 600, w-64)

	// The words of a line and the line under it go as their keys do: the
	// line shown dimmed through the break before it only brightens.
	col := ui.Column(c.Key("lyrics")).Absolute().Top(deskTop + deskTool).Left(32).Right(32).Bottom(12).Center().Gap(6)
	lit := col.AnimateTo("lit", 1-0.45*b2f(resting), ui.Animation{Duration: 300 * time.Millisecond})
	col.Opacity(lit).PassThrough().Children(func() {
		// As wide as the window, the words going stay where they were, the
		// line coming centered over them.
		ui.Row(c.Key("line")).FillWidth().Justify(ui.Center).PassThrough().Children(func() {
			words, color := lyrics.SpreadWords(text, 0, 0), func(int) ui.Color { return white }
			if cur >= 0 && len(lines[cur].Words) > 0 {
				words, color = lines[cur].Words, d.wordColors(c, &lines[cur], pos, playing)
			}
			// The space after a word, which its text alone loses at its end.
			aa, _ := c.MeasureText(0, ui.Span{Text: "aa", Size: size, Weight: 800})
			asa, _ := c.MeasureText(0, ui.Span{Text: "a a", Size: size, Weight: 800})
			fade.n = len(words)
			for i, wd := range words {
				e := deskShadow(ui.Text(c.Key(deskBit{cur, text, i}), wd.Text).FontSize(size).FontWeight(800).TextColor(color(i)).SingleLine().PassThrough())
				if strings.HasSuffix(wd.Text, " ") {
					e.Margin(0, asa-aa, 0, 0)
				}
				fade.style(e, i, wd.End)
			}
		})
		if under != "" {
			e := deskShadow(ui.Text(c.Key(deskBit{cur, under, -1}), under).FontSize(underSize).FontWeight(600).TextColor(white.Alpha(0.78)).SingleLine().PassThrough())
			fade.style(e, fade.n/2, 0)
		}
	})
	if playing {
		fade.frames(c)
	}
}

// deskGoes returns when line cur, sung at pos, dissolves, the lyrics
// changing in wait: as another line takes its place. 0 when it stays, as
// the last line does to the end, dimmed: it dissolved, and came back.
func deskGoes(lines []lyrics.Line, cur int, pos, wait time.Duration) time.Duration {
	if next, _ := deskLine(lines, pos+wait+20*time.Millisecond); next == cur {
		return 0
	}
	return pos + wait
}

// deskFade is where the words of the line shown are in coming in and going,
// at pos: a line sung from at, going at change (0 for one that stays), of
// n words.
type deskFade struct {
	pos, at, change time.Duration
	enters          bool
	n               int
}

// The words come in one after another over at most deskInSpan, each over
// deskInTime; as the line's time runs out they go one after another over
// deskOutSpan, each over deskOutTime, starting deskOutFrom before the next
// line: the last half gone as it comes in over it, with no empty moment.
const (
	deskInSpan, deskInTime   = 700 * time.Millisecond, 800 * time.Millisecond
	deskOutSpan, deskOutTime = 600 * time.Millisecond, 800 * time.Millisecond
	deskOutFrom              = deskOutSpan + deskOutTime - 400*time.Millisecond
)

// word returns how far the i-th word, sung by sung, has come in and gone,
// each 0 to 1.
func (f deskFade) word(i int, sung time.Duration) (in, out float32) {
	in = 1
	if f.enters {
		step := min(70*time.Millisecond, deskInSpan/time.Duration(max(f.n, 1)))
		in = clamp01(float32((f.pos - f.at - time.Duration(i)*step).Seconds() / deskInTime.Seconds()))
	}
	if f.change > 0 {
		step := deskOutSpan / time.Duration(max(f.n-1, 1))
		// Never before it is sung: the rest go with the line, as it goes.
		from := max(f.change-deskOutFrom+time.Duration(i)*step, sung)
		out = clamp01(float32((f.pos - from).Seconds() / deskOutTime.Seconds()))
	}
	return in, out
}

// style sets the i-th word, e, sung by sung, where it is in coming and
// going: up into focus out of a blur, and up and apart into one, as smoke.
// What is left of it as the line changes goes with its Exit.
func (f deskFade) style(e ui.Element, i int, sung time.Duration) {
	spread := (float32(i)+0.5)/float32(max(f.n, 1)) - 0.5 // -0.5 at the left, 0.5 at the right
	in, out := f.word(i, sung)
	in, out = deskIn(in), deskOut(out)
	if in < 1 || out > 0 {
		s := 0.9 + 0.1*in + 0.14*out
		e.Opacity(in*(1-out)).Blur(12*(1-in)+14*out).Translate(spread*30*out, 18*(1-in)-22*out).Scale(s, s)
	}
	var enter *ui.Motion
	if !f.enters { // a title, or a line dimmed before it is sung, fades in
		enter = &ui.Motion{Blur: 8}
	}
	e.Transition(ui.ElementTransition{
		Position:     true, // not the words' colors, which change as they are sung
		Duration:     450 * time.Millisecond,
		Ease:         deskOut,
		Enter:        enter,
		Exit:         &ui.Motion{X: spread * 30, Y: -22, Scale: 1.14, Blur: 14},
		ExitDuration: 450 * time.Millisecond,
	})
}

// frames asks for frames at the display's rate as the line comes in or
// goes, and none in between.
func (f deskFade) frames(c *ui.Context) {
	if f.enters && f.pos < f.at+deskInSpan+deskInTime {
		c.AnimationFrame()
	}
	if f.change > 0 {
		if from := f.change - deskOutFrom; f.pos >= from {
			c.AnimationFrame()
		} else {
			c.After(from - f.pos)
		}
	}
}

// deskBit is what a word of the desktop lyrics is, or with i -1 the line
// under them: one that changes comes in, and the one before goes.
type deskBit struct {
	line int
	text string
	i    int
}

// deskIn comes quickly and settles slowly; deskOut starts gently, as a
// word beginning to blow away, and drifts off. Curves, not springs: a
// spring's tail kept asking for frames past a second, over every app.
var deskIn, deskOut = ui.CubicBezier(0.22, 1, 0.36, 1), ui.CubicBezier(0.45, 0, 0.35, 1)

// deskLine returns the line of timed lines to show at pos: the line sung,
// or resting, the line coming, through a break or before the first, or
// after the last, that line. -1 when no line has words.
func deskLine(lines []lyrics.Line, pos time.Duration) (cur int, resting bool) {
	if cur := sungLine(lines, pos); cur >= 0 {
		return cur, false
	}
	i := lyrics.Index(lines, pos)
	for j := i + 1; j < len(lines); j++ {
		if lines[j].Text != "" {
			return j, true
		}
	}
	for j := min(i, len(lines)-1); j >= 0; j-- {
		if lines[j].Text != "" {
			return j, true
		}
	}
	return -1, false
}

// wordColors returns the colors of line l's words, sung by pos in the
// color chosen.
func (d *desk) wordColors(c *ui.Context, l *lyrics.Line, pos time.Duration, playing bool) func(int) ui.Color {
	tint := d.tint()
	// White words sung stand out from those to come only as these dim.
	ahead := white
	if tint == white {
		ahead = white.Alpha(0.5)
	}
	// Words light up smoothly enough at 25 frames a second; sung, they rest.
	// Shown ahead through a break, they wait for the first.
	if playing && pos < l.Words[len(l.Words)-1].End {
		c.After(max(40*time.Millisecond, l.Words[0].Start-pos))
	}
	return func(i int) ui.Color {
		wd := l.Words[i]
		return ahead.Mix(tint, clamp01(float32((pos-wd.Start).Seconds()/max(0.05, (wd.End-wd.Start).Seconds()))))
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
