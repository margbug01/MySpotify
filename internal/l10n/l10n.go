// Package l10n holds the app's texts in the language MyGo selected.
package l10n

import (
	"embed"

	"github.com/egoist/mygo/i18n"
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

// Language is also used for Spotify's localized headings.
func Language() string {
	if English() {
		return "en"
	}
	return "zh-CN"
}
