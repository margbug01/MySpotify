package spotify

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// testTokenServer swaps where the identity comes from for a server of the
// test's own, and puts the real places back after it.
func testTokenServer(t *testing.T, token func(w http.ResponseWriter, r *http.Request)) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/time":
			json.NewEncoder(w).Encode(map[string]any{"serverTime": time.Now().Unix()})
		case "/token":
			token(w, r)
		default: // the client token is not what this is about
			http.Error(w, "no", http.StatusInternalServerError)
		}
	}))
	oldToken, oldTime, oldClient := apiTokenURL, serverTimeURL, clientTokenURL
	apiTokenURL, serverTimeURL, clientTokenURL = srv.URL+"/token", srv.URL+"/time", srv.URL+"/clienttoken"
	t.Cleanup(func() {
		apiTokenURL, serverTimeURL, clientTokenURL = oldToken, oldTime, oldClient
		srv.Close()
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
		json.NewEncoder(w).Encode(map[string]any{
			"clientId":                         "x",
			"accessToken":                      "theirs",
			"accessTokenExpirationTimestampMs": time.Now().Add(time.Hour).UnixMilli(),
			"isAnonymous":                      false,
		})
	})
	id, err := AccountIdentity("sp_dc=abc; sp_key=def")
	if err != nil {
		t.Fatalf("AccountIdentity: %v", err)
	}
	if id.Token != "theirs" || id.Anonymous {
		t.Fatalf("identity = %+v, want the listener's token", id)
	}
	if cookie != "sp_dc=abc; sp_key=def" {
		t.Errorf("Cookie = %q, want the cookies it was given", cookie)
	}
	if len(totp) != 6 {
		t.Errorf("totp = %q, want a six-digit code", totp)
	}
}

// A visitor's token, which is what stale cookies get, is not the listener's:
// taking it would browse and play as nobody, quietly.
func TestAccountIdentityRefusesTheVisitor(t *testing.T) {
	testTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"accessToken":                      "visitor",
			"accessTokenExpirationTimestampMs": time.Now().Add(time.Hour).UnixMilli(),
			"isAnonymous":                      true,
		})
	})
	if id, err := AccountIdentity("sp_dc=stale"); err == nil {
		t.Fatalf("AccountIdentity = %+v, want an error for the visitor's token", id)
	}
}
