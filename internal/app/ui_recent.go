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

// maxPlayedFrom is how many lists, artists and podcasts Vinyl keeps of
// those it played from.
const maxPlayedFrom = 20

// played.json keeps the songs Vinyl played, played-from.json what it
// played them from.
func playedPath(name string) (string, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

// loadPlayed reads the songs Vinyl played, and what it played them from,
// as last kept.
func loadPlayed() (played []spotify.Track, from []spotify.HomeItem) {
	for name, v := range map[string]any{"played.json": &played, "played-from.json": &from} {
		if path, err := playedPath(name); err == nil {
			if b, err := os.ReadFile(path); err == nil {
				json.Unmarshal(b, v)
			}
		}
	}
	return played, from
}

// keepPlayed writes v down as name, unless in the scenes and tests, which
// keep the listener's as it was.
func (a *app) keepPlayed(name string, v any) {
	if a.settings.path == "" {
		return
	}
	b, _ := json.Marshal(v)
	go func() {
		path, err := playedPath(name)
		if err == nil {
			err = os.WriteFile(path, b, 0o600)
		}
		if err != nil {
			log.Printf("%s: %v", name, err)
		}
	}()
}

// notePlayed puts the song playing first among those played, once it
// plays, and what the queue was played from first among those: as the
// player says something changed.
func (a *app) notePlayed() {
	tr := a.pl.Current()
	if tr == nil || !a.pl.Playing() || !a.pl.Ready() {
		return
	}
	a.notePlayedFrom()
	if len(a.played) > 0 && a.played[0].ID == tr.ID {
		return
	}
	t := *tr
	t.UID = "" // its place in a playlist, not here
	played := slices.DeleteFunc(slices.Clone(a.played), func(p spotify.Track) bool { return p.ID == t.ID })
	a.played = append([]spotify.Track{t}, played[:min(len(played), maxPlayed-1)]...)
	a.keepPlayed("played.json", a.played)
}

// queued is what the queue was played from, until a song of it plays:
// the queue's first song and its length tell it again.
type queued struct {
	from  spotify.HomeItem
	first string
	n     int
}

// playFrom plays tracks from start, as from, a list, an artist or a
// podcast: once a song of them plays, from goes first among what was
// played lately, which Spotify hears nothing of.
func (a *app) playFrom(from spotify.HomeItem, tracks []spotify.Track, start int) {
	a.pl.PlayQueue(tracks, start)
	a.queuedFrom = nil
	if from.ID() != "" && len(tracks) > 0 {
		if from.List != nil {
			// Its songs are the queue's, not to keep twice.
			pl := *from.List
			pl.Tracks, pl.Preview = nil, nil
			from.List = &pl
		}
		a.queuedFrom = &queued{from, tracks[0].ID, len(tracks)}
	}
}

// notePlayedFrom puts what the queue playing was played from first among
// those: once, and only if the queue is still that one.
func (a *app) notePlayedFrom() {
	q := a.queuedFrom
	if q == nil {
		return
	}
	a.queuedFrom = nil
	if queue := a.pl.Queue(); len(queue) != q.n || queue[0].ID != q.first {
		return
	}
	id := q.from.ID()
	rest := slices.DeleteFunc(slices.Clone(a.playedFrom), func(it spotify.HomeItem) bool { return it.ID() == id })
	a.playedFrom = append([]spotify.HomeItem{q.from}, rest[:min(len(rest), maxPlayedFrom-1)]...)
	a.keepPlayed("played-from.json", a.playedFrom)
}

// routeItem is what the page shows the songs of, to play them from: a
// list or an artist, or nothing.
func (a *app) routeItem() spotify.HomeItem {
	switch a.route.page {
	case pagePlaylist:
		if pl := a.playlists[a.route.playlist]; pl != nil {
			return spotify.HomeItem{List: pl}
		}
	case pageArtist:
		if pg := a.artists[a.route.artist]; pg != nil {
			return spotify.HomeItem{Artist: &pg.ArtistCard}
		}
	}
	return spotify.HomeItem{}
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
