package spotify

import (
	"encoding/json"
	"errors"
	"net/url"
)

// RecentlyPlayed returns what the listener user played lately in Spotify's
// own apps, the latest first: playlists and albums, and artists. Spotify
// hears nothing of what Vinyl plays.
func RecentlyPlayed(user string) ([]Playlist, []ArtistCard, error) {
	raw, err := spGet("/recently-played/v3/user/"+url.PathEscape(user)+
		"/recently-played?format=json&offset=0&limit=50&filter=default,collection-new-episodes", "application/json")
	if errors.Is(err, ErrNotFound) {
		// Nothing played yet.
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var played struct {
		PlayContexts []struct {
			URI string `json:"uri"`
		} `json:"playContexts"`
	}
	if err := json.Unmarshal(raw, &played); err != nil {
		return nil, nil, err
	}
	uris := make([]string, 0, len(played.PlayContexts))
	for _, c := range played.PlayContexts {
		uris = append(uris, c.URI)
	}
	if len(uris) == 0 {
		return nil, nil, nil
	}
	var res struct {
		Lookup []struct {
			Data gqlEntity `json:"data"`
		} `json:"lookup"`
	}
	if err := query("fetchEntitiesForRecentlyPlayed", map[string]any{"uris": uris}, &res); err != nil {
		return nil, nil, err
	}
	var lists []Playlist
	var artists []ArtistCard
	for _, l := range res.Lookup {
		if ar, ok := l.Data.artist(); ok {
			artists = append(artists, ar)
		} else if pl, ok := l.Data.playlist(); ok {
			lists = append(lists, pl)
		}
	}
	return lists, artists, nil
}
