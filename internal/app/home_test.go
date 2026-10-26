package app

import (
	"os"
	"path/filepath"
	"testing"

	"vinyl/internal/spotify"
)

// What the home page showed shows again on the next start, until signing
// out forgets it.
func TestHomeSnapshotShowsAgain(t *testing.T) {
	file := filepath.Join(t.TempDir(), "home.json")
	a := newTestApp()
	a.homeFile = file
	a.profile.Email = "listener@example.com"
	a.featured = []spotify.Playlist{{ID: "spotify:playlist:f", Name: "精选"}}
	a.charts = []spotify.Playlist{{ID: "spotify:playlist:c", Preview: []spotify.ChartLine{{First: "歌", Second: "人"}}}}
	a.daily = []spotify.Track{{ID: "t1", Name: "一首"}}
	a.mine = []spotify.Playlist{{ID: spotify.LikedURI, Name: "我喜欢的音乐"}}
	a.followed = []spotify.ArtistCard{{ID: "spotify:artist:a", Name: "艺人"}}
	a.keepHome()

	b := newTestApp()
	b.profile = nil
	b.homeFile = file
	b.restoreHome()
	if b.profile == nil || b.profile.UserID != "listener" || b.profile.Email != "listener@example.com" {
		t.Fatalf("profile = %+v", b.profile)
	}
	if len(b.featured) != 1 || len(b.charts) != 1 || b.charts[0].Preview[0].Second != "人" ||
		len(b.daily) != 1 || len(b.mine) != 1 || len(b.followed) != 1 {
		t.Fatalf("restored %+v %+v %+v %+v %+v", b.featured, b.charts, b.daily, b.mine, b.followed)
	}

	b.profile = nil
	b.keepHome()
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("signed out, home.json stays: %v", err)
	}
}

// Signing in as the one restoreHome showed keeps what shows, and the
// email its picture comes from, until they load again; as someone else,
// what was theirs goes.
func TestSignedInKeepsWhatShowedForTheSameListener(t *testing.T) {
	for _, tc := range []struct {
		user string
		keep bool
	}{{"listener", true}, {"someone-else", false}} {
		a := newTestApp()
		a.restored = true // no session.json to read
		a.profile.Email = "listener@example.com"
		a.featured = []spotify.Playlist{{ID: "spotify:playlist:f"}}
		a.mine = []spotify.Playlist{{ID: spotify.LikedURI}}
		a.daily = []spotify.Track{{ID: "t1"}}
		a.signedIn(&spotify.Profile{UserID: tc.user})
		kept := len(a.featured) == 1 && len(a.mine) == 1 && len(a.daily) == 1
		if kept != tc.keep {
			t.Errorf("%s: kept = %v, want %v", tc.user, kept, tc.keep)
		}
		if email := a.profile.Email != ""; email != tc.keep {
			t.Errorf("%s: email kept = %v, want %v", tc.user, email, tc.keep)
		}
	}
}
