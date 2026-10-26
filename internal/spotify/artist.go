package spotify

import (
	"fmt"
	"strconv"
)

// ArtistCard is an artist as lists show one: a name and a picture.
type ArtistCard struct {
	ID     string // its URI
	Name   string
	Avatar string
}

// ArtistPage is what an artist's page shows.
type ArtistPage struct {
	ArtistCard
	Banner           string // the wide picture over the page, if the artist has one
	Verified         bool
	Followers        int64
	MonthlyListeners int64
	Bio              string
	Followed         bool
	TopTracks        []Track
	// Popular are the releases most listened to; then come some of the
	// artist's releases of each kind, the latest first.
	Popular                       []Playlist
	Albums, Singles, Compilations Releases
	Related                       []ArtistCard
}

// Releases are some of an artist's releases of a kind, of Total.
type Releases struct {
	Kind  ReleaseKind
	Items []Playlist
	Total int
}

// ReleaseKind is a kind of release, as the artist's discography sorts them.
type ReleaseKind string

const (
	KindAlbums       ReleaseKind = "albums"
	KindSingles      ReleaseKind = "singles"
	KindCompilations ReleaseKind = "compilations"
)

type gqlRelease struct {
	URI      string   `json:"uri"`
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	CoverArt gqlImage `json:"coverArt"`
	Date     struct {
		Year int `json:"year"`
	} `json:"date"`
}

// playlist returns r, by artist, as a card shows it.
func (r gqlRelease) playlist(artist []Artist) Playlist {
	pl := Playlist{ID: r.URI, Name: r.Name, CoverURL: r.CoverArt.best(), Artists: artist}
	pl.Creator.Nickname = Track{Artists: artist}.ArtistNamesOf()
	kind := map[string]string{"ALBUM": "专辑", "SINGLE": "单曲", "EP": "EP", "COMPILATION": "合辑"}[r.Type]
	switch {
	case r.Date.Year > 0 && kind != "":
		pl.Note = strconv.Itoa(r.Date.Year) + " · " + kind
	case r.Date.Year > 0:
		pl.Note = strconv.Itoa(r.Date.Year)
	default:
		pl.Note = kind
	}
	return pl
}

// gqlReleaseGroups are a discography's releases of a kind: each item holds
// a release with its other editions, the first the one to show.
type gqlReleaseGroups struct {
	TotalCount int `json:"totalCount"`
	Items      []struct {
		Releases struct {
			Items []gqlRelease `json:"items"`
		} `json:"releases"`
	} `json:"items"`
}

func (g gqlReleaseGroups) releases(kind ReleaseKind, artist []Artist) Releases {
	rs := Releases{Kind: kind, Total: g.TotalCount}
	for _, it := range g.Items {
		if len(it.Releases.Items) > 0 && it.Releases.Items[0].URI != "" {
			rs.Items = append(rs.Items, it.Releases.Items[0].playlist(artist))
		}
	}
	return rs
}

type gqlArtistCard struct {
	URI     string `json:"uri"`
	Profile struct {
		Name string `json:"name"`
	} `json:"profile"`
	Visuals struct {
		AvatarImage gqlImage `json:"avatarImage"`
	} `json:"visuals"`
}

func (c gqlArtistCard) card() ArtistCard {
	return ArtistCard{ID: c.URI, Name: c.Profile.Name, Avatar: c.Visuals.AvatarImage.best()}
}

// ArtistDetail returns the page of the artist uri stands for.
func ArtistDetail(uri string) (*ArtistPage, error) {
	var res struct {
		ArtistUnion struct {
			Typename string `json:"__typename"`
			URI      string `json:"uri"`
			Saved    bool   `json:"saved"`
			Profile  struct {
				Name      string `json:"name"`
				Verified  bool   `json:"verified"`
				Biography struct {
					Text string `json:"text"`
				} `json:"biography"`
			} `json:"profile"`
			Stats struct {
				Followers        int64 `json:"followers"`
				MonthlyListeners int64 `json:"monthlyListeners"`
			} `json:"stats"`
			Visuals struct {
				AvatarImage gqlImage `json:"avatarImage"`
			} `json:"visuals"`
			HeaderImage *struct {
				Data gqlImage `json:"data"`
			} `json:"headerImage"`
			Discography struct {
				TopTracks struct {
					Items []struct {
						Track *gqlTrack `json:"track"`
					} `json:"items"`
				} `json:"topTracks"`
				PopularReleasesAlbums struct {
					Items []gqlRelease `json:"items"`
				} `json:"popularReleasesAlbums"`
				Albums       gqlReleaseGroups `json:"albums"`
				Singles      gqlReleaseGroups `json:"singles"`
				Compilations gqlReleaseGroups `json:"compilations"`
			} `json:"discography"`
			RelatedContent struct {
				RelatedArtists struct {
					Items []gqlArtistCard `json:"items"`
				} `json:"relatedArtists"`
			} `json:"relatedContent"`
		} `json:"artistUnion"`
	}
	if err := query("queryArtistOverview", map[string]any{"uri": uri, "locale": ""}, &res); err != nil {
		return nil, err
	}
	u := &res.ArtistUnion
	if u.Typename != "Artist" || u.URI == "" {
		return nil, ErrNotFound
	}
	me := []Artist{{ID: idOf(u.URI), Name: u.Profile.Name}}
	pg := &ArtistPage{
		ArtistCard:       ArtistCard{ID: u.URI, Name: u.Profile.Name, Avatar: u.Visuals.AvatarImage.best()},
		Verified:         u.Profile.Verified,
		Followers:        u.Stats.Followers,
		MonthlyListeners: u.Stats.MonthlyListeners,
		Bio:              stripTags(u.Profile.Biography.Text),
		Followed:         u.Saved,
		Albums:           u.Discography.Albums.releases(KindAlbums, me),
		Singles:          u.Discography.Singles.releases(KindSingles, me),
		Compilations:     u.Discography.Compilations.releases(KindCompilations, me),
	}
	if u.HeaderImage != nil {
		pg.Banner = u.HeaderImage.Data.best()
	}
	// The top tracks' albums come without names: those listed here have
	// theirs.
	names := map[string]string{}
	for _, r := range u.Discography.PopularReleasesAlbums.Items {
		pg.Popular = append(pg.Popular, r.playlist(me))
		names[r.URI] = r.Name
	}
	for _, rs := range []Releases{pg.Albums, pg.Singles, pg.Compilations} {
		for _, pl := range rs.Items {
			names[pl.ID] = pl.Name
		}
	}
	items := make([]*gqlTrack, len(u.Discography.TopTracks.Items))
	for i, it := range u.Discography.TopTracks.Items {
		items[i] = it.Track
	}
	pg.TopTracks = tracks(items, nil)
	for i := range pg.TopTracks {
		if al := &pg.TopTracks[i].Album; al.Name == "" {
			al.Name = names[al.ID]
		}
	}
	for _, c := range u.RelatedContent.RelatedArtists.Items {
		if c.URI != "" {
			pg.Related = append(pg.Related, c.card())
		}
	}
	return pg, nil
}

// ArtistReleases returns all the releases of kind by the artist uri
// stands for, named name, the latest first.
func ArtistReleases(uri, name string, kind ReleaseKind) (Releases, error) {
	op := map[ReleaseKind]string{
		KindAlbums:       "queryArtistDiscographyAlbums",
		KindSingles:      "queryArtistDiscographySingles",
		KindCompilations: "queryArtistDiscographyCompilations",
	}[kind]
	if op == "" {
		return Releases{}, fmt.Errorf("spotify: no releases of kind %q", kind)
	}
	me := []Artist{{ID: idOf(uri), Name: name}}
	all := Releases{Kind: kind}
	for offset := 0; ; {
		var res struct {
			ArtistUnion struct {
				Discography map[string]gqlReleaseGroups `json:"discography"`
			} `json:"artistUnion"`
		}
		if err := query(op, map[string]any{"uri": uri, "offset": offset, "limit": 50, "order": "DATE_DESC"}, &res); err != nil {
			return all, err
		}
		g := res.ArtistUnion.Discography[string(kind)]
		all.Items, all.Total = append(all.Items, g.releases(kind, me).Items...), g.TotalCount
		offset += len(g.Items)
		if len(g.Items) == 0 || offset >= g.TotalCount {
			return all, nil
		}
	}
}

// SearchArtists returns the artists matching q, the best first.
func SearchArtists(q string, limit int) ([]ArtistCard, error) {
	var res struct {
		SearchV2 struct {
			Artists struct {
				Items []struct {
					Data gqlArtistCard `json:"data"`
				} `json:"items"`
			} `json:"artists"`
		} `json:"searchV2"`
	}
	err := query("searchArtists", map[string]any{
		"searchTerm": q, "offset": 0, "limit": limit, "numberOfTopResults": 5,
		"includeAudiobooks": false, "includePreReleases": false, "includeAuthors": false,
	}, &res)
	if err != nil {
		return nil, err
	}
	var out []ArtistCard
	for _, it := range res.SearchV2.Artists.Items {
		if it.Data.URI != "" {
			out = append(out, it.Data.card())
		}
	}
	return out, nil
}
