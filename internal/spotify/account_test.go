package spotify

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// tokenServer stands in for where the identity comes from: the token the
// test's handler gives, and a client token, or a 500 for one while
// clientDown says so. clientAsks counts the asks for one.
type tokenServer struct {
	clientDown atomic.Bool
	clientAsks atomic.Int32
}

// testTokenServer swaps where the identity comes from for a server of the
// test's own, and puts the real places back after it.
func testTokenServer(t *testing.T, token func(w http.ResponseWriter, r *http.Request)) *tokenServer {
	t.Helper()
	ts := &tokenServer{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/time":
			json.NewEncoder(w).Encode(map[string]any{"serverTime": time.Now().Unix()})
		case "/token":
			token(w, r)
		case "/clienttoken":
			ts.clientAsks.Add(1)
			if ts.clientDown.Load() {
				http.Error(w, "no", http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"granted_token": map[string]any{"token": "device"}})
		default:
			http.NotFound(w, r)
		}
	}))
	oldToken, oldTime, oldClient := apiTokenURL, serverTimeURL, clientTokenURL
	apiTokenURL, serverTimeURL, clientTokenURL = srv.URL+"/token", srv.URL+"/time", srv.URL+"/clienttoken"
	t.Cleanup(func() {
		apiTokenURL, serverTimeURL, clientTokenURL = oldToken, oldTime, oldClient
		srv.Close()
		SignOut()
	})
	return ts
}

// theirs answers as /api/token does to a listener's cookies.
func theirs(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]any{
		"clientId":                         "x",
		"accessToken":                      "theirs",
		"accessTokenExpirationTimestampMs": time.Now().Add(time.Hour).UnixMilli(),
		"isAnonymous":                      false,
	})
}

// AccountIdentity asks with the listener's cookies, and takes the token as
// theirs: the one the web view would have handed over, without its page.
func TestAccountIdentityAsksWithTheCookies(t *testing.T) {
	var cookie, totp string
	testTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		cookie, totp = r.Header.Get("Cookie"), r.URL.Query().Get("totp")
		if r.URL.Query().Get("totpVer") != "61" {
			t.Errorf("totpVer = %q, want the version the secret is for", r.URL.Query().Get("totpVer"))
		}
		theirs(w, r)
	})
	id, err := AccountIdentity("sp_dc=abc; sp_key=def")
	if err != nil {
		t.Fatalf("AccountIdentity: %v", err)
	}
	if id.Token != "theirs" || id.Anonymous || id.ClientToken != "device" {
		t.Fatalf("identity = %+v, want the listener's token and a client token", id)
	}
	if cookie != "sp_dc=abc; sp_key=def" {
		t.Errorf("Cookie = %q, want the cookies it was given", cookie)
	}
	if len(totp) != 6 {
		t.Errorf("totp = %q, want a six-digit code", totp)
	}
}

// A visitor's token, which is what stale cookies get, is not the listener's:
// taking it would browse and play as nobody, quietly. It is told apart, for
// the app to forget the cookies, and without a client token asked for it.
func TestAccountIdentityRefusesTheVisitor(t *testing.T) {
	ts := testTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"clientId":                         "x",
			"accessToken":                      "visitor",
			"accessTokenExpirationTimestampMs": time.Now().Add(time.Hour).UnixMilli(),
			"isAnonymous":                      true,
		})
	})
	id, err := AccountIdentity("sp_dc=stale")
	if !errors.Is(err, ErrStaleCookies) {
		t.Fatalf("AccountIdentity = %+v, %v; want ErrStaleCookies", id, err)
	}
	if n := ts.clientAsks.Load(); n != 0 {
		t.Errorf("asked for a client token %d times for a token thrown away", n)
	}
}

// Spotify turning the request down is not the cookies going stale: they
// are kept, and the web view tried.
func TestAccountIdentityTurnedDownIsNotStale(t *testing.T) {
	testTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusUnauthorized)
	})
	if _, err := AccountIdentity("sp_dc=abc"); err == nil || errors.Is(err, ErrStaleCookies) {
		t.Fatalf("err = %v, want an error that is not ErrStaleCookies", err)
	}
}

// A client token that cannot be had this time keeps the one requests go
// with now, which lasts a fortnight; with none to keep, the renewal fails,
// rather than giving the listener an hour of spclient turning them down.
func TestAccountIdentityKeepsTheClientToken(t *testing.T) {
	ts := testTokenServer(t, theirs)
	ts.clientDown.Store(true)
	SetIdentity(Identity{Token: "before", ClientToken: "kept", Expires: time.Now().Add(time.Minute)})
	id, err := AccountIdentity("sp_dc=abc")
	if err != nil || id.Token != "theirs" || id.ClientToken != "kept" {
		t.Fatalf("AccountIdentity = %+v, %v; want the new token with the client token kept", id, err)
	}
	SignOut()
	if id, err := AccountIdentity("sp_dc=abc"); err == nil {
		t.Fatalf("AccountIdentity = %+v with no client token to go with it, want an error", id)
	}
}

// A reply that names no expiry is taken to last half an hour, not to have
// ended in 1970, which would renew it on every request.
func TestATokenWithNoExpiryLasts(t *testing.T) {
	testTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"clientId": "x", "accessToken": "theirs"})
	})
	id, err := AccountIdentity("sp_dc=abc")
	if err != nil {
		t.Fatal(err)
	}
	if left := time.Until(id.Expires); left < 25*time.Minute {
		t.Fatalf("expires in %v, want half an hour", left)
	}
}
