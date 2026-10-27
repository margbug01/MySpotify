package spotify

import (
	"testing"
	"time"
)

// The secret the web player carries unmasks to a 60-digit string: the
// algorithm joins the XORed code points as digits and reads those back as
// the key, so the key is ASCII digits, not the bytes they spell.
func TestTotpSecretUnmasks(t *testing.T) {
	key := totpSecrets[0].unmask()
	if len(key) != 60 {
		t.Fatalf("key is %d bytes, want 60: %q", len(key), key)
	}
	for _, c := range key {
		if c < '0' || c > '9' {
			t.Fatalf("key %q is not all digits", key)
		}
	}
	// The digits are the code points XORed with this, in order.
	const want = "376136387538459893883312310911992847112448894410210511297108"
	if string(key) != want {
		t.Fatalf("key is %q, want %q", key, want)
	}
}

// Each version unmasks to a key of its own, all digits.
func TestEverySecretUnmasks(t *testing.T) {
	if len(totpSecrets) != 3 {
		t.Fatalf("%d secrets, want 3", len(totpSecrets))
	}
	seen := map[string]bool{}
	for _, s := range totpSecrets {
		key := s.unmask()
		if len(key) == 0 {
			t.Fatalf("version %d unmasked to nothing", s.version)
		}
		if seen[string(key)] {
			t.Fatalf("version %d unmasked to another version's key", s.version)
		}
		seen[string(key)] = true
	}
}

// HOTP is RFC 4226: with the RFC's key and counter 0, the code is 755224.
func TestHotpMatchesTheRFC(t *testing.T) {
	// The RFC's key is "12345678901234567890", as bytes.
	key := []byte("12345678901234567890")
	for counter, want := range map[uint64]string{
		0: "755224", 1: "287082", 2: "359152", 3: "969429", 4: "338314",
	} {
		if got := hotp(key, counter); got != want {
			t.Errorf("hotp(%d) = %s, want %s", counter, got, want)
		}
	}
}

// The 30-second step: two times within one step share a code, and the next
// step does not.
func TestTotpStepsEveryThirtySeconds(t *testing.T) {
	key := totpSecrets[0].unmask()
	at := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	same := []int64{0, 1, 29_999}
	for _, ms := range same[1:] {
		if a, b := totp(key, at.UnixMilli()), totp(key, at.UnixMilli()+ms); a != b {
			t.Errorf("+%dms gave %s, want %s", ms, b, a)
		}
	}
	if a, b := totp(key, at.UnixMilli()), totp(key, at.UnixMilli()+30_000); a == b {
		t.Errorf("the next step gave the same code %s", a)
	}
}

// The query names the version and both codes, as the endpoint wants them.
func TestTokenQuery(t *testing.T) {
	key := []byte("12345678901234567890")
	// The second step of the epoch: counter 1, whose code the RFC gives.
	now := time.Unix(30, 0)
	got := tokenQuery(key, 61, now, now.Add(3*time.Second))
	const want = "reason=init&productType=web_player&totp=287082&totpServer=287082&totpVer=61"
	if got != want {
		t.Fatalf("query is %q, want %q", got, want)
	}
	// A different server clock gives a different code once the step turns.
	later := tokenQuery(key, 61, now, now.Add(30*time.Second))
	if later == got {
		t.Fatal("a server clock a step ahead gave the same query")
	}
}
