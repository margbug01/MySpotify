package app

import (
	"cmp"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"vinyl/internal/spotify"
)

// The home page, as Spotify's: chips to narrow it to music or podcasts,
// a grid of what the listener played lately, then Spotify's sections, a
// row of cards each, in its order and under its titles.

// homeFacets are the chips over the home page: all of it, its music, its
// podcasts.
var homeFacets = []struct{ id, label string }{{"", "全部"}, {"music-chip", "音乐"}, {"podcasts-chip", "播客"}}

// What the home page loads, which tests replace.
var (
	homeFeed         = spotify.HomeFeed
	homeSectionItems = spotify.HomeSectionItems
)

// Spotify's green, of its play buttons, which take a black icon.
var (
	spotifyGreen      = ui.Hex("#1ed760")
	spotifyGreenHover = ui.Hex("#3be477")
)

// homeCardMin is how narrow a card of the home page gets, without its
// padding.
const homeCardMin = 150

// shortcutsMax is how many the grid over the sections shows.
const shortcutsMax = 8

func (a *app) homePage(c *ui.Context) {
	if len(a.homeSections) == 0 && !a.homeLoading {
		a.centered(c, a.homeErr, a.loadHome)
		return
	}
	top, bottom := a.insets(c)
	page := ui.Scroll(c.Key("home")).TrackScroll(&a.pageScroll).Grow(1).Padding(top+pageGap, pagePad, bottom+48, pagePad).ScrollbarInsets(top, 0, bottom, 0)
	a.homeWash(c, page, top)
	page.Children(func() {
		a.startMark(c)
		defer a.endMark(c)
		a.homeChips(c)
		if len(a.homeSections) == 0 {
			a.homeSkeleton(c, a.pageShimmer(c))
			return
		}
		sections := a.homeSections
		if a.homeFacet != "" {
			sections = a.facets[a.homeFacet]
			if sections == nil {
				key := "home:" + a.homeFacet
				if err := a.loadErr[key]; err != "" && !a.loading[key] {
					ui.Column(c).Padding(80, 0).Center().Gap(12).Children(func() {
						ui.Text(c, err).TextColor(a.pal.muted).FontSize(13)
						if ui.Button(c, "重试").Clicked() {
							a.loadFacet(a.homeFacet)
						}
					})
					return
				}
				a.shelvesSkeleton(c, a.pageShimmer(c), 3)
				return
			}
		} else {
			a.shortcutGrid(c)
		}
		for i := range sections {
			a.shelf(c.Key(sectionKey(&sections[i])), &sections[i])
		}
	})
}

// homeWash colors the top of page, as Spotify's, with the cover pointed at
// in the grid, else the first's: it fades as the page scrolls. It is the
// page's background, under what scrolls, which an element over it would
// tint, and take the pointer from.
func (a *app) homeWash(c *ui.Context, page ui.Element, top float32) {
	if a.homeTint == (ui.Color{}) {
		return
	}
	tint := page.AnimateColor("tint", a.homeTint, ui.Animation{Duration: 600 * time.Millisecond})
	strength := float32(0.2)
	if a.pal.dark {
		strength = 0.42
	}
	strength *= max(0, 1-a.pageScroll.Y/400)
	_, h := c.Size()
	page.LinearGradient(ui.LinearGradient{Angle: 180, Oklab: true, Stops: []ui.GradientStop{
		{Color: tint.Alpha(strength), At: 0},
		{Color: tint.Alpha(0), At: min(1, (top+340)/max(h, 1))},
	}})
}

// homeChips narrow the home page to music or podcasts, as Spotify's: the
// chip chosen again goes back to all of it.
func (a *app) homeChips(c *ui.Context) {
	p := a.pal
	ui.Row(c).Gap(8).Margin(0, 0, 22, 0).Children(func() {
		for _, f := range homeFacets {
			on := a.homeFacet == f.id
			b := ui.ButtonBase(c.Key(f.label)).Height(32).PaddingX(14).Radius(999).Center().FocusRing(false).Label(f.label)
			bg, fg := p.text.Alpha(0.07), p.text
			switch {
			case on:
				bg, fg = p.text, p.bg
			case b.Hovered():
				bg = p.text.Alpha(0.13)
			}
			b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 140 * time.Millisecond}))
			b.Children(func() {
				ui.Text(c, f.label).FontSize(13).FontWeight(500).TextColor(fg).SingleLine()
			})
			if b.Clicked() {
				a.chooseFacet(f.id)
			}
		}
	})
}

func (a *app) chooseFacet(id string) {
	if id == a.homeFacet {
		id = ""
	}
	a.homeFacet = id
	a.pageScroll = ui.ScrollState{}
	if id != "" && a.facets[id] == nil {
		a.loadFacet(id)
	}
}

// loadFacet loads the home with the chip id chosen.
func (a *app) loadFacet(id string) {
	fetch(a, "home:"+id, "加载失败，请稍后再试", func() (*spotify.Home, error) { return homeFeed(id) }, func(h *spotify.Home) {
		if a.facets == nil {
			a.facets = map[string][]spotify.HomeSection{}
		}
		a.facets[id] = h.Sections
	})
}

// gridItems are what the grid over the sections shows, as Spotify's: what
// the listener played lately, their liked songs and lists, the artists
// they follow, and as long as there is room, the lists of the sections.
func (a *app) gridItems() []spotify.HomeItem {
	var out []spotify.HomeItem
	seen := map[string]bool{}
	add := func(it spotify.HomeItem) {
		if id := it.ID(); len(out) < shortcutsMax && id != "" && !seen[id] {
			seen[id] = true
			out = append(out, it)
		}
	}
	for _, it := range a.homeShortcuts {
		add(it)
	}
	for _, lists := range [][]spotify.Playlist{a.mine, a.saved} {
		for i := range lists {
			add(spotify.HomeItem{List: &lists[i]})
		}
	}
	for i := range a.followed {
		add(spotify.HomeItem{Artist: &a.followed[i]})
	}
	for _, s := range a.homeSections {
		for _, it := range s.Items {
			if it.List != nil {
				add(it)
			}
		}
	}
	return out
}

func (a *app) shortcutGrid(c *ui.Context) {
	items := a.gridItems()
	if len(items) == 0 {
		return
	}
	var tint ui.Color
	ui.Grid(c).Columns(a.shortcutColumns(c)).Gap(8).Margin(0, 0, 8, 0).Children(func() {
		for i := range items {
			if t, hovered := a.shortcutTile(c.Key(items[i].ID()), &items[i]); hovered || i == 0 && tint == (ui.Color{}) {
				tint = cmp.Or(t, tint)
			}
		}
	})
	if tint != (ui.Color{}) {
		a.homeTint = tint
	}
}

// shortcutColumns is how many columns the grid has: four where they fit.
func (a *app) shortcutColumns(c *ui.Context) int {
	w, _ := c.Size()
	if w-sidebarWidth-2*pagePad >= 4*190 {
		return 4
	}
	return 2
}

// shortcutTile is a cover and a name, in a row of the grid: it returns the
// cover's color, once loaded, and whether the pointer is on it.
func (a *app) shortcutTile(c *ui.Context, it *spotify.HomeItem) (ui.Color, bool) {
	name := itemName(it)
	b := ui.ButtonBase(c).Height(56).Radius(6).Gap(12).Padding(0, 10, 0, 0).AlignItems(ui.Center).FocusRing(false).Label(name)
	hovered := b.Hovered()
	bg := a.pal.text.Alpha(0.06)
	if hovered {
		bg = a.pal.text.Alpha(0.12)
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 140 * time.Millisecond}))
	var tint ui.Color
	b.Children(func() {
		ui.Box(c).Size(56, 56).Shrink(0).Radius(6).Shadow(0, 4, 12, -4, a.pal.cardShade).Children(func() {
			tint = a.itemArt(c, it, 120, 6)
		})
		ui.Text(c, name).FontSize(14).FontWeight(700).MaxLines(2).LineHeight(1.3).Grow(1).Shrink(1).MinWidth(0)
		a.greenPlay(c, ui.ButtonBase(c.Key("play")).Shrink(0), it, 32, hovered)
	})
	if b.Clicked() {
		a.openItem(it)
	}
	return tint, hovered
}

// shelf is a section of the home page: its title, and as many of its cards
// as fit across, with a way to see all of them.
func (a *app) shelf(c *ui.Context, s *spotify.HomeSection) {
	cols := a.columns(c, homeCardMin)
	ui.Row(c).AlignItems(ui.End).Gap(16).Margin(26, 0, 6, 0).Children(func() {
		ui.Text(c, s.Title).FontSize(22).FontWeight(700).LetterSpacing(-0.4).SingleLine().Grow(1).Shrink(1).MinWidth(0)
		if max(s.Total, len(s.Items)) > cols && link(c.Key("all"), "显示全部", 13, a.pal.muted, a.pal.text).Clicked() {
			a.goTo(route{page: pageSection, section: sectionKey(s)})
		}
	})
	// The cards' padding reaches past the page's, so that their covers line
	// up with the title.
	ui.Grid(c).Columns(cols).Margin(0, -12, 0, -12).Children(func() {
		for i := range s.Items[:min(cols, len(s.Items))] {
			a.homeCard(c.Key(s.Items[i].ID()), &s.Items[i])
		}
	})
}

// sectionKey names s for its page: its URI, or its title if it has none.
func sectionKey(s *spotify.HomeSection) string { return cmp.Or(s.URI, s.Title) }

// findSection returns the section of the home named key, of all of it or
// with a chip chosen.
func (a *app) findSection(key string) *spotify.HomeSection {
	all := [][]spotify.HomeSection{a.homeSections}
	for _, f := range homeFacets {
		all = append(all, a.facets[f.id])
	}
	for _, sections := range all {
		for i := range sections {
			if sectionKey(&sections[i]) == key {
				return &sections[i]
			}
		}
	}
	return nil
}

// loadSection loads all the cards of the section key, if Spotify lists
// them.
func (a *app) loadSection(key string) {
	if !strings.HasPrefix(key, "spotify:section:") || a.sectionAll[key] != nil {
		return
	}
	fetch(a, key, "加载失败，请稍后再试", func() ([]spotify.HomeItem, error) { return homeSectionItems(key) }, func(items []spotify.HomeItem) {
		if a.sectionAll == nil {
			a.sectionAll = map[string][]spotify.HomeItem{}
		}
		a.sectionAll[key] = items
	})
}

// sectionPage is all of a section's cards, as its "show all" opens them.
func (a *app) sectionPage(c *ui.Context) {
	s := a.findSection(a.route.section)
	if s == nil {
		a.centered(c, "这一栏已经不在首页了", nil)
		return
	}
	items := s.Items
	if all := a.sectionAll[a.route.section]; len(all) > len(items) {
		items = all
	}
	top, bottom := a.insets(c)
	ui.Scroll(c.Key("section")).TrackScroll(&a.pageScroll).Grow(1).Padding(top+pageGap, pagePad, bottom+48, pagePad).ScrollbarInsets(top, 0, bottom, 0).Children(func() {
		a.startMark(c)
		defer a.endMark(c)
		a.heading(c, s.Title, "")
		ui.Grid(c).Columns(a.columns(c, homeCardMin)).GapY(6).Margin(0, -12, 0, -12).Children(func() {
			for i := range items {
				a.homeCard(c.Key(items[i].ID()), &items[i])
			}
		})
	})
}

// homeCard is a card of a section, as Spotify's: a cover, round for an
// artist, its name and a line or two about it; pointed at, its background
// lightens and a green play button rises over the cover.
func (a *app) homeCard(c *ui.Context, it *spotify.HomeItem) {
	p := a.pal
	name := itemName(it)
	card := ui.ButtonBase(c).Column().Gap(4).Padding(12).Radius(8).AlignItems(ui.Stretch).FocusRing(false).Label(name)
	hovered := card.Hovered()
	bg := p.text.Alpha(0)
	if hovered {
		bg = p.text.Alpha(0.06)
	}
	card.Background(card.AnimateColor("bg", bg, ui.Animation{Duration: 160 * time.Millisecond}))
	card.Children(func() {
		radius := float32(6)
		if it.Artist != nil {
			radius = 999
		}
		ui.Box(c).AspectRatio(1).Radius(radius).Margin(0, 0, 6, 0).Shadow(0, 8, 22, -8, p.cardShade).Children(func() {
			a.itemArt(c, it, 400, radius)
			a.greenPlay(c, ui.ButtonBase(c.Key("play")).Absolute().Bottom(8).Right(8), it, 46, hovered)
		})
		ui.Text(c, name).FontSize(14).FontWeight(600).SingleLine()
		if note := a.itemNote(it, c.Now()); note != "" {
			ui.Text(c, note).FontSize(13).TextColor(p.muted).MaxLines(2).LineHeight(1.35)
		}
	})
	if card.Clicked() {
		a.openItem(it)
	}
}

// itemArt fills its parent with the cover of it, and returns the cover's
// color once it has loaded.
func (a *app) itemArt(c *ui.Context, it *spotify.HomeItem, px int, radius float32) ui.Color {
	if it.List != nil && it.List.Liked() {
		likedArt(c, radius, float32(px)/6)
		return ui.Hex("#5038a0")
	}
	url := itemCover(it)
	a.coverFill(c, url, px, radius)
	if art := a.art.get(spotify.Sized(url, px), false); art != nil && !art.failed {
		return art.tint
	}
	return ui.Color{}
}

// likedArt is the cover of the liked songs, as Spotify's: a heart on violet
// fading to mint, the heart icon DIPs across.
func likedArt(c *ui.Context, radius, icon float32) {
	ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Radius(radius).Center().TextColor(ui.RGB(255, 255, 255)).
		LinearGradient(ui.LinearGradient{Angle: 135, Stops: []ui.GradientStop{
			{Color: ui.Hex("#450af5"), At: 0},
			{Color: ui.Hex("#c4efd9"), At: 1},
		}}).
		Children(func() { ui.Icon(c, iconHeartFill).Size(icon, icon) })
}

// greenPlay makes b Spotify's play button, which plays it: it rises into
// view as shown turns true.
func (a *app) greenPlay(c *ui.Context, b ui.Element, it *spotify.HomeItem, size float32, shown bool) {
	b.Size(size, size).Radius(999).Center().FocusRing(false).Label("播放").TextColor(ui.RGB(0, 0, 0)).
		Shadow(0, 8, 16, -6, ui.RGBA(0, 0, 0, 0.35))
	fill := spotifyGreen
	pose := ui.Pose{Fade: 1, Scale: 1, Y: 8}
	if shown || b.Hovered() {
		pose = ui.Pose{Scale: 1}
		if b.Hovered() {
			fill, pose.Scale = spotifyGreenHover, 1.05
		}
	}
	b.Background(fill)
	b.AnimatePose(pose, ui.Snappy)
	b.Children(func() { ui.Icon(c, iconPlay).Size(size*0.38, size*0.38).Margin(0, 0, 0, size*0.04) })
	if b.Clicked() {
		a.playItem(it)
	}
}

func itemName(it *spotify.HomeItem) string {
	switch {
	case it.List != nil:
		return it.List.Name
	case it.Artist != nil:
		return it.Artist.Name
	case it.Episode != nil:
		return it.Episode.Name
	}
	return ""
}

func itemCover(it *spotify.HomeItem) string {
	switch {
	case it.List != nil:
		return it.List.Cover()
	case it.Artist != nil:
		return it.Artist.Avatar
	case it.Episode != nil:
		return it.Episode.Album.PicURL
	}
	return ""
}

// itemNote is the line under a card's name, as Spotify's: who made a list,
// or what it is about, and when an episode came out, and how long it is.
func (a *app) itemNote(it *spotify.HomeItem, now time.Time) string {
	switch {
	case it.Artist != nil:
		return "艺人"
	case it.Episode != nil:
		var parts []string
		if day, err := time.ParseInLocation(time.DateOnly, it.Released, time.Local); err == nil {
			if day.Year() == now.Year() {
				parts = append(parts, fmt.Sprintf("%d月%d日", day.Month(), day.Day()))
			} else {
				parts = append(parts, fmt.Sprintf("%d年%d月%d日", day.Year(), day.Month(), day.Day()))
			}
		}
		if m := (it.Episode.Duration + 30000) / 60000; m >= 60 {
			parts = append(parts, fmt.Sprintf("%d 小时 %d 分钟", m/60, m%60))
		} else if m > 0 {
			parts = append(parts, fmt.Sprintf("%d 分钟", m))
		}
		return strings.Join(parts, " · ")
	case it.List != nil:
		pl := it.List
		switch {
		case pl.Liked():
			return "歌单"
		case strings.HasPrefix(pl.ID, "spotify:playlist:") && strings.TrimSpace(pl.Description) != "":
			return strings.TrimSpace(pl.Description)
		}
		return cmp.Or(pl.Note, pl.Creator.Nickname)
	}
	return ""
}

// openItem opens the page of what it stands for: an episode, its podcast's.
func (a *app) openItem(it *spotify.HomeItem) {
	switch {
	case it.List != nil:
		a.goTo(route{page: pagePlaylist, playlist: it.List.ID})
	case it.Artist != nil:
		a.openArtist(it.Artist.ID)
	case it.Episode != nil && it.Episode.Album.ID != "":
		a.goTo(route{page: pagePlaylist, playlist: it.Episode.Album.ID})
	}
}

// playItem plays what it stands for: an artist, their popular songs.
func (a *app) playItem(it *spotify.HomeItem) {
	switch {
	case it.List != nil:
		a.loadPlaylist(it.List.ID, true)
	case it.Artist != nil:
		a.playArtist(artistURI(it.Artist.ID))
	case it.Episode != nil:
		a.pl.PlayQueue([]spotify.Track{*it.Episode}, 0)
	}
}

// playArtist plays the popular songs of the artist uri, loading their page
// first if it has not.
func (a *app) playArtist(uri string) {
	if pg := a.artists[uri]; pg != nil {
		a.pl.PlayQueue(pg.TopTracks, firstPlayable(pg.TopTracks))
		return
	}
	go func() {
		pg, err := spotify.ArtistDetail(uri)
		a.post(func() {
			if err != nil {
				a.toast = "艺人加载失败"
				return
			}
			a.artists[uri] = pg
			a.pl.PlayQueue(pg.TopTracks, firstPlayable(pg.TopTracks))
		})
	}()
}

// homeSkeleton stands in for the grid and the sections as they load.
func (a *app) homeSkeleton(c *ui.Context, s shimmer) {
	ui.Grid(c.Key("grid")).Columns(a.shortcutColumns(c)).Gap(8).Margin(0, 0, 8, 0).Children(func() {
		for i := range shortcutsMax {
			ui.Row(c.Key(i)).Height(56).Radius(6).Gap(12).AlignItems(ui.Center).Background(a.pal.text.Alpha(0.04)).Children(func() {
				s.bone(c, 6).Size(56, 56)
				s.text(c, 14, 1.3).WidthPercent(boneWidth(i) / 2)
			})
		}
	})
	a.shelvesSkeleton(c, s, 2)
}

// shelvesSkeleton stands in for n sections.
func (a *app) shelvesSkeleton(c *ui.Context, s shimmer, n int) {
	cols := a.columns(c, homeCardMin)
	for k := range n {
		ui.Column(c.Key(k)).Children(func() {
			s.text(c, 22, 1.3).WidthPercent(24).Margin(26, 0, 12, 0)
			ui.Grid(c).Columns(cols).Margin(0, -12, 0, -12).Children(func() {
				for i := range cols {
					ui.Column(c.Key(i)).Padding(12).Gap(8).Children(func() {
						s.bone(c, 6).AspectRatio(1)
						s.text(c, 14, 1.3).WidthPercent(70)
						s.text(c, 13, 1.35).WidthPercent(boneWidth(k*cols + i))
					})
				}
			})
		})
	}
}
