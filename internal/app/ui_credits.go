package app

import (
	"log"
	"strings"

	"github.com/egoist/mygo/ui"

	"vinyl/internal/l10n"
	"vinyl/internal/spotify"
)

// trackCredits loads who made a track; tests replace it.
var trackCredits = spotify.TrackCredits

// creditsDialog shows who made a song, opened from its menu.
type creditsDialog struct {
	open  bool
	track spotify.Track
}

// showCredits opens the dialog of who made tr, loading them if need be.
func (a *app) showCredits(tr *spotify.Track) {
	a.creditsOf = creditsDialog{open: true, track: *tr}
	a.loadCredits(tr.ID)
}

// loadCredits loads who made track id, unless it has. Their groups are
// named in the app's language: a change of it drops them (language.go).
func (a *app) loadCredits(id string) {
	if a.credits == nil {
		a.credits, a.loadingCr, a.crErr = map[string]*spotify.Credits{}, map[string]bool{}, map[string]string{}
	}
	if a.credits[id] != nil || a.loadingCr[id] {
		return
	}
	a.loadingCr[id] = true
	delete(a.crErr, id)
	gen := a.languageGen
	go func() {
		cr, err := trackCredits(id)
		a.post(func() {
			if gen != a.languageGen {
				return
			}
			delete(a.loadingCr, id)
			if err != nil {
				log.Printf("credits %s: %v", id, err)
				a.crErr[id] = l10n.T("制作人员加载失败，请稍后再试")
				return
			}
			a.credits[id] = cr
		})
	}()
}

// creditsDialogView is the dialog of who made a song: its title and
// artists, then the credits group by group, scrolling when they are long.
func (a *app) creditsDialogView(c *ui.Context) {
	p := a.pal
	d := &a.creditsOf
	ui.DialogBase(c.Key("credits"), &d.open, func(backdrop, panel ui.Element) {
		_, h := c.Size()
		backdrop.Background(ui.RGBA(0, 0, 0, 0.35))
		panel.Width(440).MaxHeight(max(240, min(h-96, 640))).Padding(22, 0, 22, 0).Radius(18).Gap(16).
			Background(p.bg).Border(1, p.hairline).Shadow(0, 24, 60, -12, ui.RGBA(0, 0, 0, 0.4)).
			Transition(ui.ElementTransition{Animation: &ui.Snappy, Enter: &ui.Motion{Scale: 0.94}, Exit: &ui.Motion{Scale: 0.96}})
		tr := &d.track
		ui.Row(c).PaddingX(24).Gap(12).AlignItems(ui.Start).Children(func() {
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(4).Children(func() {
				ui.Text(c, l10n.T("制作人员")).FontSize(17).FontWeight(700)
				ui.Text(c, tr.Name+" · "+tr.ArtistNames()).FontSize(13).TextColor(p.muted).SingleLine()
			})
			if iconButton(c.Key("close"), iconX, 16, p.muted, l10n.T("关闭")).Margin(-4, -8, 0, 0).Clicked() {
				d.open = false
			}
		})
		cr := a.credits[tr.ID]
		switch {
		case cr != nil && len(cr.Groups) > 0:
			ui.Scroll(c.Key("body")).Shrink(1).MinHeight(0).PaddingX(24).Children(func() {
				a.creditGroups(c, cr, func(artist string) {
					d.open = false
					a.goTo(route{page: pageArtist, artist: artist})
				})
			})
		case cr != nil:
			ui.Text(c, l10n.T("Spotify 没有这首歌的制作人员信息")).FontSize(13).TextColor(p.muted).PaddingX(24)
		case a.loadingCr[tr.ID]:
			ui.Text(c, l10n.T("正在加载…")).FontSize(13).TextColor(p.muted).PaddingX(24)
		default:
			ui.Row(c).PaddingX(24).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Text(c, a.crErr[tr.ID]).FontSize(13).TextColor(p.muted).Grow(1).Shrink(1)
				if pillButton(c.Key("retry"), nil, l10n.T("重试"), p.hover.Over(p.bg), p.text).Clicked() {
					a.loadCredits(tr.ID)
				}
			})
		}
	})
}

// creditGroups lists who made a song group by group, as 作曲和作词 or
// 制作兼工程, then the labels it came from. A name with an artist page
// opens it through open.
func (a *app) creditGroups(c *ui.Context, cr *spotify.Credits, open func(artist string)) {
	p := a.pal
	ui.Column(c).Gap(24).Children(func() {
		for gi, g := range cr.Groups {
			ui.Column(c.Key(gi)).Gap(12).AlignItems(ui.Start).Children(func() {
				if g.Name != "" {
					ui.Text(c, g.Name).FontSize(12).FontWeight(600).TextColor(p.muted)
				}
				for i, who := range g.People {
					ui.Column(c.Key(i)).Gap(3).AlignItems(ui.Start).Children(func() {
						if who.Artist == "" {
							ui.Text(c, who.Name).FontSize(14).FontWeight(500)
						} else if link(c.Key("name"), who.Name, 14, p.text, p.text).Clicked() {
							open(who.Artist)
						}
						ui.Text(c, strings.Join(who.Roles, "、")).FontSize(12.5).TextColor(p.muted)
					})
				}
			})
		}
		if len(cr.Sources) > 0 {
			ui.Column(c.Key("sources")).Gap(6).Children(func() {
				ui.Text(c, l10n.T("来源")).FontSize(12).FontWeight(600).TextColor(p.muted)
				ui.Text(c, strings.Join(cr.Sources, "、")).FontSize(13)
			})
		}
	})
}
