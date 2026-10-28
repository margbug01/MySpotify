package app

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"testing"
)

// The cookies seal under a key the login keychain keeps, and open again;
// a file the key did not seal does not. It uses the keychain, so it runs
// only when VINYL_KEYCHAIN is set, under an item of its own, removed after.
func TestSealWithTheKeychain(t *testing.T) {
	if os.Getenv("VINYL_KEYCHAIN") == "" {
		t.Skip("set VINYL_KEYCHAIN=1 to use the login keychain")
	}
	old := keychainService
	keychainService = "dev.margbug.vinyl.test-" + strconv.Itoa(os.Getpid())
	keyMu.Lock()
	cookieKey = nil
	keyMu.Unlock()
	t.Cleanup(func() {
		exec.Command("/usr/bin/security", "delete-generic-password", "-s", keychainService, "-a", keychainAccount).Run()
		keychainService = old
		keyMu.Lock()
		cookieKey = nil
		keyMu.Unlock()
	})
	plain := []byte("sp_dc=secret; sp_key=k")
	sealed, err := seal(plain)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if bytes.Contains(sealed, plain) {
		t.Fatal("sealed in the clear")
	}
	// From the keychain again, as at the next start.
	keyMu.Lock()
	cookieKey = nil
	keyMu.Unlock()
	if got, err := unseal(sealed); err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("unseal = %q, %v", got, err)
	}
	if _, err := unseal(plain); !errors.Is(err, errUnreadable) {
		t.Fatalf("unseal of the clear text: %v, want errUnreadable", err)
	}
}
