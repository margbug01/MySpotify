package app

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"slices"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"vinyl/internal/spotify"
)

// Recently played: the songs Vinyl played, which it keeps itself, as
// Spotify hears nothing of them, and what the listener played in Spotify's
// own apps.

// maxPlayed is how many songs Vinyl keeps of those it played.
const maxPlayed = 100

// recentlyPlayed loads what the listener played in Spotify's apps; tests
// replace it.
var recentlyPlayed = spotify.RecentlyPlayed

func playedPath() (string, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "played.json"), nil
}

// loadPlayed reads the songs Vinyl played, as last kept.
func loadPlayed() []spotify.Track {
	path, err := playedPath()
	if err != nil {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var played []spotify.Track
	json.Unmarshal(b, &played)
	return played
}

// notePlayed puts the song playing first among those played, once it
// plays: as the player says something changed.
func (a *app) notePlayed() {
	tr := a.pl.Current()
	if tr == nil || !a.pl.Playing() || !a.pl.Ready() || len(a.played) > 0 && a.played[0].ID == tr.ID {
		return
	}
	t := *tr
	t.UID = "" // its place in a playlist, not here
	played := slices.DeleteFunc(slices.Clone(a.played), func(p spotify.Track) bool { return p.ID == t.ID })
	a.played = append([]spotify.Track{t}, played[:min(len(played), maxPlayed-1)]...)
	if a.settings.path == "" {
		// The scenes and tests keep the listener's as it was.
		return
	}
	b, _ := json.Marshal(a.played)
	go func() {
		path, err := playedPath()
		if err == nil {
			err = os.WriteFile(path, b, 0o600)
		}
		if err != nil {
			log.Printf("played: %v", err)
		}
	}()
}

// loadRecent loads what the listener played in Spotify's apps.
func (a *app) loadRecent() {
	if a.profile == nil || a.recentLoading {
		return
	}
	a.recentLoading = true
	user := a.profile.UserID
	go func() {
		lists, artists, err := recentlyPlayed(user)
		a.post(func() {
			a.recentLoading = false
			if err != nil {
				log.Printf("recently played: %v", err)
				return
			}
			a.recentLists, a.recentArtists = lists, artists
		})
	}()
}

// recentPage lists the songs Vinyl played, the latest first, under what
// the listener played in Spotify's apps.
func (a *app) recentPage(c *ui.Context) {
	elsewhere := len(a.recentLists) > 0 || len(a.recentArtists) > 0
	title := func(c *ui.Context, s string) {
		ui.Text(c, s).FontSize(19).FontWeight(700).LetterSpacing(-0.3).Margin(4, 0, 14, 0)
	}
	header := func(c *ui.Context) {
		a.heading(c, "最近播放", "")
		if !elsewhere {
			return
		}
		ui.Column(c).PaddingX(12).Margin(0, 0, 8, 0).Children(func() {
			title(c, "在 Spotify 其他应用里")
			if lists := a.recentLists; len(lists) > 0 {
				cols := a.columns(c, 150)
				ui.Grid(c).Columns(cols).GapX(20).GapY(28).Children(func() {
					for i := range lists[:min(len(lists), cols)] {
						a.playlistCard(c.Key(lists[i].ID), &lists[i])
					}
				})
			}
			if len(a.recentArtists) > 0 {
				ui.Box(c).Height(24)
				a.artistRow(c, a.recentArtists)
			}
			ui.Box(c).Height(16)
			title(c, "在 Vinyl 里")
		})
	}
	if len(a.played) == 0 {
		top, bottom := a.insets(c)
		ui.Column(c).Grow(1).Padding(top+pageGap, pagePad, bottom, pagePad).Children(func() {
			header(c)
			a.centered(c, "在 Vinyl 里听过的歌会出现在这里", nil)
		})
		return
	}
	a.trackRows(c, a.played, true, header, nil)
}
