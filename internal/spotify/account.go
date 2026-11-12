package spotify

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Where the identity comes from, and Spotify's clock: tests swap them.
var (
	apiTokenURL   = "https://open.spotify.com/api/token"
	serverTimeURL = "https://open.spotify.com/api/server-time"
)

// ErrStaleCookies is what AccountIdentity fails with when the cookies it was
// given are signed out: Spotify answered them with a visitor's token, so
// keeping them only costs a request every renewal.
var ErrStaleCookies = errors.New("spotify: the cookies are signed out")

// AccountIdentity gets the identity of the listener these cookies belong to,
// as the web player does once they have signed in: the same one-time code
// proves the client is the web player, and the cookies say who is asking.
//
// It is how a signed-in identity is renewed without the web view, whose page
// would otherwise have to load, ask, and be read for its token. It fails
// with ErrStaleCookies when the cookies have gone stale, or with another
// error when Spotify turns the request down or there is no client token to
// go with it: the caller opens the web view then, which can sign in again.
func AccountIdentity(cookies string) (Identity, error) {
	r, err := apiToken(cookies)
	if err != nil {
		return Identity{}, fmt.Errorf("spotify: account token: %w", err)
	}
	return accountIdentity(r)
}

// AccountIdentityViaBrowser asks through the browser's own cookie store. This
// avoids exporting HttpOnly cookies when the WebView cannot enumerate a cold store.
func AccountIdentityViaBrowser(fetch func(string) ([]byte, error)) (Identity, error) {
	body, err := fetch(tokenRequestURL())
	if err != nil {
		return Identity{}, err
	}
	r, err := parseTokenReply(body)
	if err != nil {
		return Identity{}, err
	}
	return accountIdentity(r)
}

func accountIdentity(r tokenReply) (Identity, error) {
	// Before the client token is asked for: a visitor's would be thrown away.
	if r.id.Anonymous {
		return Identity{}, ErrStaleCookies
	}
	id := r.id
	// spclient refuses a request without a client token, and what a listener
	// does goes through it: playback, their library, Connect. The one the
	// identity before had lasts a fortnight, so a failure to get a new one
	// keeps it; with none to keep, the web view gets one with its token,
	// rather than an hour of nothing playing.
	ct, err := clientToken(r.clientID)
	if err != nil {
		if ct = lastClientToken(); ct == "" {
			return Identity{}, fmt.Errorf("spotify: account token: %w", err)
		}
	}
	id.ClientToken = ct
	return id, nil
}

// lastClientToken is the client token requests go with now, the listener's
// or else the guest's, or else the one kept from a run before: any is this
// device's, which is what it names.
func lastClientToken() string {
	idMu.Lock()
	ct := current.ClientToken
	idMu.Unlock()
	if ct != "" {
		return ct
	}
	guestMu.Lock()
	ct = guest.ClientToken
	guestMu.Unlock()
	if ct != "" {
		return ct
	}
	return keptClientToken()
}

// withClientToken gives id the last client token if it came without one, as
// the web view's does when its page could not get one in time.
func withClientToken(id Identity) Identity {
	if id.ClientToken == "" && id.Token != "" && !id.Anonymous {
		id.ClientToken = lastClientToken()
	}
	return id
}

// ClientTokenFile, set by the app, keeps the last client token granted and
// when it runs out, for the next start: on a network that cannot reach
// clienttoken, songs play with it, rather than only once some other network
// has got one this run (#20).
var ClientTokenFile string

func keepClientToken(token string, until time.Time) {
	if ClientTokenFile != "" {
		os.WriteFile(ClientTokenFile, []byte(token+"\n"+strconv.FormatInt(until.Unix(), 10)), 0o600)
	}
}

// keptClientToken is the client token ClientTokenFile keeps, "" if there is
// none or it has run out.
func keptClientToken() string {
	if ClientTokenFile == "" {
		return ""
	}
	b, err := os.ReadFile(ClientTokenFile)
	if err != nil {
		return ""
	}
	token, until, _ := strings.Cut(string(b), "\n")
	sec, err := strconv.ParseInt(until, 10, 64)
	if err != nil || !time.Now().Before(time.Unix(sec, 0)) {
		return ""
	}
	return token
}

// tokenReply is what /api/token answered: the identity, without its client
// token, and the client ID one is asked for with.
type tokenReply struct {
	id       Identity
	clientID string
}

// apiToken asks /api/token for an identity, with cookies (none for a
// visitor). The access token lasts about an hour.
func tokenRequestURL() string {
	now := time.Now()
	sec := totpSecrets[0]

	// The code of now and the code of Spotify's clock must both be right, so
	// ask Spotify what its clock reads first. If it does not say, now is
	// close enough: the codes are 30 seconds wide.
	serverTime := now
	if t, err := serverTimeOf(); err == nil {
		serverTime = t
	}

	return apiTokenURL + "?" + tokenQuery(sec.unmask(), sec.version, now, serverTime)
}

func apiToken(cookies string) (tokenReply, error) {
	req, err := http.NewRequest(http.MethodGet, tokenRequestURL(), nil)
	if err != nil {
		return tokenReply{}, err
	}
	req.Header.Set("User-Agent", tokenUserAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", "https://open.spotify.com/")
	if cookies != "" {
		req.Header.Set("Cookie", cookies)
	}
	res, err := Client.Do(req)
	if err != nil {
		return tokenReply{}, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return tokenReply{}, err
	}
	if res.StatusCode != http.StatusOK {
		return tokenReply{}, fmt.Errorf("/api/token: %s", res.Status)
	}
	return parseTokenReply(body)
}

func parseTokenReply(body []byte) (tokenReply, error) {
	var v struct {
		ClientID    string `json:"clientId"`
		AccessToken string `json:"accessToken"`
		Expires     int64  `json:"accessTokenExpirationTimestampMs"`
		Anonymous   bool   `json:"isAnonymous"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return tokenReply{}, fmt.Errorf("/api/token: %w", err)
	}
	if v.AccessToken == "" {
		return tokenReply{}, fmt.Errorf("/api/token: no token in the reply")
	}
	id := Identity{Token: v.AccessToken, Expires: time.UnixMilli(v.Expires), Anonymous: v.Anonymous}
	if v.Expires == 0 {
		id.Expires = time.Now().Add(30 * time.Minute)
	}
	return tokenReply{id: id, clientID: v.ClientID}, nil
}
