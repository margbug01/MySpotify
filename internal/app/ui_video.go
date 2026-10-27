package app

import (
	"time"

	"github.com/egoist/mygo/ui"
)

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

// npVideo shows the music video where the cover goes: as wide as the
// column, as tall as the picture, in the cover's place.
func (a *app) npVideo(c *ui.Context, width, size float32) {
	ui.Column(c.Key("mv-place")).Size(width, size).Justify(ui.Center).AlignSelf(ui.Center).Children(func() {
		ui.WebView(c, a.pl.Video.View()).Size(width, width*9/16).Radius(10).Background(ui.Hex("#000000")).Label("MV")
	})
}
