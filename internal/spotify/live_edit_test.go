package spotify

import (
	"os"
	"slices"
	"testing"
)

// With LIVE and LIVE_EDIT set, edits the account LIVE names, and undoes
// it: makes a playlist, adds two songs, takes one out, renames it, deletes
// it; likes a song and follows an artist, then not. Each step is checked by
// reading back what Spotify has.
func TestLiveEdit(t *testing.T) {
	path := os.Getenv("LIVE")
	if path == "" || os.Getenv("LIVE_EDIT") == "" {
		t.Skip("LIVE or LIVE_EDIT not set")
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
	listed := func(uri string) bool {
		lib, err := UserLibrary(me.UserID)
		if err != nil {
			t.Fatal("library:", err)
		}
		return slices.ContainsFunc(lib.Made, func(p Playlist) bool { return p.ID == uri })
	}

	pl, err := CreatePlaylist(me.UserID, "Vinyl 测试")
	if err != nil {
		t.Fatal("create:", err)
	}
	t.Logf("made %s", pl.ID)
	defer func() {
		if err := DeletePlaylist(me.UserID, pl.ID); err != nil {
			t.Error("delete:", err)
		}
		if listed(pl.ID) {
			t.Error("still listed after deleting")
		}
	}()
	if !listed(pl.ID) {
		t.Fatal("not listed after making it")
	}

	const one, two = "0DekZOl0nJdJ7bMs0hhVas", "3n3Ppam7vgaVa1iaRUc9Lp"
	if err := AddToPlaylist(pl.ID, one, two); err != nil {
		t.Fatal("add:", err)
	}
	got, err := PlaylistDetail(pl.ID)
	if err != nil || len(got.Tracks) != 2 || got.Tracks[0].UID == "" {
		t.Fatalf("after adding: %v, %+v", err, got)
	}
	if err := RemoveFromPlaylist(pl.ID, got.Tracks[0].UID); err != nil {
		t.Fatal("remove:", err)
	}
	got, err = PlaylistDetail(pl.ID)
	if err != nil || len(got.Tracks) != 1 || got.Tracks[0].ID != two {
		t.Fatalf("after removing the first: %v, %+v", err, got)
	}
	if err := RenamePlaylist(pl.ID, "Vinyl 测试 改名"); err != nil {
		t.Fatal("rename:", err)
	}
	if got, err = PlaylistDetail(pl.ID); err != nil || got.Name != "Vinyl 测试 改名" {
		t.Fatalf("after renaming: %v, %q", err, got.Name)
	}

	likes := func() []string {
		ids, err := LikedIDs()
		if err != nil {
			t.Fatal("liked:", err)
		}
		return ids
	}
	before := len(likes())
	if err := Like(one, true); err != nil {
		t.Fatal("like:", err)
	}
	if ids := likes(); !slices.Contains(ids, one) || len(ids) != before+1 {
		t.Errorf("after liking: %d, had %d", len(ids), before)
	}
	if err := Like(one, false); err != nil {
		t.Fatal("unlike:", err)
	}
	if ids := likes(); slices.Contains(ids, one) || len(ids) != before {
		t.Errorf("after unliking: %d, had %d", len(ids), before)
	}

	const artist = "spotify:artist:2QcZxAgcs2I1q7CtCkl6MI"
	followed := func() bool {
		ar, err := ArtistDetail(artist)
		if err != nil {
			t.Fatal("artist:", err)
		}
		return ar.Followed
	}
	was := followed()
	if err := Save(artist, !was); err != nil {
		t.Fatal("follow:", err)
	}
	if followed() == was {
		t.Error("following did not change")
	}
	if err := Save(artist, was); err != nil {
		t.Fatal("unfollow:", err)
	}
	if followed() != was {
		t.Error("following not back as it was")
	}
}
