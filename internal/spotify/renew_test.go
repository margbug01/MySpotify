package spotify

import (
	"sync/atomic"
	"testing"
	"time"
)

// /api/token answers with the token it gave until that runs out: one
// handed back with less than renewAhead left is used until just before it
// ends, where every request once renewed it again.
func TestARenewalHandedBackIsKept(t *testing.T) {
	oldRenew := Renew
	defer func() { Renew = oldRenew; SignOut() }()
	var renewals atomic.Int32
	ends := time.Now().Add(90 * time.Second)
	Renew = func() (Identity, error) {
		renewals.Add(1)
		return Identity{Token: "same", ClientToken: "c", Expires: ends}, nil
	}
	SetIdentity(Identity{Token: "same", ClientToken: "c", Expires: ends})
	for range 20 {
		id, err := CurrentIdentity()
		if err != nil || id.Token != "same" {
			t.Fatalf("CurrentIdentity = %+v, %v", id, err)
		}
	}
	if n := renewals.Load(); n != 1 {
		t.Fatalf("%d renewals for 20 requests, want the one that handed it back", n)
	}

	// In its last seconds, it is renewed again: by then Spotify gives a new
	// one.
	ends = time.Now().Add(3 * time.Second)
	SetIdentity(Identity{Token: "same", ClientToken: "c", Expires: ends})
	Renew = func() (Identity, error) {
		renewals.Add(1)
		return Identity{Token: "next", ClientToken: "c", Expires: time.Now().Add(time.Hour)}, nil
	}
	if id, err := CurrentIdentity(); err != nil || id.Token != "next" {
		t.Fatalf("CurrentIdentity = %+v, %v; want the next token", id, err)
	}
}
