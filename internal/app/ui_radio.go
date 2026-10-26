package app

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"vinyl/internal/spotify"
)

// Radios play on without end, from a song or an artist, as Spotify's own:
// "radio:<its URI>#<its name>", which the session keeps.

func radioName(uri, label string) string { return "radio:" + uri + "#" + label }

// radioLabel names a radio as the listener knows it.
func radioLabel(name string) string {
	_, label, _ := strings.Cut(name, "#")
	return label + " 电台"
}

// radioTracks loads the songs of Spotify's radio of seed, those that play.
// It blocks, so it runs on a goroutine; tests replace it.
var radioTracks = func(seed string) ([]spotify.Track, error) {
	uri, err := spotify.Radio(seed)
	if err != nil {
		return nil, err
	}
	pl, err := spotify.PlaylistDetail(uri)
	if err != nil {
		return nil, err
	}
	var out []spotify.Track
	for _, t := range pl.Tracks {
		if t.Playable() {
			out = append(out, t)
		}
	}
	return out, nil
}

// radioFeed gives the radio more songs, like the last one queued: on a
// goroutine, to got on the main thread, none if it cannot.
func (a *app) radioFeed() func(got func([]spotify.Track)) {
	return func(got func([]spotify.Track)) {
		q := a.pl.Queue()
		if len(q) == 0 {
			got(nil)
			return
		}
		seed := q[len(q)-1].URI()
		go func() {
			tracks, _ := radioTracks(seed)
			a.post(func() { got(tracks) })
		}()
	}
}

// startRadio plays Spotify's radio of uri, a song's or an artist's, named
// label: from the song playing, if it is the radio's.
func (a *app) startRadio(uri, label string) {
	name := radioName(uri, label)
	if a.pl.Radio() == name {
		a.nowPlaying = true
		return
	}
	a.radioStarting = name
	go func() {
		tracks, err := radioTracks(uri)
		a.post(func() { a.radioLoaded(name, uri, tracks, err) })
	}()
}

// radioLoaded plays the radio name of uri, which loaded tracks, unless
// another was asked for meanwhile.
func (a *app) radioLoaded(name, uri string, tracks []spotify.Track, err error) {
	if a.radioStarting != name {
		return
	}
	a.radioStarting = ""
	if err != nil || len(tracks) == 0 {
		a.toast = "电台暂时无法开启，请稍后再试"
		return
	}
	if cur := a.pl.Current(); cur != nil && cur.URI() == uri {
		a.pl.RadioFrom(name, tracks, a.radioFeed())
	} else {
		a.pl.PlayRadio(name, tracks, a.radioFeed())
	}
	a.toast = "已开启" + radioLabel(name)
}

// exitRadio stops the radio playing, but not the song it plays.
func (a *app) exitRadio() {
	if name := a.pl.Radio(); name != "" {
		a.pl.EndRadio()
		a.toast = "已退出" + radioLabel(name)
	}
}

// radioChip says which radio plays, over the title: a click on it, or on
// its cross, leaves the radio, playing on the song.
func (a *app) radioChip(c *ui.Context, name string) {
	const exit = "退出电台"
	chip := ui.ButtonBase(c.Key("radio-chip")).Gap(5).Padding(3, 6, 3, 8).Margin(0, 0, 4, -8).Radius(999).
		AlignItems(ui.Center).AlignSelf(ui.Start).FocusRing(false).Label(exit).Tooltip(exit)
	bg, fg, cross := ui.Transparent, white.Alpha(0.6), float32(0.5)
	if chip.Hovered() {
		bg, fg, cross = white.Alpha(0.12), white.Alpha(0.85), 1
	}
	chip.Background(chip.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond})).TextColor(fg)
	chip.Children(func() {
		ui.Icon(c, iconRadio).Size(12, 12)
		ui.Text(c, radioLabel(name)).FontSize(11.5).FontWeight(700).LetterSpacing(0.3).SingleLine().Shrink(1).MinWidth(0)
		ui.Icon(c, iconX).Size(11, 11).Opacity(cross)
	})
	if chip.Clicked() {
		a.exitRadio()
	}
}
