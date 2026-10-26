package spotify

import (
	"encoding/json"
	"net/url"
	"strings"
)

// Category is a tile of the browse pages: a genre, a mood, podcasts. Its
// page lists playlists, albums and podcasts, and maybe tiles of its own.
type Category struct {
	ID    string // its page's URI
	Name  string
	Color string // its tile's color, "#rrggbb"
	Image string
}

// Shelf is a row of a category's page: lists, or tiles.
type Shelf struct {
	Title string
	Lists []Playlist
	Tiles []Category
}

// CategoryPage is what a category's page shows.
type CategoryPage struct {
	Name    string
	Color   string
	Shelves []Shelf
}

type gqlCard struct {
	Artwork         gqlImage `json:"artwork"`
	BackgroundColor struct {
		Hex string `json:"hex"`
	} `json:"backgroundColor"`
	Title struct {
		TransformedLabel string `json:"transformedLabel"`
	} `json:"title"`
}

// tile returns e, the item uri of a browse section, as a category's tile,
// or false if it is none: such as a link to a feature of the web player.
func (e *gqlEntity) tile(uri string) (Category, bool) {
	c := e.Data.CardRepresentation
	ok := e.Typename == "BrowseSectionContainer" && strings.HasPrefix(uri, "spotify:page:") && c.Title.TransformedLabel != ""
	return Category{ID: uri, Name: c.Title.TransformedLabel, Color: c.BackgroundColor.Hex, Image: c.Artwork.best()}, ok
}

type browseSections struct {
	Items []struct {
		Data struct {
			Title struct {
				TransformedLabel string `json:"transformedLabel"`
			} `json:"title"`
		} `json:"data"`
		SectionItems struct {
			Items []struct {
				URI     string `json:"uri"`
				Content struct {
					Data gqlEntity `json:"data"`
				} `json:"content"`
			} `json:"items"`
		} `json:"sectionItems"`
	} `json:"items"`
}

// shelves returns the rows of sections that have something to show.
func (s browseSections) shelves() []Shelf {
	var out []Shelf
	for _, sec := range s.Items {
		sh := Shelf{Title: sec.Data.Title.TransformedLabel}
		for _, it := range sec.SectionItems.Items {
			if t, ok := it.Content.Data.tile(it.URI); ok {
				sh.Tiles = append(sh.Tiles, t)
			} else if pl, ok := it.Content.Data.playlist(); ok {
				sh.Lists = append(sh.Lists, pl)
			}
		}
		if len(sh.Tiles)+len(sh.Lists) > 0 {
			out = append(out, sh)
		}
	}
	return out
}

// Categories returns the tiles of the browse page: genres, moods,
// podcasts.
func Categories() ([]Category, error) {
	var res struct {
		BrowseStart struct {
			Sections browseSections `json:"sections"`
		} `json:"browseStart"`
	}
	err := query("browseAll", map[string]any{
		"pagePagination":           map[string]int{"offset": 0, "limit": 10},
		"sectionPagination":        map[string]int{"offset": 0, "limit": 99},
		"browseEndUserIntegration": "INTEGRATION_WEB_PLAYER",
	}, &res)
	if err != nil {
		return nil, err
	}
	var out []Category
	for _, sh := range res.BrowseStart.Sections.shelves() {
		out = append(out, sh.Tiles...)
	}
	return out, nil
}

// CategoryDetail returns the page of the category uri.
func CategoryDetail(uri string) (*CategoryPage, error) {
	var res struct {
		Browse struct {
			Header struct {
				Title struct {
					TransformedLabel string `json:"transformedLabel"`
				} `json:"title"`
				Color struct {
					Hex string `json:"hex"`
				} `json:"color"`
			} `json:"header"`
			Sections browseSections `json:"sections"`
		} `json:"browse"`
	}
	err := query("browsePage", map[string]any{
		"uri":                      uri,
		"pagePagination":           map[string]int{"offset": 0, "limit": 10},
		"sectionPagination":        map[string]int{"offset": 0, "limit": 20},
		"browseEndUserIntegration": "INTEGRATION_WEB_PLAYER",
	}, &res)
	if err != nil {
		return nil, err
	}
	b := &res.Browse
	return &CategoryPage{Name: b.Header.Title.TransformedLabel, Color: b.Header.Color.Hex, Shelves: b.Sections.shelves()}, nil
}

// UserPage is someone's profile: who they are, and their public playlists.
type UserPage struct {
	ID                   string // their username
	Name                 string
	Image                string
	Color                int // 0xRRGGBB, what their initial shows on without a picture
	Followers, Following int
	Playlists            []Playlist
	Total                int // their public playlists, of which Playlists are the first
}

// UserProfile returns the profile of user, a username.
func UserProfile(user string) (*UserPage, error) {
	raw, err := spGet("/user-profile-view/v3/profile/"+url.PathEscape(user)+
		"?playlist_limit=50&artist_limit=10&episode_limit=10&market=from_token", "application/json")
	if err != nil {
		return nil, err
	}
	var v struct {
		Name      string `json:"name"`
		ImageURL  string `json:"image_url"`
		Color     int    `json:"color"`
		Followers int    `json:"followers_count"`
		Following int    `json:"following_count"`
		Total     int    `json:"total_public_playlists_count"`
		Playlists []struct {
			URI       string `json:"uri"`
			Name      string `json:"name"`
			ImageURL  string `json:"image_url"`
			OwnerName string `json:"owner_name"`
			OwnerURI  string `json:"owner_uri"`
		} `json:"public_playlists"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	pg := &UserPage{ID: user, Name: v.Name, Image: v.ImageURL, Color: v.Color, Followers: v.Followers, Following: v.Following, Total: v.Total}
	for _, p := range v.Playlists {
		pl := Playlist{ID: p.URI, Name: p.Name, CoverURL: imageURL(p.ImageURL), Owner: strings.TrimPrefix(p.OwnerURI, "spotify:user:")}
		pl.Creator.Nickname = p.OwnerName
		pg.Playlists = append(pg.Playlists, pl)
	}
	return pg, nil
}

// imageURL returns where the picture image is: a URL already, or the
// cover a playlist of the listener's makes of four albums' covers, which
// the profile names as spotify:mosaic:<id>:<id>:<id>:<id>.
func imageURL(image string) string {
	if ids, ok := strings.CutPrefix(image, "spotify:mosaic:"); ok {
		return "https://mosaic.scdn.co/640/" + strings.ReplaceAll(ids, ":", "")
	}
	return image
}
