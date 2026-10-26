package netease

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
)

// Editing the listener's own playlists, through the eapi, which carries
// the session.

// ErrInPlaylist is what adding a song a playlist has already gives.
var ErrInPlaylist = errors.New("the song is in the playlist already")

// CreatePlaylist makes a playlist named name for the listener, and returns
// it, empty.
func CreatePlaylist(name string) (*Playlist, error) {
	var res struct {
		Code     int      `json:"code"`
		Message  string   `json:"message"`
		ID       int64    `json:"id"`
		Playlist Playlist `json:"playlist"`
	}
	data := map[string]any{"name": name, "privacy": "0", "type": "NORMAL"}
	if err := eapiPost("/api/playlist/create", data, &res); err != nil {
		return nil, err
	}
	if res.Code != 200 {
		return nil, fmt.Errorf("netease: create playlist: code %d %s", res.Code, res.Message)
	}
	pl := res.Playlist
	if pl.ID == 0 {
		pl.ID = res.ID
	}
	if pl.Name == "" {
		pl.Name = name
	}
	return &pl, nil
}

// RenamePlaylist names the listener's playlist id name.
func RenamePlaylist(id int64, name string) error {
	var res struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	data := map[string]any{"id": strconv.FormatInt(id, 10), "name": name}
	if err := eapiPost("/api/playlist/update/name", data, &res); err != nil {
		return err
	}
	if res.Code != 200 {
		return fmt.Errorf("netease: rename playlist: code %d %s", res.Code, res.Message)
	}
	return nil
}

// AddToPlaylist adds songs to the listener's playlist pid, at its top.
func AddToPlaylist(pid int64, songs ...int64) error { return manipulate("add", pid, songs) }

// RemoveFromPlaylist takes songs out of the listener's playlist pid.
func RemoveFromPlaylist(pid int64, songs ...int64) error { return manipulate("del", pid, songs) }

func manipulate(op string, pid int64, songs []int64) error {
	var res struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	send := func(ids []int64) error {
		b, _ := json.Marshal(ids)
		data := map[string]any{"op": op, "pid": strconv.FormatInt(pid, 10), "trackIds": string(b), "imme": "true"}
		return eapiPost("/api/playlist/manipulate/tracks", data, &res)
	}
	if err := send(songs); err != nil {
		return err
	}
	// Some playlists refuse a list of songs once, as the NeteaseCloudMusicApi
	// project found, and take it twice over.
	if res.Code == 512 {
		if err := send(slices.Concat(songs, songs)); err != nil {
			return err
		}
	}
	switch res.Code {
	case 200:
		return nil
	case 502:
		return ErrInPlaylist
	}
	return fmt.Errorf("netease: %s playlist songs: code %d %s", op, res.Code, res.Message)
}

// DeletePlaylist deletes the listener's playlist id, for good.
func DeletePlaylist(id int64) error {
	var res struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := eapiPost("/api/playlist/remove", map[string]any{"ids": fmt.Sprintf("[%d]", id)}, &res); err != nil {
		return err
	}
	if res.Code != 200 {
		return fmt.Errorf("netease: delete playlist: code %d %s", res.Code, res.Message)
	}
	return nil
}
