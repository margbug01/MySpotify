package spotify

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// A command Spotify turns down says which it was, and why, as Spotify said.
func TestConnectErrorSaysWhy(t *testing.T) {
	old := Client
	defer func() { Client = old }()
	SetIdentity(Identity{Token: "t", ClientToken: "c", Expires: time.Now().Add(time.Hour)})
	defer SetIdentity(Identity{})
	Client = &http.Client{Transport: lyricsTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 403, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error_type":"FORBIDDEN","message":"no"}`))}, nil
	})}
	err := NewConnect("me", "MySpotify", nil).Command("me", "skip_next", nil)
	var turned *ConnectError
	if !errors.As(err, &turned) || turned.Status != 403 || turned.Endpoint != "skip_next" || turned.Type != "FORBIDDEN" || turned.Message != "no" ||
		!strings.Contains(err.Error(), "skip_next /connect-state/v1/player/command/from/me-observer/to/me HTTP 403: FORBIDDEN no") {
		t.Fatalf("%#v: %v", turned, err)
	}
}

// Spotify answering with server errors is not reachable: its 502s for every
// track counted as answers, so the player passed three tracks over in three
// seconds and stopped, rather than waiting for Spotify (issue #32).
func TestReachableNotOnServerErrors(t *testing.T) {
	old := Client
	defer func() { Client = old }()
	for status, want := range map[int]bool{404: true, 502: false, 503: false} {
		Client = &http.Client{Transport: lyricsTransport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
		})}
		if got := Reachable(); got != want {
			t.Errorf("HTTP %d: reachable %v, want %v", status, got, want)
		}
	}
}
