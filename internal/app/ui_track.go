package app

import (
	"log"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"vinyl/internal/spotify"
)

// trackDetail loads a track's page; tests replace it.
var trackDetail = spotify.TrackDetail

// loadTrack loads the page of track id, unless it has.
func (a *app) loadTrack(id string) {
	if a.tracks == nil {
		a.tracks, a.loadingTr, a.trErr = map[string]*spotify.TrackPage{}, map[string]bool{}, map[string]string{}
	}
	if a.tracks[id] != nil || a.loadingTr[id] {
		return
	}
	a.loadingTr[id] = true
	delete(a.trErr, id)
	go func() {
		pg, err := trackDetail(id)
		a.post(func() {
			delete(a.loadingTr, id)
			if err != nil {
				log.Printf("track %s: %v", id, err)
				a.trErr[id] = "歌曲加载失败，请稍后再试"
				return
			}
			a.tracks[id] = pg
		})
	}()
}

// trackPage shows a track: who made it and when, songs like it, and its
// first artist's popular releases.
func (a *app) trackPage(c *ui.Context) {
	id := a.route.track
	pg := a.tracks[id]
	if pg == nil {
		if !a.loadingTr[id] {
			a.centered(c, a.trErr[id], func() { a.loadTrack(id) })
			return
		}
		s := a.pageShimmer(c)
		a.trackSkeleton(c, s, true, func(c *ui.Context) { a.playlistHeaderSkeleton(c, s) })
		return
	}
	a.trackRows(c, pg.Recommended, true, func(c *ui.Context) { a.trackHeader(c, pg) }, func(c *ui.Context) { a.trackFooter(c, pg) })
}

// trackHeader is the top of a song's page, as a list's (playlistHeader):
// from the top of the window, on its cover's color fading into the page.
func (a *app) trackHeader(c *ui.Context, pg *spotify.TrackPage) {
	p := a.pal
	tr := &pg.Track
	top, _ := a.insets(c)
	band := a.bleed(c).Padding(top+pageGap+28, pagePad, 30, pagePad)
	tintBand(band, a.pageTint(a.artTint(tr.Album.PicURL, 600)))
	band.Children(func() {
		ui.Row(c).Gap(28).AlignItems(ui.End).Children(func() {
			ui.Box(c).Size(232, 232).Shrink(0).Radius(10).Shadow(0, 18, 44, -10, ui.RGBA(0, 0, 0, 0.4)).Children(func() {
				a.coverFill(c, tr.Album.PicURL, 600, 10)
			})
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
				ui.Text(c, "歌曲").FontSize(12.5).FontWeight(600)
				ui.Text(c, tr.Name).FontSize(a.titleSize(c)).FontWeight(800).LetterSpacing(-1).MaxLines(2).LineHeight(1.12)
				ui.Row(c).AlignItems(ui.Center).Children(func() {
					a.artistLinks(c.Key("by"), tr.Artists, 13, p.muted, p.text)
					if tr.Album.Name != "" {
						ui.Text(c, " · ").FontSize(13).TextColor(p.muted).Shrink(0)
						if link(c.Key("album"), tr.Album.Name, 13, p.muted, p.text).Clicked() && tr.Album.ID != "" {
							a.goTo(route{page: pagePlaylist, playlist: tr.Album.ID})
						}
					}
					var meta []string
					if pg.Year > 0 {
						meta = append(meta, strconv.Itoa(pg.Year))
					}
					meta = append(meta, clock(tr.Length()))
					if pg.Plays > 0 {
						meta = append(meta, plays(float64(pg.Plays))+" 次播放")
					}
					ui.Text(c, " · "+strings.Join(meta, " · ")).FontSize(13).TextColor(p.muted).Shrink(0)
				})
				ui.Row(c).Gap(10).Margin(10, 0, 0, 0).AlignItems(ui.Center).Children(func() {
					if pillButton(c.Key("play"), iconPlay, "播放", p.accent, p.onAccent).Clicked() && tr.Playable() {
						// The track, then songs like it.
						a.pl.PlayQueue(append([]spotify.Track{*tr}, pg.Recommended...), 0)
					}
					label := "电台"
					if a.radioStarting == radioName(tr.URI(), tr.Name) {
						label = "正在开启…"
					}
					if pillButton(c.Key("radio"), iconRadio, label, p.hover.Over(p.bg), p.text).Clicked() {
						a.startRadio(tr.URI(), tr.Name)
					}
					a.likeButton(c.Key("like"), tr, 20, p.muted).Margin(0, 0, 0, 4)
					a.songMenu(c.Key("more"), tr, 18, p.muted)
				})
			})
		})
		// Content is under the bar at the top once the band is.
		a.startMark(c)
	})
	if len(pg.Recommended) > 0 {
		ui.Box(c).PaddingX(12).Children(func() { a.section(c, "推荐") })
	}
}

// trackFooter follows the songs like a track: who made it, and its first
// artist's popular releases.
func (a *app) trackFooter(c *ui.Context, pg *spotify.TrackPage) {
	p := a.pal
	ui.Column(c).PaddingX(12).Children(func() {
		if len(pg.Credits) > 0 {
			a.section(c, "制作人员")
			ui.Column(c).Gap(14).Children(func() {
				for i, cr := range pg.Credits {
					ui.Column(c.Key(i)).Gap(3).AlignItems(ui.Start).Children(func() {
						if cr.Artist == "" {
							ui.Text(c, cr.Name).FontSize(14).FontWeight(500)
						} else if link(c.Key("name"), cr.Name, 14, p.text, p.text).Clicked() {
							a.goTo(route{page: pageArtist, artist: cr.Artist})
						}
						ui.Text(c, strings.Join(cr.Roles, "、")).FontSize(12.5).TextColor(p.muted)
					})
				}
			})
		}
		if len(pg.Popular) > 0 {
			a.section(c, pg.Artist.Name+" 的热门作品")
			cols := a.columns(c, 150)
			ui.Grid(c).Columns(cols).GapX(20).GapY(28).Children(func() {
				for i := range pg.Popular[:min(len(pg.Popular), cols)] {
					a.playlistCard(c.Key(pg.Popular[i].ID), &pg.Popular[i])
				}
			})
		}
	})
}
