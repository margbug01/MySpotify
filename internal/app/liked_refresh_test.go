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
	a.receivePlaylist(spotify.LikedURI, &spotify.Playlist{ID: spotify.LikedURI, Tracks: []spotify.Track{{ID: "wrong-world"}}}, nil, false, time.Now())
	if !a.liked["wrong-world"] || a.liked["unliked"] {
		t.Fatalf("hearts did not follow the liked songs: %v", a.liked)
	}
	a.receivePlaylist("spotify:playlist:1", &spotify.Playlist{ID: "spotify:playlist:1", Tracks: []spotify.Track{{ID: "other"}}}, nil, false, time.Now())
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

// A song liked or unliked here keeps its heart as a list of the liked songs
// asked for before Spotify took it comes in: the list loading as one was
// liked showed its heart empty again. A list asked for after it goes as it
// is.
func TestLikesOutliveListsAskedBefore(t *testing.T) {
	calls := make(chan bool, 2)
	likeSong = func(_ string, like bool) error { calls <- like; return nil }
	defer func() { likeSong = spotify.Like }()
	defer func() { <-calls; <-calls }()
	a := newTestApp()
	a.liked = map[string]bool{"gone": true}
	before := time.Now()
	a.toggleLike(&spotify.Track{ID: "new"})
	a.toggleLike(&spotify.Track{ID: "gone"})
	a.likedFrom([]string{"gone"}, before) // as Spotify is asked
	if !a.liked["new"] || a.liked["gone"] {
		t.Fatalf("asking: hearts %v", a.liked)
	}
	took := before.Add(time.Second)
	a.liking["new"].took, a.liking["gone"].took = took, took
	a.receivePlaylist(spotify.LikedURI, &spotify.Playlist{ID: spotify.LikedURI, Tracks: []spotify.Track{{ID: "gone"}}}, nil, false, before)
	if !a.liked["new"] || a.liked["gone"] {
		t.Fatalf("a list from before: hearts %v", a.liked)
	}
	// Changed elsewhere since.
	a.likedFrom([]string{"gone"}, took.Add(time.Second))
	if a.liked["new"] || !a.liked["gone"] || len(a.liking) != 0 {
		t.Fatalf("a list from after: hearts %v, held %d", a.liked, len(a.liking))
	}
}
