// Package l10n holds the app's texts in the language MyGo selected.
package l10n

import (
	"embed"

	"github.com/egoist/mygo/i18n"
	// MyGo's own strings, its controls', dialogs' and menus', in the
	// app's languages that MyGo translates.
	_ "github.com/egoist/mygo/locales/ja"
	_ "github.com/egoist/mygo/locales/zh-Hans"
	_ "github.com/egoist/mygo/locales/zh-Hant"
	"golang.org/x/text/language"
)

//go:embed *.json
var files embed.FS

var catalog = func() *i18n.Catalog {
	c, err := i18n.NewCatalog(files, "zh-Hans")
	if err != nil {
		panic(err)
	}
	i18n.Register(c)
	return c
}()

// T translates an app-owned label; user content never goes through it.
func T(key string) string { return catalog.Format(i18n.Locale(), key, nil) }

// Count formats a count with the selected language's plural forms.
func Count(key string, n int) string {
	return catalog.Format(i18n.Locale(), key, map[string]any{"count": n})
}

// English reports whether the selected interface is English.
func English() bool { return i18n.Base(i18n.Locale()) == "en" }

// Canonical interface language tags returned by Language.
const (
	LangZHCN = "zh-CN"
	LangZHTW = "zh-TW"
	LangEN   = "en"
	LangIT   = "it"
	LangJA   = "ja"
)

// Language is also used for Spotify's localized headings.
func Language() string {
	loc := i18n.Normalize(i18n.Locale())
	switch i18n.Base(loc) {
	case "en":
		return LangEN
	case "it":
		return LangIT
	case "ja":
		return LangJA
	case "zh":
		if t, err := language.Parse(loc); err == nil {
			if script, _ := t.Script(); script.String() == "Hant" {
				return LangZHTW
			}
		}
	}
	return LangZHCN
}

// Chinese reports whether the selected interface is Chinese (Simplified or Traditional).
func Chinese() bool {
	lang := Language()
	return lang == LangZHCN || lang == LangZHTW
}
