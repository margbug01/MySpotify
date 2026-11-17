package app

import (
	"fmt"
	"runtime"

	"github.com/egoist/mygo/ui"

	"vinyl/internal/l10n"
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
// media goes through Web Audio. On a Mac or Linux it is untried, so its setting
// is hidden.
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
