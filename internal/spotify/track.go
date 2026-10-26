package spotify

import (
	"log"
	"strconv"
	"sync"
)

// TrackPage is what a track's page shows.
type TrackPage struct {
	Track Track
	Plays int64 // how many times it was played, as Spotify counts
	Year  int   // when its album came out
	// Artist is its first artist, whose popular releases follow.
	Artist      ArtistCard
	Popular     []Playlist
	Recommended []Track // songs like it
	Credits     []Credit
}

// Credit is someone who made a track, with what they did.
type Credit struct {
	Name   string
	Artist string // the URI of their artist page, or ""
	Roles  []string
}

type gqlTrackArtist struct {
	URI     string `json:"uri"`
	Profile struct {
		Name string `json:"name"`
	} `json:"profile"`
	Visuals struct {
		AvatarImage gqlImage `json:"avatarImage"`
	} `json:"visuals"`
	Discography struct {
		PopularReleasesAlbums struct {
			Items []gqlRelease `json:"items"`
		} `json:"popularReleasesAlbums"`
	} `json:"discography"`
}

// TrackDetail returns the page of track id: the track, songs like it, and
// who made it.
func TrackDetail(id string) (*TrackPage, error) {
	uri := "spotify:track:" + id
	var (
		wg        sync.WaitGroup
		recs      []Track
		credits   []Credit
		recErr    error
		creditErr error
	)
	wg.Go(func() { recs, recErr = recommended(uri) })
	wg.Go(func() { credits, creditErr = trackCredits(uri) })
	var res struct {
		TrackUnion struct {
			URI         string `json:"uri"`
			Name        string `json:"name"`
			Duration    millis `json:"duration"`
			Playcount   string `json:"playcount"`
			Playability struct {
				Playable bool `json:"playable"`
			} `json:"playability"`
			AlbumOfTrack struct {
				URI      string   `json:"uri"`
				Name     string   `json:"name"`
				CoverArt gqlImage `json:"coverArt"`
				Date     struct {
					Year int `json:"year"`
				} `json:"date"`
			} `json:"albumOfTrack"`
			FirstArtist struct {
				Items []gqlTrackArtist `json:"items"`
			} `json:"firstArtist"`
			OtherArtists struct {
				Items []gqlTrackArtist `json:"items"`
			} `json:"otherArtists"`
		} `json:"trackUnion"`
	}
	err := query("getTrack", map[string]any{"uri": uri}, &res)
	wg.Wait()
	if err != nil {
		return nil, err
	}
	t := &res.TrackUnion
	if t.URI == "" {
		return nil, ErrNotFound
	}
	al := t.AlbumOfTrack
	pg := &TrackPage{
		Track: Track{
			ID: id, Name: t.Name, Duration: t.Duration.TotalMilliseconds, Unplayable: !t.Playability.Playable,
			Album: Album{ID: al.URI, Name: al.Name, PicURL: al.CoverArt.best()},
		},
		Year:        al.Date.Year,
		Recommended: recs,
		Credits:     credits,
	}
	pg.Plays, _ = strconv.ParseInt(t.Playcount, 10, 64)
	for _, ar := range append(t.FirstArtist.Items, t.OtherArtists.Items...) {
		pg.Track.Artists = append(pg.Track.Artists, Artist{ID: idOf(ar.URI), Name: ar.Profile.Name})
	}
	if len(t.FirstArtist.Items) > 0 {
		ar := t.FirstArtist.Items[0]
		pg.Artist = ArtistCard{ID: ar.URI, Name: ar.Profile.Name, Avatar: ar.Visuals.AvatarImage.best()}
		by := []Artist{{ID: idOf(ar.URI), Name: ar.Profile.Name}}
		for _, r := range ar.Discography.PopularReleasesAlbums.Items {
			pg.Popular = append(pg.Popular, r.playlist(by))
		}
	}
	// Without songs like it or its credits, the page still shows.
	for _, err := range []error{recErr, creditErr} {
		if err != nil {
			log.Printf("track %s: %v", id, err)
		}
	}
	return pg, nil
}

// recommended returns songs like the track uri, as its page on the web
// player lists them.
func recommended(uri string) ([]Track, error) {
	var res struct {
		SeoRecommendedTrack struct {
			Items []struct {
				Data *gqlTrack `json:"data"`
			} `json:"items"`
		} `json:"seoRecommendedTrack"`
	}
	if err := query("internalLinkRecommenderTrack", map[string]any{"uri": uri, "limit": 20}, &res); err != nil {
		return nil, err
	}
	items := make([]*gqlTrack, len(res.SeoRecommendedTrack.Items))
	for i, it := range res.SeoRecommendedTrack.Items {
		items[i] = it.Data
	}
	return tracks(items, nil), nil
}

// trackCredits returns who made the track uri, each once, with all they
// did, in Spotify's order.
func trackCredits(uri string) ([]Credit, error) {
	var res struct {
		TrackUnion struct {
			CreditsTrait struct {
				Contributors struct {
					Items []struct {
						Name string `json:"name"`
						Role string `json:"role"`
						URI  string `json:"uri"`
					} `json:"items"`
				} `json:"contributors"`
			} `json:"creditsTrait"`
		} `json:"trackUnion"`
	}
	if err := query("queryTrackCreditsGroupedModal", map[string]any{"trackUri": uri}, &res); err != nil {
		return nil, err
	}
	var out []Credit
	at := map[string]int{}
	for _, c := range res.TrackUnion.CreditsTrait.Contributors.Items {
		key := c.Name + "\n" + c.URI
		i, ok := at[key]
		if !ok {
			i = len(out)
			at[key] = i
			out = append(out, Credit{Name: c.Name, Artist: c.URI})
		}
		if c.Role != "" {
			out[i].Roles = append(out[i].Roles, c.Role)
		}
	}
	return out, nil
}
