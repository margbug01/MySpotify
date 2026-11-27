package app

import (
	"fmt"
	"time"

	"github.com/egoist/mygo"

	"vinyl/internal/l10n"
)

var setLocale = mygo.App.SetLocale

var languageOptions = []struct {
	tag   string
	label string
}{
	{"", ""},
	{"zh-Hans", "简体中文"},
	{"zh-Hant", "繁體中文"},
	{"en", "English"},
	{"it", "Italiano"},
}

func supportedLanguage(tag string) bool {
	for _, opt := range languageOptions {
		if opt.tag == tag {
			return true
		}
	}
	return false
}

// chooseLanguage keeps the choice, while MyGo saves and applies its locale.
func (a *app) chooseLanguage(tag string) {
	if !supportedLanguage(tag) {
		return
	}
	a.settings.Language = tag
	a.settings.save()
	setLocale(tag)
}

// languageChanged rebuilds native menus and refreshes Spotify's headings.
func (a *app) languageChanged() {
	a.setMenu()
	a.dockSig = ""
	a.syncDockMenu()
	if a.signin.win != nil {
		a.signin.win.SetTitle(l10n.T("登录 Spotify"))
	}
	if a.notch != nil && a.notch.win != nil {
		a.notch.win.SetTitle(appName + " · " + l10n.T("迷你播放器"))
	}
	if a.desk != nil && a.desk.win != nil {
		a.desk.win.SetTitle(l10n.T("桌面歌词"))
	}
	a.reloadLocalizedPages()
	if a.pl != nil {
		a.pl.ReloadLyrics()
	}
	a.redraw()
}

// reloadLocalizedPages keeps the selected facet while replacing localized data.
func (a *app) reloadLocalizedPages() {
	reloadHome := len(a.homeSections) > 0 || a.homeLoading
	a.languageGen++
	a.homeSections, a.homeShortcuts, a.charts = nil, nil, nil
	a.facets, a.sectionAll = nil, nil
	a.cats, a.catPages = nil, nil
	a.loading, a.loadErr = nil, nil
	a.credits, a.loadingCr, a.crErr = nil, nil, nil
	a.toast, a.proxyErr, a.upd.err = "", "", ""
	if reloadHome {
		a.loadHome()
	}
	if a.homeFacet != "" {
		a.loadFacet(a.homeFacet)
	}
	switch a.route.page {
	case pageBrowse:
		a.loadCategories()
	case pageCategory:
		a.loadCategory(a.route.category)
	case pageUser:
		a.loadUser(a.route.user)
	case pageSection:
		a.loadSection(a.route.section)
	case pageTrack:
		a.loadCredits(a.route.track)
	}
	if a.creditsOf.open {
		a.loadCredits(a.creditsOf.track.ID)
	}
}

func shortDate(day time.Time, year bool) string {
	if l10n.Language() == l10n.LangIT {
		if year {
			return fmt.Sprintf("%d/%d/%d", day.Day(), day.Month(), day.Year())
		}
		return fmt.Sprintf("%d/%d", day.Day(), day.Month())
	}
	if l10n.English() {
		if year {
			return day.Format("Jan 2, 2006")
		}
		return day.Format("Jan 2")
	}
	if year {
		return fmt.Sprintf("%d年%d月%d日", day.Year(), day.Month(), day.Day())
	}
	return fmt.Sprintf("%d月%d日", day.Month(), day.Day())
}

func dailyMonth(day time.Time) string {
	if l10n.Language() == l10n.LangIT {
		return fmt.Sprintf("%s · %s", [...]string{"gen", "feb", "mar", "apr", "mag", "giu", "lug", "ago", "set", "ott", "nov", "dic"}[day.Month()-1], weekday(day))
	}
	if l10n.English() {
		return day.Format("Jan · Mon")
	}
	return fmt.Sprintf("%d月 · %s", day.Month(), weekday(day))
}

func dailyDate(day time.Time) string {
	if l10n.Language() == l10n.LangIT {
		return fmt.Sprintf("%s %d/%d", weekday(day), day.Day(), day.Month())
	}
	if l10n.English() {
		return day.Format("Mon, Jan 2")
	}
	return fmt.Sprintf("%d月%d日 %s", day.Month(), day.Day(), weekday(day))
}
