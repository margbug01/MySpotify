package app

import (
	"errors"
	"testing"
	"time"

	"vinyl/internal/spotify"
)

// The liked songs' list, once loaded, is what the hearts go by: songs liked
// elsewhere after sign in showed an empty heart as they played, and songs
// unliked a full one. Other lists leave the hearts be.
func TestLikedListRefreshesHearts(t *testing.T) {
	a := newTestApp()
	a.liked = map[string]bool{"unliked": true}
	a.receivePlaylist(spotify.LikedURI, &spotify.Playlist{ID: spotify.LikedURI, Tracks: []spotify.Track{{ID: "wrong-world"}}}, nil, false)
	if !a.liked["wrong-world"] || a.liked["unliked"] {
		t.Fatalf("hearts did not follow the liked songs: %v", a.liked)
	}
	a.receivePlaylist("spotify:playlist:1", &spotify.Playlist{ID: "spotify:playlist:1", Tracks: []spotify.Track{{ID: "other"}}}, nil, false)
	if a.liked["other"] || !a.liked["wrong-world"] {
		t.Fatalf("another list changed the hearts: %v", a.liked)
	}
}

// Liked songs that could not be loaded at sign in, which left every heart
// empty for good, are asked for again as the library refreshes; once
// loaded they are not, the whole list being pages of requests.
func TestLikedAskedAgainUntilLoaded(t *testing.T) {
	defer func(old func(string) (*spotify.Library, error)) { userLibrary = old }(userLibrary)
	defer func(old func() ([]string, error)) { likedIDs = old }(likedIDs)
	libs := make(chan struct{}, 4)
	userLibrary = func(string) (*spotify.Library, error) { libs <- struct{}{}; return &spotify.Library{}, nil }
	asked := make(chan struct{}, 4)
	likedIDs = func() ([]string, error) { asked <- struct{}{}; return nil, errors.New("offline") }

	a := newTestApp()
	a.liked = nil
	a.refreshMine(time.Now().Add(time.Minute))
	select {
	case <-asked:
	case <-time.After(time.Second):
		t.Fatal("liked songs not asked for again")
	}

	a = newTestApp()
	a.refreshMine(time.Now().Add(time.Minute))
	<-libs
	<-libs
	time.Sleep(50 * time.Millisecond) // the liked songs would be asked on a goroutine of their own
	select {
	case <-asked:
		t.Fatal("loaded liked songs asked for again")
	default:
	}
}
