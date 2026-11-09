package spotify

import (
	"errors"
	"net/http/httptest"
	"net/url"
	"testing"
)

// A browser-supplied account response completes the normal device-token handshake.
func TestAccountIdentityViaBrowser(t *testing.T) {
	server := testTokenServer(t, theirs)
	calls := 0
	id, err := AccountIdentityViaBrowser(func(target string) ([]byte, error) {
		calls++
		u, err := url.Parse(target)
		if err != nil || len(u.Query().Get("totp")) != 6 {
			t.Fatal("browser request lacks the web-player proof")
		}
		response := httptest.NewRecorder()
		theirs(response, httptest.NewRequest("GET", target, nil))
		return response.Body.Bytes(), nil
	})
	if err != nil || calls != 1 || id.Anonymous || id.Token == "" || id.ClientToken == "" || server.clientAsks.Load() != 1 {
		t.Fatal("browser identity did not complete with a device token")
	}
}

// Guest responses and failed browser requests must never become signed-in identities.
func TestAccountIdentityViaBrowserRefusesGuestAndPreservesErrors(t *testing.T) {
	server := testTokenServer(t, theirs)
	if _, err := AccountIdentityViaBrowser(func(string) ([]byte, error) { return []byte(`{"accessToken":"fixture","isAnonymous":true}`), nil }); !errors.Is(err, ErrStaleCookies) {
		t.Fatal("visitor identity was accepted as signed in")
	}
	closed := errors.New("browser closed")
	if _, err := AccountIdentityViaBrowser(func(string) ([]byte, error) { return nil, closed }); !errors.Is(err, closed) {
		t.Fatal("browser failure was not preserved")
	}
	if server.clientAsks.Load() != 0 {
		t.Fatal("failed identity requested a device token")
	}
}
