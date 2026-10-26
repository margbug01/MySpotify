package app

import (
	"cmp"
	"fmt"
	"log"
	"strings"

	"github.com/egoist/mygo/ui"

	"vinyl/internal/spotify"
)

// Browsing: the tiles of genres, moods and podcasts, the page of each, and
// people's profiles. Tests replace these.
var (
	categories     = spotify.Categories
	categoryDetail = spotify.CategoryDetail
	userProfile    = spotify.UserProfile
)

// fetch loads what get gives, once, unless key is loading or loaded, and
// gives it to keep on the main thread; failing, the page says msg.
func fetch[T any](a *app, key, msg string, get func() (T, error), keep func(T)) {
	if a.loading == nil {
		a.loading, a.loadErr = map[string]bool{}, map[string]string{}
	}
	if a.loading[key] {
		return
	}
	a.loading[key] = true
	delete(a.loadErr, key)
	go func() {
		v, err := get()
		a.post(func() {
			delete(a.loading, key)
			if err != nil {
				log.Printf("%s: %v", key, err)
				a.loadErr[key] = msg
				return
			}
			keep(v)
		})
	}()
}

func (a *app) loadCategories() {
	if len(a.cats) > 0 {
		return
	}
	fetch(a, "categories", "分类加载失败，请稍后再试", categories, func(c []spotify.Category) { a.cats = c })
}

func (a *app) loadCategory(uri string) {
	if a.catPages[uri] != nil {
		return
	}
	fetch(a, "category:"+uri, "分类加载失败，请稍后再试", func() (*spotify.CategoryPage, error) { return categoryDetail(uri) },
		func(pg *spotify.CategoryPage) {
			if a.catPages == nil {
				a.catPages = map[string]*spotify.CategoryPage{}
			}
			a.catPages[uri] = pg
		})
}

func (a *app) loadUser(id string) {
	if a.users[id] != nil {
		return
	}
	fetch(a, "user:"+id, "个人主页加载失败，请稍后再试", func() (*spotify.UserPage, error) { return userProfile(id) },
		func(pg *spotify.UserPage) {
			if a.users == nil {
				a.users = map[string]*spotify.UserPage{}
			}
			a.users[id] = pg
		})
}

// scrollPage is a page that scrolls under the bars, its content built by
// content.
func (a *app) scrollPage(c *ui.Context, key string, content func()) {
	top, bottom := a.insets(c)
	ui.Scroll(c.Key(key)).TrackScroll(&a.pageScroll).Grow(1).Padding(top+pageGap, pagePad, bottom+48, pagePad).ScrollbarInsets(top, 0, bottom, 0).Children(func() {
		a.startMark(c)
		defer a.endMark(c)
		content()
	})
}

// waiting shows what stands for a page while key loads, or why it did
// not: true if the page is not there to show.
func (a *app) waiting(c *ui.Context, key string, there bool, retry func()) bool {
	switch {
	case there:
		return false
	case a.loading[key]:
		s := a.pageShimmer(c)
		a.scrollPage(c, key, func() {
			s.text(c, 30, 1.2).Width(220).Margin(0, 0, 24, 0)
			a.playlistCardsSkeleton(c, s, a.columns(c, 168)*2, 168, true)
		})
	default:
		a.centered(c, cmp.Or(a.loadErr[key], "暂时没有内容"), retry)
	}
	return true
}

func (a *app) browsePage(c *ui.Context) {
	if a.waiting(c, "categories", len(a.cats) > 0, a.loadCategories) {
		return
	}
	a.scrollPage(c, "browse", func() {
		a.heading(c, "浏览全部", "")
		a.tiles(c, a.cats)
	})
}

func (a *app) tiles(c *ui.Context, cats []spotify.Category) {
	ui.Grid(c).Columns(a.columns(c, 180)).Gap(20).Children(func() {
		for i := range cats {
			a.categoryTile(c.Key(cats[i].ID), &cats[i])
		}
	})
}

// categoryTile is a category's colored tile, its picture tilted in its
// corner, as on Spotify.
func (a *app) categoryTile(c *ui.Context, cat *spotify.Category) {
	bg := a.pal.press
	if len(cat.Color) == 7 && cat.Color[0] == '#' {
		bg = ui.Hex(cat.Color)
	}
	b := ui.ButtonBase(c).Column().AlignItems(ui.Start).Justify(ui.Start).AspectRatio(1.6).Padding(14).Radius(10).Clip().
		Background(bg).FocusRing(false).Label(cat.Name)
	if b.Hovered() {
		b.Shadow(0, 10, 24, -8, a.pal.cardShade)
	}
	b.Children(func() {
		ui.Text(c, cat.Name).FontSize(17).FontWeight(800).TextColor(white).MaxLines(2).LineHeight(1.25).Margin(0, 56, 0, 0)
		if cat.Image != "" {
			ui.Box(c).Absolute().Right(-12).Bottom(-4).Size(76, 76).Rotate(25).Radius(6).Shadow(0, 4, 14, 0, ui.RGBA(0, 0, 0, 0.3)).
				Children(func() { a.coverFill(c, cat.Image, 200, 6) })
		}
	})
	if b.Clicked() {
		a.goTo(route{page: pageCategory, category: cat.ID})
	}
}

// categoryPage lists a category's rows: a row of each, the rest shown on
// asking, and tiles of the categories within it.
func (a *app) categoryPage(c *ui.Context) {
	uri := a.route.category
	pg := a.catPages[uri]
	if a.waiting(c, "category:"+uri, pg != nil, func() { a.loadCategory(uri) }) {
		return
	}
	a.scrollPage(c, "category", func() {
		a.heading(c, pg.Name, "分类")
		cols := a.columns(c, 168)
		for i, sh := range pg.Shelves {
			key := uri + "#" + fmt.Sprint(i)
			if sh.Title != "" || i > 0 {
				ui.Row(c.Key(key)).AlignItems(ui.Center).Margin(34, 0, 14, 0).Children(func() {
					ui.Text(c, sh.Title).FontSize(19).FontWeight(700).LetterSpacing(-0.3).Grow(1)
					if len(sh.Lists) > cols {
						label := "显示全部"
						if a.shelfOpen[key] {
							label = "收起"
						}
						if ui.Button(c.Key("all"), label).Clicked() {
							if a.shelfOpen == nil {
								a.shelfOpen = map[string]bool{}
							}
							a.shelfOpen[key] = !a.shelfOpen[key]
						}
					}
				})
			}
			if len(sh.Tiles) > 0 {
				a.tiles(c.Key(key+"t"), sh.Tiles)
			}
			lists := sh.Lists
			if !a.shelfOpen[key] {
				lists = lists[:min(len(lists), cols)]
			}
			if len(lists) > 0 {
				ui.Grid(c.Key(key+"l")).Columns(cols).GapX(20).GapY(28).Children(func() {
					for j := range lists {
						a.playlistCard(c.Key(lists[j].ID), &lists[j])
					}
				})
			}
		}
	})
}

// userPage is someone's profile: who they are, and their public playlists.
func (a *app) userPage(c *ui.Context) {
	id := a.route.user
	pg := a.users[id]
	if a.waiting(c, "user:"+id, pg != nil, func() { a.loadUser(id) }) {
		return
	}
	p := a.pal
	a.scrollPage(c, "user", func() {
		ui.Row(c).Gap(28).AlignItems(ui.End).Padding(0, 0, 8, 0).Children(func() {
			a.profileAvatar(c, &spotify.Profile{UserID: pg.ID, Nickname: pg.Name, AvatarURL: pg.Image, AvatarColor: pg.Color}, 180)
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
				ui.Text(c, "个人主页").FontSize(12).FontWeight(600).TextColor(p.accent)
				ui.Text(c, pg.Name).FontSize(40).FontWeight(800).LetterSpacing(-0.8).MaxLines(2).LineHeight(1.15)
				stats := []string{fmt.Sprintf("%d 个公开歌单", pg.Total)}
				if pg.Followers > 0 {
					stats = append(stats, plays(float64(pg.Followers))+" 位粉丝")
				}
				if pg.Following > 0 {
					stats = append(stats, fmt.Sprintf("关注 %d 人", pg.Following))
				}
				ui.Text(c, strings.Join(stats, " · ")).FontSize(13).TextColor(p.muted)
			})
		})
		if len(pg.Playlists) == 0 {
			ui.Text(c, "没有公开歌单").FontSize(13).TextColor(p.muted).Margin(34, 0, 0, 0)
			return
		}
		a.section(c, "公开歌单")
		ui.Grid(c).Columns(a.columns(c, 168)).GapX(20).GapY(28).Children(func() {
			for i := range pg.Playlists {
				a.playlistCard(c.Key(pg.Playlists[i].ID), &pg.Playlists[i])
			}
		})
		if more := pg.Total - len(pg.Playlists); more > 0 {
			ui.Text(c, fmt.Sprintf("还有 %d 个歌单在 Spotify 上", more)).FontSize(12.5).TextColor(p.muted).Margin(20, 0, 0, 0)
		}
	})
}
