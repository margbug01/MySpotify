package app

import (
	"cmp"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"vinyl/internal/player"
	"vinyl/internal/spotify"
)

const pagePad = 40

// columns returns how many cards of at least min DIPs fit across the page.
func (a *app) columns(c *ui.Context, min float32) int {
	w, _ := c.Size()
	inner := w - sidebarWidth - 2*pagePad
	return max(2, int((inner+20)/(min+20)))
}

func (a *app) heading(c *ui.Context, title, sub string) {
	ui.Column(c).Gap(4).Margin(0, 0, 18, 0).Children(func() {
		if sub != "" {
			ui.Text(c, sub).FontSize(12).FontWeight(600).TextColor(a.pal.muted).LetterSpacing(0.4)
		}
		ui.Text(c, title).FontSize(30).FontWeight(700).LetterSpacing(-0.6)
	})
}

func (a *app) section(c *ui.Context, title string) {
	ui.Text(c, title).FontSize(19).FontWeight(700).LetterSpacing(-0.3).Margin(34, 0, 14, 0)
}

func weekday(t time.Time) string {
	return [...]string{"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"}[t.Weekday()]
}

func (a *app) chartsPage(c *ui.Context) {
	loading := len(a.charts) == 0
	if loading && !a.homeLoading {
		a.centered(c, a.homeErr, a.loadHome)
		return
	}
	top, bottom := a.insets(c)
	ui.Scroll(c.Key("charts")).TrackScroll(&a.pageScroll).Grow(1).Padding(top+pageGap, pagePad, bottom+48, pagePad).ScrollbarInsets(top, 0, bottom, 0).Children(func() {
		a.startMark(c)
		defer a.endMark(c)
		a.heading(c, "排行榜", "Spotify 官方榜单")
		if loading {
			s := a.pageShimmer(c)
			a.chartCardsSkeleton(c, s)
			a.section(c, "全部榜单")
			a.playlistCardsSkeleton(c, s, a.columns(c, 150)*2, 150, true)
			return
		}
		official := a.charts[:min(4, len(a.charts))]
		cols := min(4, max(2, a.columns(c, 260)))
		ui.Grid(c).Columns(cols).Gap(20).Children(func() {
			for i := range official {
				a.chartCard(c.Key(official[i].ID), &official[i])
			}
		})
		if len(a.charts) > 4 {
			a.section(c, "全部榜单")
			rest := a.charts[4:]
			ui.Grid(c).Columns(a.columns(c, 150)).GapX(20).GapY(28).Children(func() {
				for i := range rest {
					a.playlistCard(c.Key(rest[i].ID), &rest[i])
				}
			})
		}
	})
}

// playlistCard is a square cover with the playlist's name below: it lifts
// under the pointer, where a play button shows on it.
func (a *app) playlistCard(c *ui.Context, pl *spotify.Playlist) {
	p := a.pal
	card := ui.ButtonBase(c).Column().Gap(10).AlignItems(ui.Stretch).FocusRing(false).Label(pl.Name)
	hovered := card.Hovered()
	card.Children(func() {
		art := ui.Box(c).AspectRatio(1).Radius(10)
		lift := ui.Pose{}
		if hovered {
			lift = ui.Pose{Y: -3}
			art.Shadow(0, 12, 28, -6, p.cardShade)
		} else {
			art.Shadow(0, 2, 8, -2, p.cardShade)
		}
		art.AnimatePose(lift, ui.Snappy)
		art.Children(func() {
			a.coverFill(c, pl.Cover(), 400, 10)
			a.coverPlayButton(c, pl.ID, hovered)
		})
		ui.Text(c, pl.Name).FontSize(13).FontWeight(500).MaxLines(2).LineHeight(1.35)
		if note := cmp.Or(pl.Note, pl.Creator.Nickname); note != "" {
			ui.Text(c, note).FontSize(12).TextColor(p.muted).SingleLine().Margin(-6, 0, 0, 0)
		}
	})
	if card.Clicked() {
		a.goTo(route{page: pagePlaylist, playlist: pl.ID})
	}
}

// coverFill fills its parent with a cover, its corners rounded by radius.
// The cover rounds itself rather than its parent clipping it: a clip moved
// by a transform, as a card lifting under the pointer, is drawn on the CPU.
func (a *app) coverFill(c *ui.Context, url string, px int, radius float32) {
	bmp := bitmap(a.art.get(spotify.Sized(url, px), false))
	box := ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Radius(radius)
	in := artIn(box, bmp)
	if in < 1 {
		box.Background(a.pal.hover)
	}
	box.Children(func() { artImage(c, bmp, radius, in) })
}

// coverPlayButton plays a playlist from its card, without opening it.
func (a *app) coverPlayButton(c *ui.Context, id string, shown bool) {
	b := ui.ButtonBase(c.Key("play")).Absolute().Bottom(10).Right(10).Size(38, 38).Radius(999).Center().
		Background(a.pal.accent).TextColor(ui.RGB(255, 255, 255)).Label("播放").FocusRing(false).
		Shadow(0, 4, 12, 0, ui.RGBA(0, 0, 0, 0.3))
	pose := ui.Pose{Fade: 1, Scale: 0.8, Y: 6}
	if shown || b.Hovered() {
		pose = ui.Pose{Scale: 1}
		if b.Hovered() {
			pose.Scale = 1.08
		}
	}
	b.AnimatePose(pose, ui.Snappy)
	b.Children(func() { ui.Icon(c, iconPlay).Size(15, 15).Margin(0, 0, 0, 2) })
	if b.Clicked() {
		a.loadPlaylist(id, true)
	}
}

// chartCard shows a chart with its top three songs beside its cover.
func (a *app) chartCard(c *ui.Context, pl *spotify.Playlist) {
	p := a.pal
	card := ui.ButtonBase(c).Gap(14).Padding(12).Radius(14).AlignItems(ui.Center).FocusRing(false).Label(pl.Name)
	hovered := card.Hovered()
	bg := p.hover
	if hovered {
		bg = p.press
	}
	card.Background(card.AnimateColor("bg", bg, ui.Animation{Duration: 140 * time.Millisecond}))
	card.Children(func() {
		ui.Box(c).Size(84, 84).Radius(9).Shadow(0, 3, 10, -2, p.cardShade).Children(func() {
			a.coverFill(c, pl.Cover(), 200, 9)
		})
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(5).Children(func() {
			ui.Text(c, pl.Name).FontSize(14).FontWeight(700).SingleLine()
			for i, line := range pl.Preview {
				ui.Row(c).Gap(7).Children(func() {
					ui.Textf(c, "%d", i+1).FontSize(12).FontWeight(700).Width(10).FontFeatures("tnum").TextColor(p.muted)
					ui.RichText(c,
						ui.Span{Text: line.First, Weight: 500},
						ui.Span{Text: "  " + line.Second, Color: p.muted},
					).FontSize(12).SingleLine().Shrink(1).MinWidth(0)
				})
			}
		})
	})
	if card.Clicked() {
		a.open(route{page: pagePlaylist, playlist: pl.ID})
	}
}

func (a *app) playlistPage(c *ui.Context) {
	id := a.route.playlist
	pl := a.playlists[id]
	if pl == nil {
		if !a.loadingPl[id] {
			a.centered(c, a.plErr[id], func() { a.loadPlaylist(id, false) })
			return
		}
		s := a.pageShimmer(c)
		a.trackSkeleton(c, s, false, func(c *ui.Context) { a.playlistHeaderSkeleton(c, s) })
		return
	}
	a.trackRows(c, pl.Tracks, false, func(c *ui.Context) { a.playlistHeader(c, pl) }, nil)
}

// playlistHeader is the top of a list's page, as Spotify's: from the top of
// the window and edge to edge, on its cover's color fading into the page.
func (a *app) playlistHeader(c *ui.Context, pl *spotify.Playlist) {
	top, _ := a.insets(c)
	tint := likedTint
	if !pl.Liked() {
		tint = a.artTint(pl.Cover(), 600)
	}
	band := a.bleed(c).Padding(top+pageGap+28, pagePad, 30, pagePad)
	tintBand(band, a.pageTint(tint))
	band.Children(func() {
		a.playlistTop(c, pl)
		// Content is under the bar at the top once the band is.
		a.startMark(c)
	})
}

func (a *app) playlistTop(c *ui.Context, pl *spotify.Playlist) {
	p := a.pal
	ui.Row(c).Gap(28).AlignItems(ui.End).Children(func() {
		ui.Box(c).Size(232, 232).Shrink(0).Radius(10).Shadow(0, 18, 44, -10, ui.RGBA(0, 0, 0, 0.4)).Children(func() {
			a.itemArt(c, &spotify.HomeItem{List: pl}, 600, 10)
		})
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
			kind, unit := "歌单", "首"
			switch {
			case strings.HasPrefix(pl.ID, "spotify:album:"):
				kind = "专辑"
			case strings.HasPrefix(pl.ID, "spotify:show:"):
				kind, unit = "播客", "集"
			}
			ui.Text(c, kind).FontSize(12.5).FontWeight(600)
			ui.Text(c, pl.Name).FontSize(a.titleSize(c)).FontWeight(800).LetterSpacing(-1).MaxLines(2).LineHeight(1.12)
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				if len(pl.Artists) > 0 {
					a.artistLinks(c.Key("by"), pl.Artists, 13, p.muted, p.text)
					ui.Text(c, " · ").FontSize(13).TextColor(p.muted)
				} else if pl.Owner != "" {
					if link(c.Key("owner"), pl.Creator.Nickname, 13, p.muted, p.text).Clicked() {
						a.goTo(route{page: pageUser, user: pl.Owner})
					}
					ui.Text(c, " · ").FontSize(13).TextColor(p.muted).Shrink(0)
				} else if pl.Creator.Nickname != "" {
					ui.Text(c, pl.Creator.Nickname+" · ").FontSize(13).TextColor(p.muted)
				}
				n := len(pl.Tracks)
				if unit == "集" {
					// The latest episodes list; the podcast has them all.
					n = max(n, pl.TrackCount)
				}
				ui.Text(c, fmt.Sprintf("%d %s", n, unit)).FontSize(13).TextColor(p.muted).Shrink(0)
			})
			if d := strings.TrimSpace(pl.Description); d != "" {
				ui.Text(c, strings.ReplaceAll(d, "\n", " ")).FontSize(12.5).TextColor(p.muted).MaxLines(2).LineHeight(1.45).Margin(2, 0, 0, 0)
			}
			ui.Row(c).Gap(10).Margin(10, 0, 0, 0).Children(func() {
				if pillButton(c.Key("play"), iconPlay, "播放", p.accent, p.onAccent).Clicked() {
					if a.pl.Shuffle() {
						a.pl.ToggleShuffle()
					}
					a.playFrom(spotify.HomeItem{List: pl}, pl.Tracks, firstPlayable(pl.Tracks))
				}
				if pillButton(c.Key("shuffle"), iconShuffle, "随机播放", p.hover.Over(p.bg), p.text).Clicked() {
					if !a.pl.Shuffle() {
						a.pl.ToggleShuffle()
					}
					a.playFrom(spotify.HomeItem{List: pl}, pl.Tracks, firstPlayable(pl.Tracks))
				}
				if a.profile != nil && !pl.Liked() && a.editable(pl.ID) == nil {
					icon, label := iconPlus, "收藏"
					if a.isSaved(pl.ID) {
						icon, label = iconCheck, "已收藏"
					}
					if pillButton(c.Key("save"), icon, label, p.hover.Over(p.bg), p.text).Clicked() {
						a.toggleSaved(pl)
					}
				}
				if mine := a.editable(pl.ID); mine != nil {
					more := iconButton(c.Key("more"), iconMore, 18, p.muted, "更多")
					more.Menu(func(m *ui.Menu) {
						if m.Item("重命名…").Chosen() {
							a.startRename(mine)
						}
						m.Separator()
						if m.Item("删除歌单…").Chosen() {
							a.confirmDelete(mine)
						}
					})
				}
			})
		})
	})
}

// bleed is a column that reaches past the list's padding to the top of the
// window and its edges, under the bars, for a page's header, as Spotify's.
func (a *app) bleed(c *ui.Context) ui.Element {
	top, _ := a.insets(c)
	return ui.Column(c).Margin(-(top + pageGap), -(pagePad - 12), 0, -(pagePad - 12))
}

// tintBand paints el with tint fading down its height into the page.
func tintBand(el ui.Element, tint ui.Color) {
	t := el.AnimateColor("tint", tint, ui.Animation{Duration: 500 * time.Millisecond})
	el.LinearGradient(ui.LinearGradient{Angle: 180, Oklab: true, Stops: []ui.GradientStop{
		{Color: t, At: 0},
		{Color: t.Alpha(0.7), At: 0.6},
		{Color: t.Alpha(0), At: 1},
	}})
}

// pageTint is the color a page's header takes from its picture's, base:
// deep, which white reads over, on a dark page, and pale on a light one;
// gray for a gray picture, or one still loading.
func (a *app) pageTint(base ui.Color) ui.Color {
	h, s, _ := hsv(float64(base.R)/255, float64(base.G)/255, float64(base.B)/255)
	if s < 0.12 {
		s = 0
	}
	s = min(s, 0.65)
	if a.pal.dark {
		return hsvColor(h, s*0.85, 0.32)
	}
	return hsvColor(h, s*0.3, 0.95)
}

// titleSize is how large the name over a page's header is, as wide as the
// page lets it.
func (a *app) titleSize(c *ui.Context) float32 {
	w, _ := c.Size()
	return min(max((w-sidebarWidth)*0.06, 36), 64)
}

func (a *app) searchPage(c *ui.Context) {
	if len(a.results) == 0 && a.searching {
		a.trackSkeleton(c, a.pageShimmer(c), true, func(c *ui.Context) {
			a.heading(c, "“"+a.searched+"”", "搜索")
		})
		return
	}
	if len(a.results) == 0 {
		msg := a.searchErr
		if msg == "" {
			msg = "没有找到相关歌曲"
		}
		top, bottom := a.insets(c)
		ui.Column(c).Grow(1).Padding(top+pageGap, pagePad, bottom, pagePad).Children(func() {
			a.heading(c, "“"+a.searched+"”", "搜索")
			a.centered(c, msg, nil)
		})
		return
	}
	a.trackRows(c, a.results, true, func(c *ui.Context) {
		a.heading(c, "“"+a.searched+"”", "搜索")
		if len(a.found)+len(a.foundAlbums)+len(a.foundLists)+len(a.foundShows) == 0 {
			return
		}
		ui.Column(c).PaddingX(12).Margin(0, 0, 8, 0).Children(func() {
			title := func(s string) {
				ui.Text(c, s).FontSize(19).FontWeight(700).LetterSpacing(-0.3).Margin(4, 0, 14, 0)
			}
			if len(a.found) > 0 {
				title("艺人")
				a.artistRow(c, a.found)
				ui.Box(c).Height(26)
			}
			// A row of each, the best: the rest a search more precise finds.
			cols := a.columns(c, 150)
			for _, s := range []struct {
				name  string
				lists []spotify.Playlist
			}{{"专辑", a.foundAlbums}, {"歌单", a.foundLists}, {"播客", a.foundShows}} {
				if len(s.lists) == 0 {
					continue
				}
				title(s.name)
				ui.Grid(c.Key(s.name)).Columns(cols).GapX(20).Margin(0, 0, 26, 0).Children(func() {
					for i := range s.lists[:min(len(s.lists), cols)] {
						a.playlistCard(c.Key(s.lists[i].ID), &s.lists[i])
					}
				})
			}
			ui.Text(c, "歌曲").FontSize(19).FontWeight(700).LetterSpacing(-0.3).Margin(4, 0, 6, 0)
		})
	}, nil)
}

// trackRows lists tracks under a header, and over a footer if any, which
// scroll with them; a click plays the list from the track clicked.
func (a *app) trackRows(c *ui.Context, tracks []spotify.Track, covers bool, header, footer func(c *ui.Context)) {
	p := a.pal
	cur := a.pl.Current()
	w, _ := c.Size()
	wide := w-sidebarWidth > 820
	top, bottom := a.insets(c)
	// Rows out of view are not built, their marks with them: a list
	// scrolled away from its start or end has content under the bars.
	a.underTop, a.underBottom = true, true
	// The playlist the tracks are listed in, if any, which they can be
	// taken out of.
	var pid string
	if a.route.page == pagePlaylist {
		pid = a.route.playlist
	}
	n := len(tracks) + 1
	if footer != nil {
		n++
	}
	ui.List(c.Key(a.route), &a.trackList, n, func(i int) {
		if i == 0 {
			ui.Column(c).Children(func() {
				a.startMark(c)
				header(c)
			})
			return
		}
		if i == len(tracks)+1 {
			ui.Column(c).Children(func() {
				footer(c)
				a.endMark(c)
			})
			return
		}
		if i == len(tracks) && footer == nil {
			defer a.endMark(c)
		}
		i--
		tr := &tracks[i]
		playing := cur != nil && cur.ID == tr.ID
		row := ui.ButtonBase(c.Key(tr.ID)).Height(54).PaddingX(12).Gap(14).Radius(10).AlignItems(ui.Center).FocusRing(false).Label(tr.Name).
			ContextMenu(a.songItems(tr, pid))
		hovered := row.Hovered()
		bg := ui.Transparent
		switch {
		case playing:
			bg = p.accent.Alpha(0.09)
		case hovered:
			bg = p.hover
		}
		row.Background(row.AnimateColor("bg", bg, ui.Animation{Duration: 100 * time.Millisecond}))
		if !tr.Playable() {
			row.Opacity(0.42)
		}
		row.Children(func() {
			ui.Box(c).Width(26).Center().Children(func() {
				switch {
				case playing:
					a.nowPlayingBars(c)
				case hovered && tr.Playable():
					ui.Icon(c, iconPlay).Size(13, 13).TextColor(p.text)
				default:
					ui.Textf(c, "%d", i+1).FontSize(13).FontFeatures("tnum").TextColor(p.faint)
				}
			})
			if covers {
				a.cover(c, tr.Album.PicURL, 96, 38, 6)
			}
			ui.Column(c).Grow(3).Basis(0).MinWidth(0).Gap(3).Children(func() {
				name := p.text
				if playing {
					name = p.accent
				}
				ui.Text(c, tr.Name).FontWeight(500).TextColor(name).FontSize(14).SingleLine()
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					if !tr.Playable() {
						ui.Text(c, "不可播放").FontSize(9).FontWeight(800).TextColor(p.accent).Padding(0, 4).Radius(3).Border(1, p.accent.Alpha(0.6))
					}
					a.artistLinks(c.Key("by"), tr.Artists, 12.5, p.muted, p.text)
				})
			})
			if wide {
				ui.Row(c).Grow(2).Basis(0).MinWidth(0).Children(func() {
					if tr.Album.ID == "" {
						ui.Text(c, tr.Album.Name).FontSize(12.5).TextColor(p.muted).SingleLine()
					} else if link(c.Key("album"), tr.Album.Name, 12.5, p.muted, p.text).Clicked() {
						a.goTo(route{page: pagePlaylist, playlist: tr.Album.ID})
					}
				})
			}
			// A liked song's heart shows; the others' as the pointer is on them.
			ui.Box(c).Size(32, 32).Center().Children(func() {
				if a.liked[tr.ID] || hovered {
					a.likeButton(c.Key("like"), tr, 15, p.muted)
				}
			})
			ui.Text(c, clock(tr.Length())).FontSize(12.5).FontFeatures("tnum").TextColor(p.faint).Width(44).TextAlign(ui.End)
		})
		if row.Clicked() && tr.Playable() {
			a.playFrom(a.routeItem(), tracks, i)
		}
	}).Grow(1).Padding(top+pageGap, pagePad-12, bottom+32, pagePad-12).ScrollbarInsets(top, 0, bottom, 0)
}

// nowPlayingBars dance in the row of the track playing.
func (a *app) nowPlayingBars(c *ui.Context) {
	color := a.pal.accent
	ui.Box(c).Size(16, 14).Draw(func(p *ui.Painter, r ui.Rect) {
		lv, live := a.pl.Levels()
		a.rowBars.step(p.Now(), lv, live)
		drawBars(p, r, &a.rowBars, 4, color, 2.5)
		if live || a.rowBars.group(0, player.Bands) > 0.01 {
			p.AnimationFrame()
		}
	})
}
