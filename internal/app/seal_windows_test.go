package app

import (
	"bytes"
	"errors"
	"testing"
)

// The cookies seal with DPAPI under the user's login, and open again; a
// file DPAPI did not seal does not.
func TestSealWithDPAPI(t *testing.T) {
	plain := []byte("sp_dc=secret; sp_key=k")
	sealed, err := seal(plain)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if bytes.Contains(sealed, plain) {
		t.Fatal("sealed in the clear")
	}
	if got, err := unseal(sealed); err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("unseal = %q, %v", got, err)
	}
	if _, err := unseal(plain); !errors.Is(err, errUnreadable) {
		t.Fatalf("unseal of the clear text: %v, want errUnreadable", err)
	}
}
