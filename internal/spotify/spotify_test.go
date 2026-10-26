package spotify

import (
	"testing"
	"time"
)

// Album art asks for the size coded in its ID, other pictures stay as they
// are, and each says the size wanted after "#".
func TestSized(t *testing.T) {
	for _, c := range []struct {
		in   string
		n    int
		want string
	}{
		{"https://i.scdn.co/image/ab67616d00001e024cacd8997b3641fca4bbff99", 600, "https://i.scdn.co/image/ab67616d0000b2734cacd8997b3641fca4bbff99#600"},
		{"https://i.scdn.co/image/ab67616d0000b2734cacd8997b3641fca4bbff99", 96, "https://i.scdn.co/image/ab67616d00001e024cacd8997b3641fca4bbff99#96"},
		{"https://i.scdn.co/image/ab67616d0000b2734cacd8997b3641fca4bbff99", 40, "https://i.scdn.co/image/ab67616d000048514cacd8997b3641fca4bbff99#40"},
		{"https://mosaic.scdn.co/640/ab67616d00001e02aa", 200, "https://mosaic.scdn.co/300/ab67616d00001e02aa#200"},
		{"https://image-cdn-ak.spotifycdn.com/image/ab67706c0000da84aa", 600, "https://image-cdn-ak.spotifycdn.com/image/ab67706c0000da84aa#600"},
		{"https://i.scdn.co/image/ab6761610000e5eb438e5730e9e86121f329d2dd", 78, "https://i.scdn.co/image/ab6761610000f178438e5730e9e86121f329d2dd#78"},
		{"https://i.scdn.co/image/ab6761610000e5eb438e5730e9e86121f329d2dd", 300, "https://i.scdn.co/image/ab67616100005174438e5730e9e86121f329d2dd#300"},
		{"", 600, ""},
	} {
		if got := Sized(c.in, c.n); got != c.want {
			t.Errorf("Sized(%q, %d) = %q, want %q", c.in, c.n, got, c.want)
		}
	}
}

// Line-synced lyrics come out as LRC; unsynced ones as nothing.
func TestLyricsLRC(t *testing.T) {
	got, err := lyricsLRC([]byte(`{"lyrics":{"syncType":"LINE_SYNCED","lines":[{"startTimeMs":"14170","words":"Yeah"},{"startTimeMs":"75005","words":"♪"},{"startTimeMs":"bad","words":"x"}]}}`))
	if err != nil || got != "[00:14.17]Yeah\n[01:15.00]\n" {
		t.Fatalf("got %q, %v", got, err)
	}
	got, _ = lyricsLRC([]byte(`{"lyrics":{"syncType":"UNSYNCED","lines":[{"startTimeMs":"0","words":"a"}]}}`))
	if got != "" {
		t.Fatalf("unsynced: got %q", got)
	}
}

// The token the web player fetched wins; a bearer seen on a request is
// taken for 45 minutes.
func TestParseIdentity(t *testing.T) {
	id, ok := ParseIdentity(`{"token":{"accessToken":"a","accessTokenExpirationTimestampMs":1792989083973,"isAnonymous":false},"clientToken":"c","bearer":"b"}`)
	if !ok || id.Token != "a" || id.ClientToken != "c" || id.Expires.UnixMilli() != 1792989083973 || id.Anonymous {
		t.Fatalf("token: %+v %v", id, ok)
	}
	id, ok = ParseIdentity(`{"bearer":"b"}`)
	if !ok || id.Token != "b" || time.Until(id.Expires) < 44*time.Minute {
		t.Fatalf("bearer: %+v %v", id, ok)
	}
	if _, ok := ParseIdentity(`{"clientToken":"c"}`); ok {
		t.Fatal("no token, yet ok")
	}
}
