package spotify

import (
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
)

// Changing the interface language reaches Spotify requests and only renames the built-in collection.
func TestRequestLanguage(t *testing.T) {
	previous := i18n.Locale()
	i18n.SetLocaleFunc(func() string { return os.Getenv("MYGO_LOCALE") })
	t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return previous }) })
	oldClient := Client
	idMu.Lock()
	oldID := current
	idMu.Unlock()
	t.Cleanup(func() { Client = oldClient; SetIdentity(oldID) })
	SetIdentity(Identity{Token: "test", Expires: time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)})
	var header string
	Client = &http.Client{Transport: lyricsTransport(func(req *http.Request) (*http.Response, error) {
		header = req.Header.Get("Accept-Language")
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	})}
	for _, tc := range []struct{ locale, header, liked string }{{"en", "en", "Liked Songs"}, {"zh-Hans", "zh-CN", likedName}} {
		t.Setenv("MYGO_LOCALE", tc.locale)
		_, err := do(func() (*http.Request, error) { return http.NewRequest(http.MethodGet, "https://example.test/", nil) })
		if err != nil {
			t.Fatal(err)
		}
		if header != tc.header {
			t.Errorf("language header %q, want %q", header, tc.header)
		}
		liked := Playlist{ID: LikedURI, Name: likedName}
		if got := liked.DisplayName(); got != tc.liked {
			t.Errorf("built-in collection %q, want %q", got, tc.liked)
		}
		custom := Playlist{ID: "spotify:playlist:custom", Name: likedName}
		if got := custom.DisplayName(); got != likedName {
			t.Errorf("renamed user playlist to %q", got)
		}
	}
}
