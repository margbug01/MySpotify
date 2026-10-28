package player

import (
	"encoding/hex"
	"os"
	"testing"
)

// A track's own init section: its moov, with the protection the licence
// matches (tenc: cenc, an 8-byte IV per sample, and the key the licence is
// for). This is the shape Spotify serves, and what telling a track the key
// system can play from one it cannot leans on.
func TestEncryptionOfATrack(t *testing.T) {
	b, err := os.ReadFile("testdata/init-cenc.mp4")
	if err != nil {
		t.Fatal(err)
	}
	enc, err := encryptionOf(b)
	if err != nil {
		t.Fatalf("encryptionOf: %v", err)
	}
	if enc.Scheme != "cenc" {
		t.Errorf("Scheme = %q, want cenc", enc.Scheme)
	}
	if enc.IVSize != 8 {
		t.Errorf("IVSize = %d, want 8", enc.IVSize)
	}
	if got := hex.EncodeToString(enc.KeyID); got != "f6e2ff1804615a9cb45b63cbba993d3b" {
		t.Errorf("KeyID = %s, want the track's key", got)
	}
}

// Bytes that are not a file's init section: a page of anything else is not
// read as an encrypted track.
func TestEncryptionOfSomethingElse(t *testing.T) {
	if _, err := encryptionOf([]byte("not a moov, no tenc in it")); err == nil {
		t.Fatal("encryptionOf read encryption out of bytes with none")
	}
}
