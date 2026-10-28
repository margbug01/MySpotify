package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"vinyl/internal/player"
	"vinyl/internal/spotify"
)

var errOffline = errors.New("context deadline exceeded")

// signInScript stands in for the network: each sign-in asked of it takes
// the next of answers, and each wait for the next try is kept in due.
type signInScript struct {
	answers []error // nil signs in as listener
	asked   int
	waits   []time.Duration
	due     []func()
}

func (s *signInScript) install(t *testing.T) {
	ask, after := askAccount, signInAfter
	t.Cleanup(func() { askAccount, signInAfter = ask, after })
	askAccount = func(_ *app, took func(*spotify.Profile, error)) {
		err := s.answers[s.asked]
		s.asked++
		if err != nil {
			took(nil, err)
			return
		}
		took(&spotify.Profile{UserID: "listener", Nickname: "又登上了"}, nil)
	}
	signInAfter = func(_ *app, d time.Duration, fn func()) {
		s.waits = append(s.waits, d)
		s.due = append(s.due, fn)
	}
}

// The app opened before the network was up, showing the listener kept from
// last time: signing in is tried again, waiting longer each time, until it
// gets through, and then signs in, which starts Connect. It was never tried
// again, and Connect never started for as long as the app ran.
func TestSignInIsTriedAgainUntilItGetsThrough(t *testing.T) {
	s := &signInScript{answers: []error{errOffline, nil}}
	s.install(t)
	a := newTestApp()
	a.restored = true // no session.json to read
	a.homeLoading = true
	kept := a.profile

	a.tookSignIn(a.accountGen, 0, nil, errOffline)
	if a.profile != kept || a.homeLoading {
		t.Fatalf("offline: profile %v, loading %v; want the one kept, shown", a.profile, a.homeLoading)
	}
	if len(s.due) != 1 {
		t.Fatalf("offline: %d tries due, want 1", len(s.due))
	}
	s.due[0]()
	if len(s.due) != 2 {
		t.Fatalf("offline again: %d tries due, want 2", len(s.due))
	}
	s.due[1]()
	if a.profile == kept || a.profile.Nickname != "又登上了" {
		t.Fatalf("through: profile %+v, want the one signed in", a.profile)
	}
	if len(s.due) != 2 || s.asked != 2 {
		t.Fatalf("signed in: %d tries due, %d asked; want no more", len(s.due), s.asked)
	}
	if want := []time.Duration{signInFirst, 2 * signInFirst}; len(s.waits) != 2 || s.waits[0] != want[0] || s.waits[1] != want[1] {
		t.Errorf("waits %v, want %v", s.waits, want)
	}
}

// The waits grow to signInLast and stay there.
func TestSignInWaitsNoLongerThanTheLast(t *testing.T) {
	answers := make([]error, 12)
	for i := range answers {
		answers[i] = errOffline
	}
	s := &signInScript{answers: answers}
	s.install(t)
	a := newTestApp()
	a.tookSignIn(a.accountGen, 0, nil, errOffline)
	for range 11 {
		s.due[len(s.due)-1]()
	}
	for i, w := range s.waits {
		if w > signInLast || i > 0 && w < s.waits[i-1] {
			t.Fatalf("waits %v", s.waits)
		}
	}
	if last := s.waits[len(s.waits)-1]; last != signInLast {
		t.Errorf("last wait %v, want %v", last, signInLast)
	}
}

// Signed out, or in, while a try was due: what it finds is no one's, and
// it is not asked.
func TestSignInDueIsDroppedOnceSignedInOrOut(t *testing.T) {
	s := &signInScript{answers: []error{nil}}
	s.install(t)
	a := newTestApp()
	a.restored = true
	a.tookSignIn(a.accountGen, 0, nil, errOffline)
	a.signedIn(nil) // as signOut does
	s.due[0]()
	if s.asked != 0 || a.profile != nil {
		t.Fatalf("signed out meanwhile: asked %d, profile %v", s.asked, a.profile)
	}

	// Asked before the listener signed in in the window, and answered after.
	b := newTestApp()
	b.restored = true
	gen := b.accountGen
	b.signedIn(&spotify.Profile{UserID: "someone-else"})
	b.tookSignIn(gen, 0, nil, errOffline)
	if len(s.due) != 1 || b.profile.UserID != "someone-else" {
		t.Fatalf("signed in meanwhile: %d tries due, profile %v", len(s.due)-1, b.profile)
	}
}

// Turned down on a later try, the listener is signed out there: the
// sign-in page, and no more tries.
func TestSignInTurnedDownLaterSignsOut(t *testing.T) {
	s := &signInScript{answers: []error{spotify.ErrSignedOut}}
	s.install(t)
	a := newTestApp()
	a.restored = true
	a.tookSignIn(a.accountGen, 0, nil, errOffline)
	s.due[0]()
	if a.profile != nil || len(s.due) != 1 {
		t.Fatalf("turned down: profile %v, %d tries due", a.profile, len(s.due))
	}
}

// With nothing kept to show, the visitor's pages show while it is tried
// again, once rather than at every try.
func TestSignInWithNothingKeptShowsTheVisitorsPagesOnce(t *testing.T) {
	s := &signInScript{answers: []error{errOffline, nil}}
	s.install(t)
	a := newTestApp()
	a.profile = nil
	a.restored = true
	a.tookSignIn(a.accountGen, 0, nil, errOffline)
	if len(s.due) != 1 {
		t.Fatalf("%d tries due, want 1", len(s.due))
	}
	gen := a.accountGen
	s.due[0]()
	if a.accountGen != gen || a.profile != nil {
		t.Fatalf("offline again: signed in or out again (%d → %d), profile %v", gen, a.accountGen, a.profile)
	}
	s.due[1]()
	if a.profile == nil || a.profile.UserID != "listener" {
		t.Fatalf("through: profile %v", a.profile)
	}
}

// Signing in picks up where playback was left, unless the listener played
// something meanwhile: what they chose stays.
func TestSessionLeavesWhatPlaysAlready(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.json")
	defer func(p func() (string, error)) { sessionPath = p }(sessionPath)
	sessionPath = func() (string, error) { return path, nil }
	b, _ := json.Marshal(player.Session{Queue: []spotify.Track{{ID: "kept", Name: "上次的", Duration: 200000}}, Order: []int{0}})
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}

	a := newTestApp()
	a.restoreSession()
	if cur := a.pl.Current(); cur == nil || cur.ID != "kept" {
		t.Fatalf("nothing playing: %v, want the one kept", cur)
	}

	c := newTestApp()
	c.pl.Cue([]spotify.Track{{ID: "chosen", Name: "刚点的", Duration: 200000}})
	c.restoreSession()
	if cur := c.pl.Current(); cur == nil || cur.ID != "chosen" {
		t.Fatalf("playing already: %v, want the one chosen", cur)
	}
}
