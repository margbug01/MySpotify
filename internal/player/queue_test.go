package player

import (
	"slices"
	"testing"
	"time"

	"vinyl/internal/spotify"
)

// played returns the IDs of p's queue in the order it plays them.
func played(p *Player) []string {
	order, _ := p.PlayOrder()
	ids := make([]string, len(order))
	for i, k := range order {
		ids[i] = p.Queue()[k].ID
	}
	return ids
}

// A song to play next goes right after the one playing, a queued one moved
// there; the one playing stays where it is.
func TestPlayNext(t *testing.T) {
	p := New(func(func()) {})
	p.Cue([]spotify.Track{{ID: "1"}, {ID: "2"}, {ID: "3"}, {ID: "4"}})
	p.at, p.track = 1, &p.queue[1]
	p.PlayNext(spotify.Track{ID: "9"})
	if got := played(p); !slices.Equal(got, []string{"1", "2", "9", "3", "4"}) {
		t.Fatalf("after a new song: %v", got)
	}
	p.PlayNext(spotify.Track{ID: "1"})
	if got := played(p); !slices.Equal(got, []string{"2", "1", "9", "3", "4"}) || p.Current().ID != "2" {
		t.Fatalf("after one played before: %v, playing %s", got, p.Current().ID)
	}
	p.PlayNext(spotify.Track{ID: "4"})
	p.PlayNext(spotify.Track{ID: "2"})
	if got := played(p); !slices.Equal(got, []string{"2", "4", "1", "9", "3"}) || len(p.Queue()) != 5 {
		t.Fatalf("after one later and the one playing: %v, queue of %d", got, len(p.Queue()))
	}
}

// A song added to the queue plays last, and only once.
func TestAddToQueue(t *testing.T) {
	p := New(func(func()) {})
	p.Cue([]spotify.Track{{ID: "1"}, {ID: "2"}})
	if !p.AddToQueue(spotify.Track{ID: "9"}) || p.AddToQueue(spotify.Track{ID: "2"}) {
		t.Fatal("added a queued song, or not a new one")
	}
	if got := played(p); !slices.Equal(got, []string{"1", "2", "9"}) {
		t.Fatalf("queue %v", got)
	}
}

// An episode's audio is asked for by its own URI, not a track's.
func TestEpisodeResolvesByItsURI(t *testing.T) {
	got := make(chan string, 1)
	real := resolve
	resolve = func(uri string) (*spotify.Media, error) { got <- uri; return nil, spotify.ErrNotFound }
	defer func() { resolve = real }()
	p := New(func(func()) {})
	p.PlayQueue([]spotify.Track{{ID: "e1", Episode: true}}, 0)
	select {
	case uri := <-got:
		if uri != "spotify:episode:e1" {
			t.Errorf("asked for %s", uri)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("not asked")
	}
}
