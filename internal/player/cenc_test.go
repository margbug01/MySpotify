package player

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"os"
	"testing"
)

// A track's own init section: its moov, with the protection the licence
// matches (tenc: cenc, an 8-byte IV per sample, and the key the licence is
// for). This is the shape Spotify serves where Widevine plays, and what
// telling a track the key system can play from one it cannot leans on.
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

// The start of a file_ids_mp4_cbcs file, what FairPlay plays: ftyp, then a
// moov whose tenc is version 1 and gives every sample the same IV, which
// it writes after the key's ID rather than in each sample.
func TestEncryptionOfAFairPlayTrack(t *testing.T) {
	b, err := os.ReadFile("testdata/init-cbcs.mp4")
	if err != nil {
		t.Fatal(err)
	}
	enc, err := encryptionOf(b)
	if err != nil {
		t.Fatalf("encryptionOf: %v", err)
	}
	if enc.Scheme != "cbcs" || enc.IVSize != 0 {
		t.Errorf("encryption = %s with a %d-byte IV, want cbcs with a constant one", enc.Scheme, enc.IVSize)
	}
	if got := hex.EncodeToString(enc.KeyID); got != "0b7be0d76862a9c0d8634586e619ebba" {
		t.Errorf("KeyID = %s, want the track's key", got)
	}
}

// A prefetched head goes on past the moov, into a minute of samples: what
// those bytes happen to spell is not read as the file's protection.
func TestEncryptionOfAHeadReadsOnlyItsMoov(t *testing.T) {
	cbcs, err := os.ReadFile("testdata/init-cbcs.mp4")
	if err != nil {
		t.Fatal(err)
	}
	// Samples that spell a cenc tenc and schm, as random bytes might.
	fake := bytes.Join([][]byte{
		box("schm", append([]byte{0, 0, 0, 0}, []byte("cenc\x00\x01\x00\x00")...)),
		box("tenc", append(make([]byte, 6), append([]byte{1, 8}, make([]byte, 16)...)...)),
	}, nil)
	head := append(append(append([]byte(nil), cbcs...), box("moof", make([]byte, 64))...), box("mdat", fake)...)
	if enc, err := encryptionOf(head); err != nil || enc.Scheme != "cbcs" {
		t.Fatalf("encryptionOf(head) = %+v, %v; want the moov's cbcs", enc, err)
	}

	// And a file whose moov names no protection is not read as protected by
	// what its samples spell.
	plain := append(append(box("ftyp", []byte("isom")), box("moov", box("mvhd", make([]byte, 100)))...), box("mdat", fake)...)
	if enc, err := encryptionOf(plain); err == nil {
		t.Fatalf("encryptionOf read %+v out of the samples", enc)
	}
}

// Bytes that are not a file's init section: a page of anything else is not
// read as an encrypted track.
func TestEncryptionOfSomethingElse(t *testing.T) {
	for _, b := range [][]byte{
		[]byte("not a moov, no tenc in it"),
		{0, 0, 0, 0},
		box("moov", box("tenc", make([]byte, 10))), // cut short
	} {
		if _, err := encryptionOf(b); err == nil {
			t.Fatalf("encryptionOf read encryption out of %q", b)
		}
	}
}

// box is an ISO BMFF box of type typ around body.
func box(typ string, body []byte) []byte {
	b := make([]byte, 8, 8+len(body))
	binary.BigEndian.PutUint32(b, uint32(8+len(body)))
	copy(b[4:], typ)
	return append(b, body...)
}
