package app

import (
	"log"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"vinyl/internal/spotify"
)

// artistURI is the URI of the artist id names: a track's artists carry
// their base62 ID, cards their URI.
func artistURI(id string) string {
	if strings.HasPrefix(id, "spotify:") {
		return id
	}
	return "spotify:artist:" + id
}

// openArtist opens the page of the artist id names, once the frame is
// built (goTo).
func (a *app) openArtist(id string) {
	a.goTo(route{page: pageArtist, artist: artistURI(id)})
}

// goTo opens the page of r once the frame is built: opening it at once from
// a link in a list's row would reset the list as it is being built.
func (a *app) goTo(r route) { a.next = &r }

// loadArtist loads the page of the artist uri, unless it has.
func (a *app) loadArtist(uri string) {
	if a.artists[uri] != nil || a.loadingAr[uri] {
		return
	}
	a.loadingAr[uri] = true
	delete(a.arErr, uri)
	go func() {
		pg, err := spotify.ArtistDetail(uri)
		a.post(func() {
			delete(a.loadingAr, uri)
			if err != nil {
				log.Printf("artist %s: %v", uri, err)
				a.arErr[uri] = "艺人加载失败"
				return
			}
			a.artists[uri] = pg
		})
	}()
}

// loadReleases loads all of an artist's releases of kind, in place of the
// few the page came with.
func (a *app) loadReleases(pg *spotify.ArtistPage, kind spotify.ReleaseKind) {
	key := pg.ID + "#" + string(kind)
	if a.loadingRel[key] {
		return
	}
	a.loadingRel[key] = true
	uri, name := pg.ID, pg.Name
	go func() {
		all, err := spotify.ArtistReleases(uri, name, kind)
		a.post(func() {
			delete(a.loadingRel, key)
			pg := a.artists[uri]
			if err != nil || pg == nil {
				if err != nil {
					log.Printf("releases %s: %v", key, err)
					a.toast = "加载失败，请稍后再试"
				}
				return
			}
			switch kind {
			case spotify.KindAlbums:
				pg.Albums = all
			case spotify.KindSingles:
				pg.Singles = all
			case spotify.KindCompilations:
				pg.Compilations = all
			}
		})
	}()
}

// toggleFollow follows the artist of pg, or stops following them, at once
// in the app and then with Spotify, which may refuse: it goes back then.
func (a *app) toggleFollow(pg *spotify.ArtistPage) {
	follow := !pg.Followed
	pg.Followed = follow
	uri := pg.ID
	go func() {
		err := saveToLibrary(uri, follow)
		a.post(func() {
			if err != nil {
				if pg := a.artists[uri]; pg != nil {
					pg.Followed = !follow
				}
				a.toast = "操作失败，请稍后再试"
				return
			}
			if a.profile != nil {
				a.loadMine(a.profile.UserID)
			}
		})
	}()
}

// saveToLibrary follows an artist or not with Spotify; tests replace it.
var saveToLibrary = spotify.Save

func (a *app) artistPage(c *ui.Context) {
	uri := a.route.artist
	pg := a.artists[uri]
	if pg == nil {
		if !a.loadingAr[uri] {
			a.centered(c, a.arErr[uri], func() { a.loadArtist(uri) })
			return
		}
		s := a.pageShimmer(c)
		a.trackSkeleton(c, s, true, func(c *ui.Context) { a.playlistHeaderSkeleton(c, s) })
		return
	}
	pic, px := pg.Banner, 1600
	if pic == "" {
		pic, px = pg.Avatar, 600 // as avatar asks for it, 200 across
	}
	a.tones = a.pageTones(a.artTint(pic, px))
	a.trackRows(c, pg.TopTracks, true,
		func(c *ui.Context) { a.artistHeader(c, pg) },
		func(c *ui.Context) { a.artistFooter(c, pg) })
}

// artistHeader is the top of an artist's page: their banner, if they have
// one, with their name over it, or else their picture beside it; then
// what they can be played or followed with.
func (a *app) artistHeader(c *ui.Context, pg *spotify.ArtistPage) {
	p := a.pal
	var stats []string
	if pg.MonthlyListeners > 0 {
		stats = append(stats, plays(float64(pg.MonthlyListeners))+" 位月听众")
	}
	if pg.Followers > 0 {
		stats = append(stats, plays(float64(pg.Followers))+" 人关注")
	}
	kind := func(color ui.Color) {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "艺人").FontSize(12).FontWeight(600).TextColor(color)
			if pg.Verified {
				ui.Text(c, "已认证").FontSize(10).FontWeight(700).TextColor(color).Padding(1, 6).Radius(999).Border(1, color.Alpha(0.6))
			}
		})
	}
	// As Spotify's: from the top of the window and edge to edge, the banner
	// melting into the page below, or the picture's color over it.
	// What they can be played or followed with, in row.
	actions := func(row ui.Element) {
		row.Gap(10).Children(func() {
			if len(pg.TopTracks) > 0 {
				if pillButton(c.Key("play"), iconPlay, "播放", p.accent, p.onAccent).Clicked() {
					if a.pl.Shuffle() {
						a.pl.ToggleShuffle()
					}
					a.playFrom(spotify.HomeItem{Artist: &pg.ArtistCard}, pg.TopTracks, firstPlayable(pg.TopTracks))
				}
				if pillButton(c.Key("shuffle"), iconShuffle, "随机播放", p.hover.Over(p.bg), p.text).Clicked() {
					if !a.pl.Shuffle() {
						a.pl.ToggleShuffle()
					}
					a.playFrom(spotify.HomeItem{Artist: &pg.ArtistCard}, pg.TopTracks, firstPlayable(pg.TopTracks))
				}
				label := "电台"
				if a.radioStarting == radioName(pg.ID, pg.Name) {
					label = "正在开启…"
				}
				if pillButton(c.Key("radio"), iconRadio, label, p.hover.Over(p.bg), p.text).Clicked() {
					a.startRadio(pg.ID, pg.Name)
				}
			}
			icon, label := iconPlus, "关注"
			if pg.Followed {
				icon, label = iconCheck, "已关注"
			}
			if pillButton(c.Key("follow"), icon, label, p.hover.Over(p.bg), p.text).Clicked() {
				a.toggleFollow(pg)
			}
		})
	}
	top, _ := a.insets(c)
	a.bleed(c).Children(func() {
		if pg.Banner != "" {
			a.artistBanner(c, pg, kind, stats, actions)
			// Content is under the bar at the top once the banner is: the
			// bar's edge covers what is left of it. Till then, the back
			// button and the window's read white over it (view, pageTop).
			a.startMark(c)
			a.overBanner = !a.underTop
			if len(pg.TopTracks) > 0 {
				ui.Box(c).Padding(0, pagePad, 4, pagePad).Children(func() { a.section(c, "热门歌曲") })
			}
			return
		}
		band := ui.Column(c).Padding(top+pageGap+28, pagePad, 4, pagePad)
		tintBand(band, a.tones.head)
		band.Children(func() {
			ui.Row(c).Gap(28).AlignItems(ui.Center).Padding(0, 0, 26, 0).Children(func() {
				a.avatar(c, pg.Avatar, 200, p.hover)
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
					kind(p.text)
					ui.Text(c, pg.Name).FontSize(a.titleSize(c)).FontWeight(800).LetterSpacing(-1).MaxLines(2).LineHeight(1.12)
					if len(stats) > 0 {
						ui.Text(c, strings.Join(stats, " · ")).FontSize(13).TextColor(p.muted)
					}
				})
			})
			actions(ui.Row(c))
			if len(pg.TopTracks) > 0 {
				a.section(c, "热门歌曲")
			}
		})
		// Content is under the bar at the top once the band is.
		a.startMark(c)
	})
}

// bannerMelt is how far below the artist's name their banner goes on,
// melting into the page's tone, with the buttons over it.
const bannerMelt = 110

// artistBanner is an artist's banner, the width of the page from the top
// of the window, their name over it, darkened under them: below it, the
// banner melts into the page's tone, which the page takes all the way
// down (pageWash), so that it ends nowhere.
func (a *app) artistBanner(c *ui.Context, pg *spotify.ArtistPage, kind func(ui.Color), stats []string, actions func(ui.Element)) {
	_, h := c.Size()
	pic := min(max(h*0.42, 300), 440)
	ui.Box(c).Height(pic + bannerMelt).Children(func() {
		a.coverFill(c, pg.Banner, 1600, 0)
		shade := ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0)
		anim := ui.Animation{Duration: 500 * time.Millisecond}
		page := shade.AnimateColor("page", a.tones.page, anim)
		deep := shade.AnimateColor("deep", a.tones.deep, anim)
		// Dark at the top too, under the back button and the window's.
		shade.LinearGradient(ui.LinearGradient{Angle: 180, Oklab: true, Stops: []ui.GradientStop{
			{Color: ui.RGBA(0, 0, 0, 0.3), At: 0},
			{Color: deep.Alpha(0), At: 0.25},
			{Color: deep.Alpha(0.55), At: pic / (pic + bannerMelt)},
			{Color: page, At: 1},
		}})
		ui.Column(c).Absolute().Left(pagePad).Right(pagePad).Bottom(bannerMelt + 12).Gap(6).TextColor(white).Children(func() {
			kind(white.Alpha(0.85))
			ui.Text(c, pg.Name).FontSize(a.titleSize(c) * 1.25).FontWeight(800).LetterSpacing(-1.5).SingleLine()
			if len(stats) > 0 {
				ui.Text(c, strings.Join(stats, " · ")).FontSize(13).TextColor(white.Alpha(0.8))
			}
		})
		// Over the banner as it melts: positioned, after the shade, so that
		// they paint over it and take the pointer.
		actions(ui.Row(c).Absolute().Left(pagePad).Bottom(38))
	})
}

// artistFooter follows an artist's top tracks: their releases, artists
// like them, and who they are.
func (a *app) artistFooter(c *ui.Context, pg *spotify.ArtistPage) {
	p := a.pal
	ui.Column(c).PaddingX(12).Children(func() {
		cols := a.columns(c, 168)
		cards := func(list []spotify.Playlist) {
			ui.Grid(c).Columns(cols).GapX(20).GapY(28).Children(func() {
				for i := range list {
					a.playlistCard(c.Key(list[i].ID), &list[i])
				}
			})
		}
		if len(pg.Popular) > 0 {
			a.section(c, "热门作品")
			cards(pg.Popular)
		}
		for _, s := range []struct {
			rs    *spotify.Releases
			title string
		}{{&pg.Albums, "专辑"}, {&pg.Singles, "单曲和 EP"}, {&pg.Compilations, "合辑"}} {
			if len(s.rs.Items) == 0 {
				continue
			}
			ui.Row(c.Key(s.title)).AlignItems(ui.Center).Margin(34, 0, 14, 0).Children(func() {
				ui.Text(c, s.title).FontSize(19).FontWeight(700).LetterSpacing(-0.3).Grow(1)
				if more := s.rs.Total - len(s.rs.Items); more > 0 {
					label := "显示全部 " + plays(float64(s.rs.Total))
					if a.loadingRel[pg.ID+"#"+string(s.rs.Kind)] {
						label = "正在加载…"
					}
					if ui.Button(c.Key("all"), label).Clicked() {
						a.loadReleases(pg, s.rs.Kind)
					}
				}
			})
			cards(s.rs.Items)
		}
		if len(pg.Related) > 0 {
			a.section(c, "相似艺人")
			ui.Grid(c).Columns(a.columns(c, 150)).GapX(20).GapY(24).Children(func() {
				for i := range pg.Related {
					a.artistCard(c.Key(pg.Related[i].ID), &pg.Related[i])
				}
			})
		}
		if bio := strings.TrimSpace(pg.Bio); bio != "" {
			a.section(c, "简介")
			ui.Text(c, bio).FontSize(13).TextColor(p.muted).LineHeight(1.6).MaxLines(8)
		}
	})
}

// artistCard is an artist's round picture with their name below: a click
// opens their page.
func (a *app) artistCard(c *ui.Context, ar *spotify.ArtistCard) {
	p := a.pal
	card := ui.ButtonBase(c).Column().Gap(10).AlignItems(ui.Stretch).FocusRing(false).Label(ar.Name)
	hovered := card.Hovered()
	card.Children(func() {
		art := ui.Box(c).AspectRatio(1).Radius(999)
		lift := ui.Pose{}
		if hovered {
			lift = ui.Pose{Y: -3}
			art.Shadow(0, 12, 28, -6, p.cardShade)
		} else {
			art.Shadow(0, 2, 8, -2, p.cardShade)
		}
		art.AnimatePose(lift, ui.Snappy)
		art.Children(func() { a.coverFill(c, ar.Avatar, 320, 999) })
		ui.Column(c).Gap(2).AlignItems(ui.Center).Children(func() {
			ui.Text(c, ar.Name).FontSize(13).FontWeight(500).SingleLine()
			ui.Text(c, "艺人").FontSize(12).TextColor(p.muted)
		})
	})
	if card.Clicked() {
		a.openArtist(ar.ID)
	}
}

// artistRow lists artists as cards, as many as fit across, in one row.
func (a *app) artistRow(c *ui.Context, list []spotify.ArtistCard) {
	cols := a.columns(c, 150)
	ui.Grid(c).Columns(cols).GapX(20).Children(func() {
		for i := range list[:min(len(list), cols)] {
			a.artistCard(c.Key(list[i].ID), &list[i])
		}
	})
}

// link is text that opens something when clicked, brighter under the
// pointer, as an artist's name or an album's in a line of credits.
func link(c *ui.Context, text string, size float32, color, hover ui.Color) ui.Element {
	b := ui.ButtonBase(c).FocusRing(false).Label(text).Shrink(1).MinWidth(0).Cursor(ui.CursorPointer)
	h := b.Hovered()
	b.Children(func() {
		t := ui.Text(c, text).FontSize(size).SingleLine()
		if h {
			t.TextColor(hover).Underline()
		} else {
			t.TextColor(color)
		}
	})
	return b
}

// artistLinks names artists, each a link to their page.
func (a *app) artistLinks(c *ui.Context, artists []spotify.Artist, size float32, color, hover ui.Color) ui.Element {
	return ui.Row(c).Shrink(1).MinWidth(0).AlignItems(ui.Center).Children(func() {
		for i, ar := range artists {
			if i > 0 {
				// A trailing space would be trimmed: the gap is a margin.
				ui.Text(c, ",").FontSize(size).TextColor(color).Shrink(0).Margin(0, size*0.3, 0, 0)
			}
			if ar.ID == "" {
				ui.Text(c, ar.Name).FontSize(size).TextColor(color).SingleLine().Shrink(1).MinWidth(0)
				continue
			}
			if link(c.Key(i), ar.Name, size, color, hover).Clicked() {
				a.openArtist(ar.ID)
			}
		}
	})
}

// followedArtists lists the artists the listener follows, in the sidebar.
func (a *app) followedArtists(c *ui.Context) {
	if len(a.followed) == 0 {
		return
	}
	p := a.pal
	ui.Text(c, "关注的艺人").FontSize(11.5).FontWeight(600).TextColor(p.faint).Margin(18, 4, 6, 10)
	for i := range a.followed {
		ar := &a.followed[i]
		selected := a.route.page == pageArtist && a.route.artist == ar.ID && !a.nowPlaying
		b := ui.ButtonBase(c.Key(ar.ID)).Height(34).PaddingX(8).Gap(10).Radius(8).AlignItems(ui.Center).Justify(ui.Start).FocusRing(false).Label(ar.Name)
		switch {
		case selected:
			b.Background(p.press)
		case b.Hovered():
			b.Background(b.AnimateColor("bg", p.hover, ui.Animation{Duration: 100 * time.Millisecond}))
		}
		b.Children(func() {
			a.avatar(c, ar.Avatar, 22, p.press)
			ui.Text(c, ar.Name).FontSize(13).SingleLine().Shrink(1).MinWidth(0)
		})
		if b.Clicked() {
			a.openArtist(ar.ID)
		}
	}
}
