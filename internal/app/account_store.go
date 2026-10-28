package app

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// The listener's cookies are kept, so that renewing the identity they name
// needs no web view: the page would load, ask Spotify for a token, and be
// read for it, which costs a window, ten seconds of its first load, and the
// memory it keeps while playing. They are what signs the app in — sp_dc is
// the account — so they are kept sealed with a key of the listener's login
// (seal_darwin.go, seal_windows.go), no more readable than the web view's
// own, and dropped once they are signed out, as signing out does.

var (
	cookieMu sync.Mutex
	// cookieEpoch counts the sign-outs: a renewal under way during one
	// keeps nothing it finds (saveAccountCookies).
	cookieEpoch int

	// Where the cookies are kept and what seals them: tests swap them.
	accountCookiePath = defaultAccountCookiePath
	sealCookies       = seal
	unsealCookies     = unseal
)

// errUnreadable is what a file unseal cannot open fails with: another
// build's, another machine's, or cut short. It is dropped then, and the web
// view signs in and keeps the cookies anew.
var errUnreadable = errors.New("account cookies: sealed with another key")

func defaultAccountCookiePath() (string, error) {
	dir, err := accountCookieDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "account-cookies"), nil
}

// accountEpoch is the count of sign-outs so far, for saveAccountCookies.
func accountEpoch() int {
	cookieMu.Lock()
	defer cookieMu.Unlock()
	return cookieEpoch
}

// loadAccountCookies returns the cookies kept, "" if there are none.
func loadAccountCookies() string {
	cookieMu.Lock()
	defer cookieMu.Unlock()
	return loadLocked()
}

func loadLocked() string {
	p, err := accountCookiePath()
	if err != nil {
		return ""
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	plain, err := unsealCookies(b)
	if errors.Is(err, errUnreadable) {
		log.Printf("%v: dropped", err)
		os.Remove(p)
		return ""
	}
	if err != nil {
		log.Printf("account cookies: %v", err)
		return ""
	}
	return strings.TrimSpace(string(plain))
}

// saveAccountCookies keeps cookies for the next renewal to ask with, unless
// the listener signed out since epoch: what a renewal under way then finds
// would sign them back in at the next start.
func saveAccountCookies(epoch int, cookies string) {
	if cookies == "" {
		return
	}
	cookieMu.Lock()
	defer cookieMu.Unlock()
	if epoch != cookieEpoch {
		return
	}
	p, err := accountCookiePath()
	if err != nil {
		return
	}
	sealed, err := sealCookies([]byte(cookies))
	if err != nil {
		log.Printf("account cookies: %v: not kept", err)
		return
	}
	if err := writeAtomic(p, sealed); err != nil {
		log.Printf("account cookies: %v", err)
	}
}

// forgetAccountCookies drops them, as signing out does, and makes what any
// renewal under way finds no one's.
func forgetAccountCookies() {
	cookieMu.Lock()
	defer cookieMu.Unlock()
	cookieEpoch++
	if p, err := accountCookiePath(); err == nil {
		os.Remove(p)
	}
}

// dropAccountCookies drops the cookies kept if they are still stale, the
// ones Spotify answered as a visitor's: a sign-in meanwhile kept new ones.
func dropAccountCookies(stale string) {
	cookieMu.Lock()
	defer cookieMu.Unlock()
	if stale == "" || loadLocked() != stale {
		return
	}
	if p, err := accountCookiePath(); err == nil {
		os.Remove(p)
	}
}

// writeAtomic writes b to p through a file beside it, which only its owner
// reads, so that p is either what it was or b: never half of it, and never
// left readable by others from before.
func writeAtomic(p string, b []byte) error {
	f, err := os.CreateTemp(filepath.Dir(p), ".account-cookies-*")
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(f.Name(), p)
	}
	if err != nil {
		os.Remove(f.Name())
	}
	return err
}
