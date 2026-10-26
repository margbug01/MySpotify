package spotify

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// The listener's own playlists, edited as the web player does: songs go in
// and out through GraphQL; making, renaming and deleting playlists go to
// the playlist service, as changes to the playlist or to the listener's
// list of playlists (the rootlist).

const playlistURL = spclientURL + "/playlist/v2"

// AddToPlaylist adds the tracks ids to the end of the playlist uri.
func AddToPlaylist(uri string, ids ...string) error {
	uris := make([]string, len(ids))
	for i, id := range ids {
		uris[i] = "spotify:track:" + id
	}
	var res json.RawMessage
	return query("addToPlaylist", map[string]any{
		"playlistUri": uri, "playlistItemUris": uris, "interactionId": nil,
		"newPosition": map[string]any{"moveType": "BOTTOM_OF_PLAYLIST", "fromUid": nil},
	}, &res)
}

// RemoveFromPlaylist takes the items uids, each a track's place in it, out
// of the playlist uri.
func RemoveFromPlaylist(uri string, uids ...string) error {
	var res json.RawMessage
	return query("removeFromPlaylist", map[string]any{"playlistUri": uri, "uids": uids}, &res)
}

// CreatePlaylist makes a playlist named name, first among the listener
// user's, and returns it, empty.
func CreatePlaylist(user, name string) (*Playlist, error) {
	raw, err := spPost(playlistURL+"/playlist", map[string]any{"ops": []any{attributes(name)}})
	if err != nil {
		return nil, err
	}
	var made struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(raw, &made); err != nil || made.URI == "" {
		return nil, errors.New("create playlist: no URI")
	}
	add := map[string]any{"kind": "ADD", "add": map[string]any{"addFirst": true, "items": []any{map[string]any{
		"uri": made.URI, "attributes": map[string]any{
			"timestamp": strconv.FormatInt(time.Now().UnixMilli(), 10),
		},
	}}}}
	if _, err := spPost(rootlist(user), changes(add)); err != nil {
		return nil, err
	}
	return &Playlist{ID: made.URI, Name: name}, nil
}

// RenamePlaylist names the playlist uri name.
func RenamePlaylist(uri, name string) error {
	_, err := spPost(playlistURL+"/playlist/"+idOf(uri)+"/changes", changes(attributes(name)))
	return err
}

// DeletePlaylist takes the playlist uri out of the listener user's: as
// Spotify deletes one, which its followers keep.
func DeletePlaylist(user, uri string) error {
	rem := map[string]any{"kind": "REM", "rem": map[string]any{"itemsAsKey": true, "items": []any{map[string]any{"uri": uri}}}}
	_, err := spPost(rootlist(user), changes(rem))
	return err
}

func rootlist(user string) string {
	return playlistURL + "/user/" + url.PathEscape(user) + "/rootlist/changes"
}

// attributes is the operation that names a playlist name.
func attributes(name string) map[string]any {
	return map[string]any{"kind": "UPDATE_LIST_ATTRIBUTES", "updateListAttributes": map[string]any{
		"newAttributes": map[string]any{"values": map[string]any{"name": name}},
	}}
}

// delta is a change of the operations ops, from the web player.
func delta(ops ...any) map[string]any {
	return map[string]any{"ops": ops, "info": map[string]any{"source": map[string]any{"client": "WEBPLAYER"}}}
}

// changes are a delta of ops, to apply to a list as it is now.
func changes(ops ...any) map[string]any {
	return map[string]any{"deltas": []any{delta(ops...)}}
}

// spPost posts body, as JSON, to url, as the web player does.
func spPost(url string, body any) ([]byte, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return do(func() (*http.Request, error) {
		req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
		if err == nil {
			req.Header.Set("Content-Type", "application/json;charset=UTF-8")
			req.Header.Set("Accept", "application/json")
		}
		return req, err
	})
}

// Radio returns the playlist of Spotify's radio of seed, a track's or an
// artist's URI: songs like it, fifty of them.
func Radio(seed string) (string, error) {
	raw, err := spGet("/inspiredby-mix/v2/seed_to_playlist/"+seed+"?response-format=json", "application/json")
	if err != nil {
		return "", err
	}
	var v struct {
		MediaItems []struct {
			URI string `json:"uri"`
		} `json:"mediaItems"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", err
	}
	if len(v.MediaItems) == 0 || v.MediaItems[0].URI == "" {
		return "", ErrNotFound
	}
	return v.MediaItems[0].URI, nil
}
