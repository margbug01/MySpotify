package app

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
)

// The listener's cookies are kept beside the settings, so that renewing the
// identity they name needs no web view: the page would load, ask Spotify for
// a token, and be read for it, which costs a window, half a minute of its
// first load, and the memory it keeps while playing. They are what signs the
// app in, so only the listener reads them.

// accountCookiePath is where those cookies are kept.
func accountCookiePath() (string, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "account-cookies"), nil
}

// loadAccountCookies returns the cookies kept, "" if there are none.
func loadAccountCookies() string {
	p, err := accountCookiePath()
	if err != nil {
		return ""
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// saveAccountCookies keeps cookies for the next renewal to ask with.
func saveAccountCookies(cookies string) {
	p, err := accountCookiePath()
	if err != nil || cookies == "" {
		return
	}
	if os.WriteFile(p, []byte(cookies), 0o600) != nil {
		return
	}
}

// forgetAccountCookies drops them, as signing out does.
func forgetAccountCookies() {
	if p, err := accountCookiePath(); err == nil {
		os.Remove(p)
	}
}
