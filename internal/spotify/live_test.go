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
	if len(home.Featured) == 0 {
		t.Error("home lacks featured lists")
	}
	// A new account has no daily mix, until it has listened a while: a
	// featured list stands in.
	first := home.Daily
	if first == "" {
		for _, pl := range home.Featured {
			if strings.HasPrefix(pl.ID, "spotify:playlist:") {
				first = pl.ID
				break
			}
		}
	}

	daily, err := PlaylistDetail(first)
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

	lib, err := UserLibrary(me.UserID)
	if err != nil {
		t.Fatal("library:", err)
	}
	t.Logf("library: made %d saved %d artists %d %v", len(lib.Made), len(lib.Saved), len(lib.Artists), lib.Artists[:min(2, len(lib.Artists))])
	t.Logf("home artists: %d %v", len(home.Artists), home.Artists[:min(2, len(home.Artists))])
	recent, recentArtists, err := RecentlyPlayed(me.UserID)
	if err != nil {
		t.Fatal("recently played:", err)
	}
	t.Logf("recently played: %d lists %d artists", len(recent), len(recentArtists))

	for _, uri := range []string{"spotify:artist:2QcZxAgcs2I1q7CtCkl6MI", "spotify:artist:2IUl3m1H1EQ7QfNbNWvgru"} {
		ar, err := ArtistDetail(uri)
		if err != nil {
			t.Fatal("artist:", err)
		}
		t.Logf("artist %s: verified %v followers %d monthly %d followed %v banner %v avatar %v bio %d", ar.Name, ar.Verified, ar.Followers, ar.MonthlyListeners, ar.Followed, ar.Banner != "", ar.Avatar != "", len(ar.Bio))
		t.Logf("  top %d first %s (%s) | popular %d %s %q | albums %d/%d singles %d/%d comps %d/%d | related %d %v",
			len(ar.TopTracks), ar.TopTracks[0].Name, ar.TopTracks[0].Album.Name, len(ar.Popular), ar.Popular[0].Name, ar.Popular[0].Note,
			len(ar.Albums.Items), ar.Albums.Total, len(ar.Singles.Items), ar.Singles.Total, len(ar.Compilations.Items), ar.Compilations.Total, len(ar.Related), ar.Related[0])
		if len(ar.TopTracks) == 0 || ar.TopTracks[0].Duration == 0 || len(ar.Popular) == 0 || ar.Avatar == "" {
			t.Errorf("artist %s incomplete", uri)
		}
	}
	all, err := ArtistReleases("spotify:artist:2QcZxAgcs2I1q7CtCkl6MI", "陳奕迅", KindAlbums)
	if err != nil || len(all.Items) != all.Total {
		t.Fatalf("all albums: %d of %d, %v", len(all.Items), all.Total, err)
	}
	t.Logf("all albums: %d, last %s %q", len(all.Items), all.Items[len(all.Items)-1].Name, all.Items[len(all.Items)-1].Note)
	singer, err := SearchArtists("陈奕迅", 8)
	if err != nil || len(singer) == 0 {
		t.Fatalf("search artists: %v %v", singer, err)
	}
	t.Logf("search artists: %d %v", len(singer), singer[0])

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

	for _, seed := range []string{daily.Tracks[0].URI(), "spotify:artist:2QcZxAgcs2I1q7CtCkl6MI"} {
		uri, err := Radio(seed)
		if err != nil {
			t.Fatal("radio:", err)
		}
		radio, err := PlaylistDetail(uri)
		if err != nil || len(radio.Tracks) == 0 {
			t.Fatalf("radio %s: %v", uri, err)
		}
		t.Logf("radio of %s: %s %q, %d tracks, first %s", seed, uri, radio.Name, len(radio.Tracks), radio.Tracks[0].Name)
	}

	tp, err := TrackDetail("3cthRwQ2X0SNTaauD7tjn5")
	if err != nil {
		t.Fatal("track:", err)
	}
	t.Logf("track %s by %v: %d plays, %d, %d like it, %d credits (%+v), %d popular of %s", tp.Track.Name, tp.Track.Artists, tp.Plays, tp.Year,
		len(tp.Recommended), len(tp.Credits), tp.Credits[:min(2, len(tp.Credits))], len(tp.Popular), tp.Artist.Name)
	if tp.Plays == 0 || len(tp.Recommended) == 0 || len(tp.Credits) == 0 || tp.Track.Album.PicURL == "" {
		t.Error("track page incomplete")
	}

	m, err := ResolveMedia(daily.Tracks[0].ID)
	if err != nil {
		t.Fatal("media:", err)
	}
	t.Logf("media: %v, %d kbps, manifest %d bytes", m.Duration, m.Bitrate/1000, len(m.Manifest))
}
