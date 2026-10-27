package app

import (
	"errors"
	"testing"
	"time"

	"vinyl/internal/spotify"
)

// A refreshed library replaces stale sidebar entries, including additions and removals.
func TestLibraryRefreshReplacesLists(t *testing.T) {
	a := newTestApp()
	a.mineGen, a.mineLoading = 3, true
	a.saved = []spotify.Playlist{{ID: "removed"}}
	lib := &spotify.Library{
		Made:  []spotify.Playlist{{ID: "created", Name: "From web"}},
		Saved: []spotify.Playlist{{ID: "followed"}},
	}
	a.receiveMine("listener", 3, lib, nil)
	if a.mineLoading || len(a.mine) != 1 || a.mine[0].ID != "created" || len(a.saved) != 1 || a.saved[0].ID != "followed" {
		t.Fatal("sidebar did not adopt the refreshed library")
	}
	a.receiveMine("listener", 3, &spotify.Library{}, errors.New("offline"))
	if len(a.saved) != 1 {
		t.Fatal("failed refresh erased the library")
	}
	a.receiveMine("listener", 3, &spotify.Library{}, nil)
	if len(a.saved) != 0 || len(a.mine) != 0 {
		t.Fatal("removals were not synchronized")
	}
}

// Late results from a previous request or account cannot overwrite the current library.
func TestLibraryRefreshIgnoresStaleResults(t *testing.T) {
	for _, mode := range []string{"generation", "account", "signed-out", "quitting"} {
		a := newTestApp()
		a.mineGen, a.mineLoading = 2, true
		a.saved = []spotify.Playlist{{ID: "keep"}}
		uid, gen := "listener", 2
		switch mode {
		case "generation":
			gen = 1
		case "account":
			uid = "other"
		case "signed-out":
			a.profile = nil
		case "quitting":
			a.quitting = true
		}
		a.receiveMine(uid, gen, &spotify.Library{}, nil)
		if len(a.saved) != 1 || !a.mineLoading {
			t.Fatalf("%s result changed current state", mode)
		}
	}
}

// Focus and periodic checks are throttled, while edits during a refresh request one follow-up.
func TestLibraryRefreshThrottleAndFollowUp(t *testing.T) {
	old := userLibrary
	defer func() { userLibrary = old }()
	calls := make(chan string, 4)
	userLibrary = func(uid string) (*spotify.Library, error) {
		calls <- uid
		return &spotify.Library{}, nil
	}
	waitCall := func() {
		t.Helper()
		select {
		case uid := <-calls:
			if uid != "listener" {
				t.Fatal(uid)
			}
		case <-time.After(time.Second):
			t.Fatal("no refresh request")
		}
	}
	a := newTestApp()
	now := time.Now()
	a.mineChecked = now
	a.refreshMine(now.Add(4 * time.Second))
	if a.mineLoading {
		t.Fatal("focus burst was not throttled")
	}
	a.refreshMine(now.Add(6 * time.Second))
	waitCall()
	gen := a.mineGen
	a.refreshMine(now.Add(time.Minute))
	if a.mineAgain || a.mineGen != gen {
		t.Fatal("poll duplicated an in-flight request")
	}
	a.loadMine("listener")
	if !a.mineAgain {
		t.Fatal("edit during refresh was dropped")
	}
	a.saved = []spotify.Playlist{{ID: "keep"}}
	a.receiveMine("listener", gen, &spotify.Library{}, nil)
	waitCall()
	if a.mineAgain || !a.mineLoading || a.mineGen != gen+1 || len(a.saved) != 1 {
		t.Fatal("edit did not supersede stale result with a single follow-up")
	}
	a.receiveMine("listener", a.mineGen, &spotify.Library{}, nil)
	a.quitting = true
	a.refreshMine(now.Add(time.Hour))
	if a.mineLoading {
		t.Fatal("refresh started after quit")
	}
}
