package app

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// useCookieStore keeps the cookies in a directory of the test's own, sealed
// by a stand-in for the platform's key: reversed, after a mark it checks.
func useCookieStore(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "account-cookies")
	oldPath, oldSeal, oldUnseal := accountCookiePath, sealCookies, unsealCookies
	accountCookiePath = func() (string, error) { return p, nil }
	sealCookies = func(b []byte) ([]byte, error) {
		out := []byte("sealed:")
		for i := len(b) - 1; i >= 0; i-- {
			out = append(out, b[i])
		}
		return out, nil
	}
	unsealCookies = func(b []byte) ([]byte, error) {
		rest, ok := bytes.CutPrefix(b, []byte("sealed:"))
		if !ok {
			return nil, errUnreadable
		}
		out := make([]byte, 0, len(rest))
		for i := len(rest) - 1; i >= 0; i-- {
			out = append(out, rest[i])
		}
		return out, nil
	}
	t.Cleanup(func() { accountCookiePath, sealCookies, unsealCookies = oldPath, oldSeal, oldUnseal })
	return p
}

// The cookies are what signs the app in: on disk they are sealed, never as
// they are, and only their owner reads the file.
func TestAccountCookiesAreKeptSealed(t *testing.T) {
	p := useCookieStore(t)
	saveAccountCookies(accountEpoch(), "sp_dc=secret; sp_key=k")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "sp_dc=secret") {
		t.Fatalf("kept in the clear: %q", b)
	}
	if got := loadAccountCookies(); got != "sp_dc=secret; sp_key=k" {
		t.Fatalf("loadAccountCookies = %q", got)
	}
	if runtime.GOOS != "windows" {
		if fi, err := os.Stat(p); err != nil || fi.Mode().Perm() != 0o600 {
			t.Fatalf("mode %v, %v; want 0600", fi.Mode().Perm(), err)
		}
	}
	// Written again, through a file beside it, which is not left behind.
	saveAccountCookies(accountEpoch(), "sp_dc=next")
	if got := loadAccountCookies(); got != "sp_dc=next" {
		t.Fatalf("loadAccountCookies = %q after a second save", got)
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(p), ".account-cookies-*")); len(left) != 0 {
		t.Fatalf("left behind: %v", left)
	}
}

// A renewal under way as the listener signs out finds what it finds, and
// keeps none of it: it would sign them back in at the next start.
func TestSigningOutKeepsNothingFoundMeanwhile(t *testing.T) {
	useCookieStore(t)
	saveAccountCookies(accountEpoch(), "sp_dc=theirs")
	epoch := accountEpoch() // a renewal starts
	forgetAccountCookies()  // the listener signs out
	saveAccountCookies(epoch, "sp_dc=theirs")
	if got := loadAccountCookies(); got != "" {
		t.Fatalf("kept %q after signing out", got)
	}
	// A sign-in after it keeps its own.
	saveAccountCookies(accountEpoch(), "sp_dc=again")
	if got := loadAccountCookies(); got != "sp_dc=again" {
		t.Fatalf("loadAccountCookies = %q, want the new sign-in's", got)
	}
}

// Cookies Spotify answered as a visitor's are dropped, so that a start
// neither asks with them again nor waits on the web view for nobody; ones a
// sign-in kept meanwhile stay.
func TestStaleCookiesAreDropped(t *testing.T) {
	p := useCookieStore(t)
	saveAccountCookies(accountEpoch(), "sp_dc=fresh")
	dropAccountCookies("sp_dc=stale")
	if got := loadAccountCookies(); got != "sp_dc=fresh" {
		t.Fatalf("dropped %q, which were not the stale ones", got)
	}
	dropAccountCookies("sp_dc=fresh")
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("the stale cookies stay: %v", err)
	}
}

// A file that does not unseal — the clear text an earlier build kept, or
// another machine's — is dropped; one the key could not be had for, as a
// locked keychain, is not, for it may unseal next time.
func TestUnreadableCookiesAreDropped(t *testing.T) {
	p := useCookieStore(t)
	if err := os.WriteFile(p, []byte("sp_dc=in-the-clear"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := loadAccountCookies(); got != "" {
		t.Fatalf("loadAccountCookies = %q from a file not sealed", got)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("the file not sealed stays: %v", err)
	}

	saveAccountCookies(accountEpoch(), "sp_dc=theirs")
	unsealCookies = func([]byte) ([]byte, error) { return nil, errors.New("keychain: locked") }
	if got := loadAccountCookies(); got != "" {
		t.Fatalf("loadAccountCookies = %q without the key", got)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("dropped for want of the key: %v", err)
	}
}
