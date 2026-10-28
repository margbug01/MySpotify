package spotify

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Where the identity comes from, and Spotify's clock: tests swap them.
var (
	apiTokenURL   = "https://open.spotify.com/api/token"
	serverTimeURL = "https://open.spotify.com/api/server-time"
)

// AccountIdentity gets the identity of the listener these cookies belong to,
// as the web player does once they have signed in: the same one-time code
// proves the client is the web player, and the cookies say who is asking.
//
// It is how a signed-in identity is renewed without the web view, whose page
// would otherwise have to load, ask, and be read for its token. It fails
// with the visitor's token when the cookies have gone stale, or with an
// error when Spotify turns the request down: the caller opens the web view
// then, which can sign in again.
func AccountIdentity(cookies string) (Identity, error) {
	id, err := apiToken(cookies)
	if err != nil {
		return Identity{}, fmt.Errorf("spotify: account token: %w", err)
	}
	if id.Anonymous {
		return Identity{}, fmt.Errorf("spotify: account token: signed out")
	}
	return id, nil
}

// apiToken asks /api/token for an identity, with cookies (none for a
// visitor), and the client token that goes with it.
//
// The access token lasts about an hour; the client token a fortnight.
func apiToken(cookies string) (Identity, error) {
	now := time.Now()
	sec := totpSecrets[0]

	// The code of now and the code of Spotify's clock must both be right, so
	// ask Spotify what its clock reads first. If it does not say, now is
	// close enough: the codes are 30 seconds wide.
	serverTime := now
	if t, err := serverTimeOf(); err == nil {
		serverTime = t
	}

	url := apiTokenURL + "?" + tokenQuery(sec.unmask(), sec.version, now, serverTime)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return Identity{}, err
	}
	req.Header.Set("User-Agent", tokenUserAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", "https://open.spotify.com/")
	if cookies != "" {
		req.Header.Set("Cookie", cookies)
	}
	res, err := Client.Do(req)
	if err != nil {
		return Identity{}, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return Identity{}, err
	}
	if res.StatusCode != http.StatusOK {
		return Identity{}, fmt.Errorf("token: %s", res.Status)
	}
	var v struct {
		ClientID    string `json:"clientId"`
		AccessToken string `json:"accessToken"`
		Expires     int64  `json:"accessTokenExpirationTimestampMs"`
		Anonymous   bool   `json:"isAnonymous"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return Identity{}, fmt.Errorf("token: %w", err)
	}
	if v.AccessToken == "" {
		return Identity{}, fmt.Errorf("token: none in the reply")
	}
	id := Identity{Token: v.AccessToken, Expires: time.UnixMilli(v.Expires), Anonymous: v.Anonymous}
	if id.Expires.IsZero() {
		id.Expires = now.Add(30 * time.Minute)
	}
	// spclient refuses a request without a client token. A failure here is
	// not fatal: browsing goes through Pathfinder, which does not want one.
	if ct, err := clientToken(v.ClientID); err == nil {
		id.ClientToken = ct
	}
	return id, nil
}
