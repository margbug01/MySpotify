package spotify

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Signed out, requests go as the anonymous visitor instead of failing: the
// pages everyone's load without anyone signing in.
func TestRequestsGoAsTheGuestSignedOut(t *testing.T) {
	oldClient, oldGuest := Client, RenewGuest
	defer func() { Client, RenewGuest = oldClient, oldGuest; SignOut() }()
	SignOut()

	var seen []string
	Client = &http.Client{Transport: lyricsTransport(func(req *http.Request) (*http.Response, error) {
		seen = append(seen, req.Header.Get("Authorization")+"|"+req.Header.Get("client-token"))
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":{}}`)), Header: http.Header{}}, nil
	})}
	var asked atomic.Int32
	RenewGuest = func() (Identity, error) {
		asked.Add(1)
		return Identity{Token: "guest", ClientToken: "ct", Expires: time.Now().Add(time.Hour)}, nil
	}

	if _, err := spGet("/x", "application/json"); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 1 || seen[0] != "Bearer guest|ct" {
		t.Fatalf("sent %q, want the guest's", seen)
	}
	// The guest's is kept: a second request does not ask for another.
	if _, err := spGet("/y", "application/json"); err != nil {
		t.Fatal(err)
	}
	if n := asked.Load(); n != 1 {
		t.Fatalf("asked %d times for a guest, want 1", n)
	}
}

// A guest's request turned down once is retried as a new guest, as a
// listener's is retried as themselves.
func TestAStaleGuestIsReplacedOnce(t *testing.T) {
	oldClient, oldGuest := Client, RenewGuest
	defer func() { Client, RenewGuest = oldClient, oldGuest; SignOut() }()
	SignOut()

	var calls atomic.Int32
	Client = &http.Client{Transport: lyricsTransport(func(req *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader(`{}`)), Header: http.Header{}}, nil
		}
		if got := req.Header.Get("Authorization"); got != "Bearer second" {
			t.Errorf("retried as %q, want the new guest", got)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":{}}`)), Header: http.Header{}}, nil
	})}
	var guests atomic.Int32
	RenewGuest = func() (Identity, error) {
		if guests.Add(1) == 1 {
			return Identity{Token: "first", Expires: time.Now().Add(time.Hour)}, nil
		}
		return Identity{Token: "second", Expires: time.Now().Add(time.Hour)}, nil
	}

	if _, err := spGet("/x", "application/json"); err != nil {
		t.Fatal(err)
	}
	if n := calls.Load(); n != 2 {
		t.Fatalf("asked %d times, want 2", n)
	}
}

// Without a way to be a guest, requests signed out fail as they did: the
// app says so rather than asking Spotify with no token at all.
func TestNoGuestKeepsSigningOut(t *testing.T) {
	oldClient, oldGuest := Client, RenewGuest
	defer func() { Client, RenewGuest = oldClient, oldGuest; SignOut() }()
	SignOut()
	RenewGuest = nil

	var called atomic.Bool
	Client = &http.Client{Transport: lyricsTransport(func(req *http.Request) (*http.Response, error) {
		called.Store(true)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{}`)), Header: http.Header{}}, nil
	})}
	if _, err := spGet("/x", "application/json"); !errors.Is(err, ErrSignedOut) {
		t.Fatalf("err is %v, want ErrSignedOut", err)
	}
	if called.Load() {
		t.Fatal("asked Spotify with nobody to ask as")
	}
}

// Guests says whether browsing works signed out, as the app sets it.
func TestGuestsFollowsTheRenewal(t *testing.T) {
	old := RenewGuest
	defer func() { RenewGuest = old }()
	RenewGuest = nil
	if Guests() {
		t.Fatal("guests without a way to be one")
	}
	RenewGuest = func() (Identity, error) { return Identity{}, nil }
	if !Guests() {
		t.Fatal("no guests with a way to be one")
	}
}

// The guest's is asked for once, however many requests want it at once.
func TestOneGuestAskAtATime(t *testing.T) {
	oldGuest := RenewGuest
	defer func() { RenewGuest = oldGuest; SignOut() }()
	SignOut()

	var asked atomic.Int32
	release := make(chan struct{})
	RenewGuest = func() (Identity, error) {
		asked.Add(1)
		<-release
		return Identity{Token: "guest", Expires: time.Now().Add(time.Hour)}, nil
	}

	done := make(chan Identity, 8)
	for range 8 {
		go func() {
			id, _ := guestIdentity()
			done <- id
		}()
	}
	// Let them all be waiting before the ask answers.
	time.Sleep(50 * time.Millisecond)
	close(release)
	for range 8 {
		if id := <-done; id.Token != "guest" {
			t.Fatalf("got %q, want the guest's", id.Token)
		}
	}
	if n := asked.Load(); n != 1 {
		t.Fatalf("asked %d times, want 1", n)
	}
}
