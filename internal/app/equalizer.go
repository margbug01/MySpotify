package app

import (
	"fmt"
	"math"
	"runtime"

	"github.com/egoist/mygo/ui"

	"vinyl/internal/l10n"
	"vinyl/internal/player"
)

// eqPresets are the gains of the six bands, in dB, each preset applies.
var eqPresets = []struct {
	name  string
	gains [6]float64
}{
	{"平坦", [6]float64{}},
	{"原声", [6]float64{4, 3, 1, 0, 2, 1}},
	{"低音增强", [6]float64{6, 5, 3, 0, 0, 0}},
	{"低音减弱", [6]float64{-6, -5, -3, 0, 0, 0}},
	{"高音增强", [6]float64{0, 0, 0, 2, 4, 6}},
	{"人声增强", [6]float64{-2, -1, 2, 4, 3, 0}},
	{"摇滚", [6]float64{5, 3, -1, -1, 3, 4}},
	{"流行", [6]float64{-1, 2, 4, 3, 0, -1}},
	{"古典", [6]float64{4, 3, 2, 0, 2, 3}},
	{"舞曲", [6]float64{6, 4, 1, 0, 3, 2}},
	{"深沉", [6]float64{5, 4, 2, 0, -2, -4}},
	{"电子", [6]float64{5, 3, 0, -2, 3, 4}},
	{"嘻哈", [6]float64{5, 4, 1, 2, -1, 2}},
	{"爵士", [6]float64{3, 2, -2, 2, 3, 3}},
	{"拉丁", [6]float64{3, 1, 0, 0, 2, 4}},
	{"响度", [6]float64{6, 3, -1, -1, 2, 5}},
	{"酒廊", [6]float64{-2, 0, 2, 3, 1, -1}},
	{"钢琴", [6]float64{2, 1, 0, 2, 3, 3}},
	{"节奏布鲁斯", [6]float64{5, 7, 2, -2, 3, 3}},
	{"小扬声器", [6]float64{4, 3, 2, 0, -1, -2}},
	{"语音", [6]float64{-4, -2, 2, 4, 3, 0}},
	{"高音减弱", [6]float64{0, 0, 0, -2, -4, -6}},
}

// eqPresetName is the preset whose gains g are, or the custom one.
func eqPresetName(g [6]float64) string {
	for _, p := range eqPresets {
		if p.gains == g {
			return l10n.T(p.name)
		}
	}
	return l10n.T("自定义")
}

// eqHere is whether this system has the equalizer: Windows, where protected
// media goes through Web Audio. On a Mac, WebKit feeds the filters silence,
// songs and podcasts alike, while a tone of the page's own passes them
// (measured 2026-11-19); on Linux it is untried. Its setting is hidden there.
var eqHere = runtime.GOOS == "windows"

// applyEqualizer hands the equalizer's settings to the player.
func (a *app) applyEqualizer() {
	a.pl.SetEqualizer(eqHere && a.settings.EQ, a.settings.EQGains)
}

// eqBandName is a band's frequency as "60 Hz" or "2.4 kHz".
func eqBandName(hz int) string {
	if hz < 1000 {
		return fmt.Sprintf("%d Hz", hz)
	}
	return fmt.Sprintf("%g kHz", float64(hz)/1000)
}

// presetSelect is a drop-down of the equalizer's presets. Its list shows
// eight at a time and scrolls, so it doesn't run over the whole window as
// the theme's does for as many options.
func (a *app) presetSelect(c *ui.Context, chosen *string, labels []string) bool {
	p := a.pal
	sel := ui.SelectBase(c, chosen)
	sel.Trigger.Width(160).Padding(7, 10).Gap(8).Radius(8).Border(1, p.hairline).Background(p.press).
		AlignItems(ui.Center).Justify(ui.SpaceBetween).Label(l10n.T("预设"))
	sel.Trigger.Children(func() {
		ui.Text(c, *chosen).FontSize(13).SingleLine().Shrink(1).MinWidth(0)
		ui.Icon(c, iconDown).Size(14, 14).Shrink(0)
	})
	sel.Popup(func(panel ui.Element) {
		panel.Margin(4, 0, 0, 0).Padding(4).Radius(10).Background(p.bg).Border(1, p.hairline).
			Shadow(0, 6, 20, 0, ui.RGBA(0, 0, 0, 0.25))
		ui.Scroll(c.Key("eq-presets")).MaxHeight(8 * 32).Children(func() {
			for _, l := range labels {
				item := sel.Item(l).Padding(7, 10).Radius(7)
				switch {
				case item.Highlighted():
					item.Background(p.accent).TextColor(white)
				case l == *chosen:
					item.Background(p.press)
				}
				item.Children(func() { ui.Text(c, l).FontSize(13).SingleLine() })
			}
		})
	})
	return sel.Trigger.Changed()
}

// crossfadeLabel is the crossfade of secs seconds as "6 s", or "Off".
func crossfadeLabel(secs float64) string {
	if secs <= 0 {
		return l10n.T("关闭")
	}
	return fmt.Sprintf(l10n.T("%d 秒"), int(secs))
}

// eqDrag is the band of the equalizer's curve being dragged, while one is.
type eqDrag struct {
	band  int
	on    bool
	moved bool // a gain changed since the settings were saved
}

// The curve's room: eqLabelW on the left for the scale, and eqPad above and
// below it.
const (
	eqLabelW = 46
	eqPad    = 16
	eqHeight = 176
)

// eqY is where the gain g, in dB, is in a curve of height h, and eqGain is
// the gain at y, to a whole dB.
func eqY(g, h float32) float32 { return eqPad + (12-g)/24*(h-2*eqPad) }
func eqGain(y, h float32) float64 {
	return math.Round(math.Min(12, math.Max(-12, float64(12-(y-eqPad)/(h-2*eqPad)*24))))
}

// eqGraph is the equalizer as a curve through its six bands, whose points
// are dragged up and down.
func (a *app) eqGraph(c *ui.Context) {
	p, s := a.pal, &a.settings
	ui.Column(c).Padding(12, 16, 16, 16).Gap(6).AlignItems(ui.Stretch).Children(func() {
		g := ui.Box(c.Key("eq-graph")).Height(eqHeight).Label(l10n.T("均衡器曲线")).Cursor(ui.CursorResizeNS)
		// width is the curve's, as last drawn.
		var width float32
		set := func(band int, y float32) {
			if v := eqGain(y, eqHeight); v != s.EQGains[band] {
				s.EQGains[band] = v
				a.eqDrag.moved = true
				// The sound follows as the point is dragged; the settings are
				// kept as it is let go. a.changed() waits for the main thread and
				// does what the equalizer doesn't need (the theme, the notch).
				a.applyEqualizer()
			}
		}
		g.HandleInput(func(ev ui.InputEvent) bool {
			switch ev.Kind {
			case ui.InputPointerDown:
				if ev.Button != 0 {
					return false
				}
				band := min(max(int((ev.X-eqLabelW)/((width-eqLabelW)/float32(len(s.EQGains)))), 0), len(s.EQGains)-1)
				a.eqDrag = eqDrag{band: band, on: true}
				set(band, ev.Y)
				return true
			case ui.InputPointerMove:
				if a.eqDrag.on {
					set(a.eqDrag.band, ev.Y)
					return true
				}
			case ui.InputPointerUp:
				a.eqDrag.on = false
				if a.eqDrag.moved {
					a.eqDrag.moved = false
					s.save()
				}
			}
			return false
		})
		grabbed, band := a.eqDrag.on, a.eqDrag.band
		g.Draw(func(pt *ui.Painter, r ui.Rect) {
			width = r.W
			n := len(s.EQGains)
			top, foot := r.Y+eqPad, r.Y+r.H-eqPad
			col := (r.W - eqLabelW) / float32(n)
			xs, ys := make([]float32, n), make([]float32, n)
			for i, v := range s.EQGains {
				xs[i], ys[i] = r.X+eqLabelW+col*(float32(i)+0.5), r.Y+eqY(float32(v), r.H)
			}
			for _, row := range []struct {
				v    string
				y    float32
				line ui.Color
			}{{"+12 dB", top, p.hairline}, {"0", (top + foot) / 2, p.muted.Alpha(0.5)}, {"-12 dB", foot, p.hairline}} {
				pt.Text(r.X, row.y-6, row.v, 11, p.muted)
				pt.Line(r.X+eqLabelW, row.y, r.X+r.W, row.y, 1, row.line)
			}
			for _, x := range xs {
				pt.Line(x, top, x, foot, 1, p.hairline)
			}
			// A smooth curve through the points, each one's tangent
			// the way from the one before it to the one after.
			along := func(path *ui.Path) {
				path.MoveTo(xs[0], ys[0])
				for i := 0; i+1 < n; i++ {
					before, after := max(i-1, 0), min(i+2, n-1)
					path.CubeTo(xs[i]+(xs[i+1]-xs[before])/6, ys[i]+(ys[i+1]-ys[before])/6,
						xs[i+1]-(xs[after]-xs[i])/6, ys[i+1]-(ys[after]-ys[i])/6, xs[i+1], ys[i+1])
				}
			}
			var area, line ui.Path
			along(&area)
			area.LineTo(xs[n-1], foot).LineTo(xs[0], foot).Close()
			pt.FillPathGradient(&area, ui.LinearGradient{From: p.accent.Alpha(0.5), To: p.accent.Alpha(0), Angle: 180})
			along(&line)
			pt.StrokePath(&line, 3, p.accent)
			for i := range xs {
				rad := float32(5)
				if grabbed && i == band {
					rad = 7
					pt.Text(xs[i]-9, ys[i]-24, fmt.Sprintf("%+.0f", s.EQGains[i]), 12, p.text)
				}
				// Ringed in the accent: white alone is lost on the light theme.
				pt.Fill(ui.Rect{X: xs[i] - rad, Y: ys[i] - rad, W: 2 * rad, H: 2 * rad}, p.accent, rad)
				in := rad - 2
				pt.Fill(ui.Rect{X: xs[i] - in, Y: ys[i] - in, W: 2 * in, H: 2 * in}, white, in)
			}
		})
		ui.Row(c).Padding(0, 0, 0, eqLabelW).Children(func() {
			for i, hz := range player.EQBands {
				// Each band's name is its slider for keys and assistive
				// technology, which the curve, dragged only, is not.
				b := ui.Row(c.Key(eqBandName(hz))).Grow(1).Basis(0).Justify(ui.Center).Focusable().
					Role(ui.RoleSlider).Orientation(ui.Vertical).Label(eqBandName(hz)).Range(-12, 12, s.EQGains[i])
				b.HandleInput(func(ev ui.InputEvent) bool {
					if ev.Kind != ui.InputKeyDown || ev.Mods != 0 {
						return false
					}
					var step float64
					switch ev.Key {
					case ui.KeyUp, ui.KeyRight:
						step = 1
					case ui.KeyDown, ui.KeyLeft:
						step = -1
					default:
						return false
					}
					if v := min(max(s.EQGains[i]+step, -12), 12); v != s.EQGains[i] {
						s.EQGains[i] = v
						a.applyEqualizer()
						s.save()
					}
					return true
				})
				b.Children(func() {
					ui.Text(c, eqBandName(hz)).FontSize(11).TextColor(p.muted)
				})
			}
		})
	})
}
