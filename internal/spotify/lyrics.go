package spotify

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Lyrics returns track id's lyrics as LRC, or "" if it has no timed
// lyrics.
func Lyrics(id string) (string, error) {
	raw, err := spGet("/color-lyrics/v2/track/"+id+"?format=json&vocalRemoval=false&market=from_token", "application/json")
	if errors.Is(err, ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return lyricsLRC(raw)
}

// lyricsLRC turns color-lyrics' JSON into LRC.
func lyricsLRC(raw []byte) (string, error) {
	var res struct {
		Lyrics struct {
			SyncType string `json:"syncType"`
			Lines    []struct {
				StartTimeMs string `json:"startTimeMs"`
				Words       string `json:"words"`
			} `json:"lines"`
		} `json:"lyrics"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return "", fmt.Errorf("lyrics: %w", err)
	}
	if res.Lyrics.SyncType != "LINE_SYNCED" && res.Lyrics.SyncType != "SYLLABLE_SYNCED" {
		return "", nil
	}
	var b strings.Builder
	for _, l := range res.Lyrics.Lines {
		ms, err := strconv.ParseInt(l.StartTimeMs, 10, 64)
		if err != nil {
			continue
		}
		// A lone note marks music without words.
		words := strings.TrimSpace(l.Words)
		if words == "♪" {
			words = ""
		}
		fmt.Fprintf(&b, "[%02d:%02d.%02d]%s\n", ms/60000, ms/1000%60, ms%1000/10, words)
	}
	return b.String(), nil
}
