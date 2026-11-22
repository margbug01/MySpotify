package spotify

import (
	"log"
	"slices"
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
}

// Credits are who made a track, as Spotify's credits list them.
type Credits struct {
	// Groups are in Spotify's order: artists, songwriting, production,
	// performers, named in the language the app asks in.
	Groups  []CreditGroup
	Sources []string // the labels it came from
}

// CreditGroup is a kind of work on a track, as 作曲和作词, and who did it.
type CreditGroup struct {
	Name   string
	People []Credit
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

// TrackDetail returns the page of track id: the track and songs like it.
func TrackDetail(id string) (*TrackPage, error) {
	uri := "spotify:track:" + id
	var (
		wg     sync.WaitGroup
		recs   []Track
		recErr error
	)
	wg.Go(func() { recs, recErr = recommended(uri) })
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
	// Without songs like it, the page still shows.
	if recErr != nil {
		log.Printf("track %s: %v", id, recErr)
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

// creditsPage is how many contributors one ask for credits returns.
// Without a limit Spotify gives 10, which cut most songs' producers,
// engineers and players off.
const creditsPage = 100

// TrackCredits returns who made track id, grouped as Spotify groups them,
// each person once in a group with all they did in it.
func TrackCredits(id string) (*Credits, error) {
	out := &Credits{}
	groups := map[string]int{} // a group's index in out.Groups
	people := map[string]int{} // a person's index in their group, by group, name and URI
	for offset := 0; offset < 10*creditsPage; offset += creditsPage {
		var res struct {
			TrackUnion struct {
				CreditsTrait struct {
					Contributors struct {
						Items []struct {
							Name      string `json:"name"`
							Role      string `json:"role"`
							URI       string `json:"uri"`
							RoleGroup struct {
								Name string `json:"name"`
							} `json:"roleGroup"`
						} `json:"items"`
					} `json:"contributors"`
					Sources struct {
						Items []struct {
							Name string `json:"name"`
						} `json:"items"`
					} `json:"sources"`
				} `json:"creditsTrait"`
			} `json:"trackUnion"`
		}
		vars := map[string]any{"trackUri": "spotify:track:" + id, "contributorsLimit": creditsPage, "contributorsOffset": offset}
		if err := query("queryTrackCreditsGroupedModal", vars, &res); err != nil {
			return nil, err
		}
		ct := &res.TrackUnion.CreditsTrait
		for _, c := range ct.Contributors.Items {
			gi, ok := groups[c.RoleGroup.Name]
			if !ok {
				gi = len(out.Groups)
				groups[c.RoleGroup.Name] = gi
				out.Groups = append(out.Groups, CreditGroup{Name: c.RoleGroup.Name})
			}
			g := &out.Groups[gi]
			key := c.RoleGroup.Name + "\n" + c.Name + "\n" + c.URI
			pi, ok := people[key]
			if !ok {
				pi = len(g.People)
				people[key] = pi
				g.People = append(g.People, Credit{Name: c.Name, Artist: c.URI})
			}
			if p := &g.People[pi]; c.Role != "" && !slices.Contains(p.Roles, c.Role) {
				p.Roles = append(p.Roles, c.Role)
			}
		}
		for _, s := range ct.Sources.Items {
			if s.Name != "" && !slices.Contains(out.Sources, s.Name) {
				out.Sources = append(out.Sources, s.Name)
			}
		}
		if len(ct.Contributors.Items) < creditsPage {
			break
		}
	}
	return out, nil
}
