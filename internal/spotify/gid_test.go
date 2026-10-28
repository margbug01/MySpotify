package spotify

import "testing"

// The bits of an ID are base 62 with the digits first, then lowercase, then
// uppercase: read with A-Z first, the same ID names other bits, which
// Spotify answers 404 for. These two are one track's, checked against
// spclient's metadata.
func TestGIDReadsTheWebPlayersAlphabet(t *testing.T) {
	const id, gid = "0VjIjW4GlUZAMYd2vXMi3b", "1e6028dab84d4a17a1b3fb28ad51df5d"
	got, err := GID(id)
	if err != nil || got != gid {
		t.Fatalf("GID(%q) = %q, %v; want %q", id, got, err, gid)
	}
	back, err := Base62(gid)
	if err != nil || back != id {
		t.Fatalf("Base62(%q) = %q, %v; want %q", gid, back, err, id)
	}
}

func TestGIDTakesWhatTheManifestsName(t *testing.T) {
	const gid = "1e6028dab84d4a17a1b3fb28ad51df5d"
	for _, in := range []string{
		gid,
		"1E6028DAB84D4A17A1B3FB28AD51DF5D",
		"spotify:track:0VjIjW4GlUZAMYd2vXMi3b",
		"0VjIjW4GlUZAMYd2vXMi3b",
	} {
		if got, err := GID(in); err != nil || got != gid {
			t.Errorf("GID(%q) = %q, %v; want %q", in, got, err, gid)
		}
	}
	for _, bad := range []string{"", "abc", "zzzzzzzzzzzzzzzzzzzzzz", "spotify:track:"} {
		if got, err := GID(bad); err == nil {
			t.Errorf("GID(%q) = %q, want an error", bad, got)
		}
	}
}
