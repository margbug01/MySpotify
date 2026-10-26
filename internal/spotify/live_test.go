package spotify

import (
	"os"
	"strings"
	"testing"
)

// Calls Spotify as the identity in the file LIVE names (window.__vinyl as
// JSON, as vinyl --webtoken saves it), and logs what comes back.
func TestLive(t *testing.T) {
	path := os.Getenv("LIVE")
	if path == "" {
		t.Skip("LIVE not set")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	id, ok := ParseIdentity(string(b))
	if !ok {
		t.Fatal("no identity in", path)
	}
	SetIdentity(id)

	me, err := Account()
	if err != nil {
		t.Fatal("account:", err)
	}
	t.Logf("account: %+v", *me)

	home, err := HomeFeed()
	if err != nil {
		t.Fatal("home:", err)
	}
	t.Logf("home: %q daily=%s forYou=%d featured=%d", home.Greeting, home.Daily, len(home.ForYou), len(home.Featured))
	if home.Daily == "" || len(home.Featured) == 0 {
		t.Error("home lacks a daily mix or featured lists")
	}

	daily, err := PlaylistDetail(home.Daily)
	if err != nil {
		t.Fatal("daily:", err)
	}
	t.Logf("daily: %s by %s, %d/%d tracks, cover %v", daily.Name, daily.Creator.Nickname, len(daily.Tracks), daily.TrackCount, daily.CoverURL != "")
	if len(daily.Tracks) == 0 || daily.Tracks[0].Album.PicURL == "" || daily.Tracks[0].Duration == 0 {
		t.Errorf("daily tracks incomplete: %+v", daily.Tracks)
	}

	for _, pl := range home.Featured {
		if strings.HasPrefix(pl.ID, "spotify:album:") {
			al, err := PlaylistDetail(pl.ID)
			if err != nil {
				t.Fatal("album:", err)
			}
			t.Logf("album: %s by %s, %d tracks, first %+v", al.Name, al.Creator.Nickname, len(al.Tracks), al.Tracks[0])
			break
		}
	}

	charts, err := Charts()
	if err != nil {
		t.Fatal("charts:", err)
	}
	t.Logf("charts: %d; first %s %v; 5th %s", len(charts), charts[0].Name, charts[0].Preview, charts[min(4, len(charts)-1)].Name)
	if len(charts) < 5 || len(charts[0].Preview) != 3 {
		t.Error("charts incomplete")
	}

	big, err := PlaylistDetail("spotify:playlist:37i9dQZEVXbKXQ4mDTEBXq")
	if err != nil {
		t.Fatal("playlist:", err)
	}
	t.Logf("top 50: %d/%d tracks", len(big.Tracks), big.TrackCount)

	found, err := Search("陈奕迅", 60)
	if err != nil {
		t.Fatal("search:", err)
	}
	t.Logf("search: %d, first %s - %s", len(found), found[0].Name, found[0].ArtistNames())

	made, saved, err := UserPlaylists(me.UserID)
	if err != nil {
		t.Fatal("library:", err)
	}
	t.Logf("library: made %d saved %d", len(made), len(saved))

	liked, err := LikedIDs()
	if err != nil {
		t.Fatal("liked:", err)
	}
	t.Logf("liked: %d", len(liked))

	lrc, err := Lyrics(daily.Tracks[0].ID)
	if err != nil {
		t.Fatal("lyrics:", err)
	}
	t.Logf("lyrics of %s: %d bytes, %q", daily.Tracks[0].Name, len(lrc), lrc[:min(60, len(lrc))])

	m, err := ResolveMedia(daily.Tracks[0].ID)
	if err != nil {
		t.Fatal("media:", err)
	}
	t.Logf("media: %v, %d kbps, manifest %d bytes", m.Duration, m.Bitrate/1000, len(m.Manifest))
}
