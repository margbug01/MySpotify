package spotify

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Artist struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Album struct {
	ID     string `json:"id"` // its URI
	Name   string `json:"name"`
	PicURL string `json:"picUrl"`
}

type Track struct {
	ID       string   `json:"id"` // base62, as in spotify:track:<ID>
	Name     string   `json:"name"`
	Artists  []Artist `json:"artists"`
	Album    Album    `json:"album"`
	Duration int64    `json:"duration"` // milliseconds
	// Unplayable marks a track Spotify won't play to this listener.
	Unplayable bool `json:"unplayable,omitempty"`
}

// ArtistNames joins the track's artists as the credits read.
func (t *Track) ArtistNames() string {
	names := make([]string, len(t.Artists))
	for i, a := range t.Artists {
		names[i] = a.Name
	}
	return strings.Join(names, ", ")
}

func (t *Track) Playable() bool        { return !t.Unplayable }
func (t *Track) Length() time.Duration { return time.Duration(t.Duration) * time.Millisecond }
func (t *Track) URI() string           { return "spotify:track:" + t.ID }

// URL is the track's page on the web player.
func (t *Track) URL() string { return "https://open.spotify.com/track/" + t.ID }

// Playlist is a list of tracks the pages show as one: a playlist, an
// album, or the listener's liked songs.
type Playlist struct {
	ID          string // its URI
	Name        string
	CoverURL    string
	Description string
	TrackCount  int
	Creator     struct{ Nickname string }
	// Artists are an album's.
	Artists []Artist
	// Note is what a card says under the name, as a release's year and
	// kind; the creator if empty.
	Note   string
	Tracks []Track
	// Preview holds a chart's top songs, as its card shows them.
	Preview []ChartLine
}

// ChartLine is a song at the top of a chart: its name and its artist.
type ChartLine struct {
	First  string `json:"first"`
	Second string `json:"second"`
}

func (p *Playlist) Cover() string { return p.CoverURL }

// LikedURI stands for the listener's liked songs.
const LikedURI = "spotify:collection:tracks"

// Liked reports whether p is the listener's liked songs.
func (p *Playlist) Liked() bool { return p.ID == LikedURI }

// Sized asks the image CDN for a picture at least n pixels across, where
// it serves more than one size, and says after "#", which no request
// sends, the n the picture is to be shrunk to: Spotify's sizes jump from
// 300 to 640 pixels.
func Sized(pic string, n int) string {
	if pic == "" {
		return ""
	}
	return cdnSized(pic, n) + "#" + strconv.Itoa(n)
}

func cdnSized(pic string, n int) string {
	if rest, ok := strings.CutPrefix(pic, "https://mosaic.scdn.co/640/"); ok {
		size := "640"
		switch {
		case n <= 60:
			size = "60"
		case n <= 300:
			size = "300"
		}
		return "https://mosaic.scdn.co/" + size + "/" + rest
	}
	if i := strings.Index(pic, "/ab676161"); i >= 0 && len(pic) >= i+17 {
		// An artist's picture: as album art, but its own sizes.
		code := "0000e5eb"
		switch {
		case n <= 160:
			code = "0000f178"
		case n <= 320:
			code = "00005174"
		}
		return pic[:i+9] + code + pic[i+17:]
	}
	if i := strings.Index(pic, "/ab67616d"); i >= 0 && len(pic) >= i+17 {
		// Album art: its size is coded in the ID.
		code := "0000b273"
		switch {
		case n <= 64:
			code = "00004851"
		case n <= 300:
			code = "00001e02"
		}
		return pic[:i+9] + code + pic[i+17:]
	}
	return pic
}

func idOf(uri string) string { return uri[strings.LastIndexByte(uri, ':')+1:] }

type gqlImage struct {
	Sources []struct {
		URL      string `json:"url"`
		Width    int    `json:"width"`
		MaxWidth int    `json:"maxWidth"` // an artist's banner says this instead
	} `json:"sources"`
}

// best returns the widest picture.
func (im gqlImage) best() string {
	url, w := "", -1
	for _, s := range im.Sources {
		if sw := max(s.Width, s.MaxWidth); sw > w {
			url, w = s.URL, sw
		}
	}
	return url
}

type gqlImages struct {
	Items []gqlImage `json:"items"`
}

func (ims gqlImages) best() string {
	if len(ims.Items) == 0 {
		return ""
	}
	return ims.Items[0].best()
}

type gqlArtists struct {
	Items []struct {
		URI     string `json:"uri"`
		Profile struct {
			Name string `json:"name"`
		} `json:"profile"`
	} `json:"items"`
}

func (as gqlArtists) list() []Artist {
	out := make([]Artist, len(as.Items))
	for i, a := range as.Items {
		out[i] = Artist{ID: idOf(a.URI), Name: a.Profile.Name}
	}
	return out
}

type millis struct {
	TotalMilliseconds int64 `json:"totalMilliseconds"`
}

// gqlTrack is a track as the GraphQL queries give it.
type gqlTrack struct {
	Typename     string     `json:"__typename"`
	URI          string     `json:"uri"`
	Name         string     `json:"name"`
	Artists      gqlArtists `json:"artists"`
	AlbumOfTrack struct {
		URI      string   `json:"uri"`
		Name     string   `json:"name"`
		CoverArt gqlImage `json:"coverArt"`
	} `json:"albumOfTrack"`
	Duration      millis `json:"duration"`      // in search results and albums
	TrackDuration millis `json:"trackDuration"` // in playlists
	Playability   struct {
		Playable bool `json:"playable"`
	} `json:"playability"`
}

// track returns t as a Track, on album if its own isn't given.
func (t *gqlTrack) track(album *Album) Track {
	tr := Track{
		ID:         idOf(t.URI),
		Name:       t.Name,
		Artists:    t.Artists.list(),
		Duration:   max(t.Duration.TotalMilliseconds, t.TrackDuration.TotalMilliseconds),
		Unplayable: !t.Playability.Playable,
	}
	if album != nil {
		tr.Album = *album
	} else {
		tr.Album = Album{ID: t.AlbumOfTrack.URI, Name: t.AlbumOfTrack.Name, PicURL: t.AlbumOfTrack.CoverArt.best()}
	}
	return tr
}

// tracks returns the songs among items, skipping episodes and what isn't a
// track, as local files.
func tracks(items []*gqlTrack, album *Album) []Track {
	out := make([]Track, 0, len(items))
	for _, t := range items {
		if t != nil && t.Typename != "Episode" && strings.HasPrefix(t.URI, "spotify:track:") {
			out = append(out, t.track(album))
		}
	}
	return out
}

// gqlEntity is a card of the home or browse pages: a playlist, an album
// or an artist.
type gqlEntity struct {
	Typename    string     `json:"__typename"`
	URI         string     `json:"uri"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Images      gqlImages  `json:"images"`   // playlists
	CoverArt    gqlImage   `json:"coverArt"` // albums
	Artists     gqlArtists `json:"artists"`
	OwnerV2     struct {
		Data struct {
			Name     string `json:"name"`
			Username string `json:"username"`
		} `json:"data"`
	} `json:"ownerV2"`
	Content struct {
		TotalCount int `json:"totalCount"`
	} `json:"content"`
	// Artists
	Profile struct {
		Name string `json:"name"`
	} `json:"profile"`
	Visuals struct {
		AvatarImage gqlImage `json:"avatarImage"`
	} `json:"visuals"`
}

// artist returns e as an ArtistCard, or false if it is no artist.
func (e *gqlEntity) artist() (ArtistCard, bool) {
	ok := e.Typename == "Artist" && e.URI != ""
	return ArtistCard{ID: e.URI, Name: e.Profile.Name, Avatar: e.Visuals.AvatarImage.best()}, ok
}

// playlist returns e as a Playlist, or false if it is no list of songs.
func (e *gqlEntity) playlist() (Playlist, bool) {
	pl := Playlist{ID: e.URI, Name: e.Name, Description: stripTags(e.Description), TrackCount: e.Content.TotalCount}
	switch e.Typename {
	case "Playlist":
		pl.CoverURL = e.Images.best()
		pl.Creator.Nickname = e.OwnerV2.Data.Name
	case "Album":
		pl.CoverURL = e.CoverArt.best()
		pl.Artists = e.Artists.list()
		pl.Creator.Nickname = Track{Artists: pl.Artists}.ArtistNamesOf()
	default:
		return pl, false
	}
	return pl, e.URI != ""
}

// ArtistNamesOf is ArtistNames on a value.
func (t Track) ArtistNamesOf() string { return t.ArtistNames() }

// stripTags drops the links Spotify puts in descriptions.
func stripTags(s string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		switch {
		case r == '<':
			in = true
		case r == '>':
			in = false
		case !in:
			b.WriteRune(r)
		}
	}
	return strings.NewReplacer("&amp;", "&", "&#x27;", "'", "&quot;", "\"", "&lt;", "<", "&gt;", ">").Replace(b.String())
}

type gqlSections struct {
	Items []struct {
		URI  string `json:"uri"`
		Data struct {
			Title struct {
				TransformedLabel string `json:"transformedLabel"`
			} `json:"title"`
		} `json:"data"`
		SectionItems struct {
			Items []struct {
				Content struct {
					Data gqlEntity `json:"data"`
				} `json:"content"`
			} `json:"items"`
		} `json:"sectionItems"`
	} `json:"items"`
}

// Home is what the home page shows.
type Home struct {
	Greeting string
	// Daily is the URI of the listener's first Daily Mix, or "".
	Daily string
	// ForYou are the playlists made for the listener.
	ForYou []Playlist
	// Featured are the rest of the home's playlists and albums.
	Featured []Playlist
	// Artists are the artists the home shows.
	Artists []ArtistCard
}

// The home's section of playlists made for the listener.
const forYouSection = "spotify:section:0JQ5DAUnp4wcj0bCb3wh3S"

// featuredMax is how many of the rest the home page shows: of over a
// hundred, those first, in the home's order.
const featuredMax = 36

// artistsMax is how many artists the home page shows, of the twenty or so
// its sections hold.
const artistsMax = 12

// HomeFeed returns the listener's home, as the web player shows it.
func HomeFeed() (*Home, error) {
	var res struct {
		Home struct {
			Greeting struct {
				TransformedLabel string `json:"transformedLabel"`
			} `json:"greeting"`
			SectionContainer struct {
				Sections gqlSections `json:"sections"`
			} `json:"sectionContainer"`
		} `json:"home"`
	}
	err := query("home", map[string]any{
		"timeZone": timeZone(), "sp_t": "", "facet": "", "sectionItemsLimit": 20,
		"homeEndUserIntegration": "INTEGRATION_WEB_PLAYER",
	}, &res)
	if err != nil {
		return nil, err
	}
	h := &Home{Greeting: res.Home.Greeting.TransformedLabel}
	seen := map[string]bool{}
	for _, s := range res.Home.SectionContainer.Sections.Items {
		for _, it := range s.SectionItems.Items {
			if ar, ok := it.Content.Data.artist(); ok && !seen[ar.ID] {
				seen[ar.ID] = true
				if len(h.Artists) < artistsMax {
					h.Artists = append(h.Artists, ar)
				}
				continue
			}
			pl, ok := it.Content.Data.playlist()
			if !ok || seen[pl.ID] {
				continue
			}
			seen[pl.ID] = true
			if s.URI != forYouSection {
				if len(h.Featured) < featuredMax {
					h.Featured = append(h.Featured, pl)
				}
				continue
			}
			h.ForYou = append(h.ForYou, pl)
			// Daily Mixes are 37i9dQZF1E3…; Discover Weekly and radios aren't.
			if h.Daily == "" && strings.HasPrefix(pl.ID, "spotify:playlist:37i9dQZF1E3") {
				h.Daily = pl.ID
			}
		}
	}
	if h.Daily == "" && len(h.ForYou) > 0 {
		h.Daily = h.ForYou[0].ID
	}
	return h, nil
}

func timeZone() string {
	if tz := time.Local.String(); tz != "Local" && tz != "" {
		return tz
	}
	return "Asia/Shanghai"
}

// The charts page, and its sections: the featured charts, and the daily
// Top 50 of each country.
const (
	chartsPage    = "spotify:page:0JQ5DAudkNjCgYMM0TZXDw"
	chartsFeature = "spotify:section:0JQ5DAzQHECxDlYNI6xD1g"
	chartsDaily   = "spotify:section:0JQ5DAzQHECxDlYNI6xD1i"
)

// chartsFirst are the places whose charts come first, before the rest in
// Spotify's order.
var chartsFirst = []string{"全球", "台湾", "香港", "日本", "韩国", "美国", "英国", "新加坡", "马来西亚"}

// Charts returns Spotify's charts: the featured ones first, with their
// top songs, then each country's.
func Charts() ([]Playlist, error) {
	var res struct {
		Browse struct {
			Sections gqlSections `json:"sections"`
		} `json:"browse"`
	}
	err := query("browsePage", map[string]any{
		"uri":                      chartsPage,
		"pagePagination":           map[string]int{"offset": 0, "limit": 10},
		"sectionPagination":        map[string]int{"offset": 0, "limit": 80},
		"browseEndUserIntegration": "INTEGRATION_WEB_PLAYER",
	}, &res)
	if err != nil {
		return nil, err
	}
	var featured, daily []Playlist
	seen := map[string]bool{}
	for _, s := range res.Browse.Sections.Items {
		if s.URI != chartsFeature && s.URI != chartsDaily {
			continue
		}
		for _, it := range s.SectionItems.Items {
			pl, ok := it.Content.Data.playlist()
			if !ok || seen[pl.ID] {
				continue
			}
			seen[pl.ID] = true
			if s.URI == chartsFeature {
				featured = append(featured, pl)
			} else {
				daily = append(daily, pl)
			}
		}
	}
	rank := func(name string) int {
		for i, p := range chartsFirst {
			if strings.HasPrefix(name, p) {
				return i
			}
		}
		return len(chartsFirst)
	}
	sort.SliceStable(daily, func(i, j int) bool { return rank(daily[i].Name) < rank(daily[j].Name) })
	// The cards of the featured charts list their top three.
	var wg sync.WaitGroup
	for i := range featured {
		wg.Add(1)
		go func(pl *Playlist) {
			defer wg.Done()
			top, err := playlistPage(pl.ID, 0, 3)
			if err != nil {
				return
			}
			for _, t := range top.Tracks {
				pl.Preview = append(pl.Preview, ChartLine{First: t.Name, Second: t.ArtistNames()})
			}
		}(&featured[i])
	}
	wg.Wait()
	return append(featured, daily...), nil
}

// PlaylistDetail returns the playlist, album or liked songs uri stands
// for, with all its tracks.
func PlaylistDetail(uri string) (*Playlist, error) {
	switch {
	case uri == LikedURI:
		return likedSongs()
	case strings.HasPrefix(uri, "spotify:album:"):
		return album(uri)
	}
	const page = 100
	pl, err := playlistPage(uri, 0, page)
	for err == nil && pl.fetched < pl.TrackCount {
		var more *pagedPlaylist
		more, err = playlistPage(uri, pl.fetched, page)
		if more == nil || more.fetched == 0 {
			break
		}
		pl.Tracks = append(pl.Tracks, more.Tracks...)
		pl.fetched += more.fetched
	}
	if err != nil {
		return nil, err
	}
	return &pl.Playlist, nil
}

type pagedPlaylist struct {
	Playlist
	fetched int // items fetched, songs or not
}

func playlistPage(uri string, offset, limit int) (*pagedPlaylist, error) {
	var res struct {
		PlaylistV2 struct {
			gqlEntity
			Content struct {
				TotalCount int `json:"totalCount"`
				Items      []struct {
					ItemV2 struct {
						Data *gqlTrack `json:"data"`
					} `json:"itemV2"`
				} `json:"items"`
			} `json:"content"`
		} `json:"playlistV2"`
	}
	err := query("fetchPlaylist", map[string]any{"uri": uri, "offset": offset, "limit": limit, "enableWatchFeedEntrypoint": false}, &res)
	if err != nil {
		return nil, err
	}
	p := &res.PlaylistV2
	if p.URI == "" {
		return nil, ErrNotFound
	}
	pl, _ := p.gqlEntity.playlist()
	pl.TrackCount = p.Content.TotalCount
	items := make([]*gqlTrack, len(p.Content.Items))
	for i, it := range p.Content.Items {
		items[i] = it.ItemV2.Data
	}
	pl.Tracks = tracks(items, nil)
	return &pagedPlaylist{Playlist: pl, fetched: len(items)}, nil
}

func album(uri string) (*Playlist, error) {
	var pl *Playlist
	for offset := 0; ; {
		var res struct {
			AlbumUnion struct {
				gqlEntity
				TracksV2 struct {
					TotalCount int `json:"totalCount"`
					Items      []struct {
						Track *gqlTrack `json:"track"`
					} `json:"items"`
				} `json:"tracksV2"`
			} `json:"albumUnion"`
		}
		err := query("getAlbum", map[string]any{"uri": uri, "locale": "", "offset": offset, "limit": 50}, &res)
		if err != nil {
			return nil, err
		}
		a := &res.AlbumUnion
		if a.URI == "" {
			return nil, ErrNotFound
		}
		if pl == nil {
			a.Typename = "Album"
			p, _ := a.gqlEntity.playlist()
			p.TrackCount = a.TracksV2.TotalCount
			pl = &p
		}
		al := Album{ID: uri, Name: pl.Name, PicURL: pl.CoverURL}
		items := make([]*gqlTrack, len(a.TracksV2.Items))
		for i, it := range a.TracksV2.Items {
			if it.Track != nil {
				it.Track.Typename = "Track"
			}
			items[i] = it.Track
		}
		pl.Tracks = append(pl.Tracks, tracks(items, &al)...)
		offset += len(items)
		if len(items) == 0 || offset >= a.TracksV2.TotalCount {
			return pl, nil
		}
	}
}

// Search returns the tracks matching q, the best first.
func Search(q string, limit int) ([]Track, error) {
	var res struct {
		SearchV2 struct {
			TracksV2 struct {
				Items []struct {
					Item struct {
						Data *gqlTrack `json:"data"`
					} `json:"item"`
				} `json:"items"`
			} `json:"tracksV2"`
		} `json:"searchV2"`
	}
	err := query("searchTracks", map[string]any{
		"searchTerm": q, "offset": 0, "limit": limit, "numberOfTopResults": 20,
		"includeAudiobooks": false, "includePreReleases": false, "includeAuthors": false,
	}, &res)
	if err != nil {
		return nil, err
	}
	items := make([]*gqlTrack, len(res.SearchV2.TracksV2.Items))
	for i, it := range res.SearchV2.TracksV2.Items {
		items[i] = it.Item.Data
	}
	return tracks(items, nil), nil
}

// Profile is who is signed in.
type Profile struct {
	UserID    string `json:"userId"` // the username, as in spotify:user:<UserID>
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatarUrl"`
}

// Account returns the profile of who is signed in.
func Account() (*Profile, error) {
	var res struct {
		Me struct {
			Profile struct {
				Name     string   `json:"name"`
				Username string   `json:"username"`
				Avatar   gqlImage `json:"avatar"`
			} `json:"profile"`
		} `json:"me"`
	}
	if err := query("profileAttributes", map[string]any{}, &res); err != nil {
		return nil, err
	}
	p := res.Me.Profile
	return &Profile{UserID: p.Username, Nickname: p.Name, AvatarURL: p.Avatar.best()}, nil
}

// Library is what the listener keeps: the playlists they made, their
// liked songs first, the playlists and albums they saved, and the artists
// they follow.
type Library struct {
	Made, Saved []Playlist
	Artists     []ArtistCard
}

// UserLibrary returns the library of user, the listener signed in.
func UserLibrary(user string) (*Library, error) {
	lib := &Library{Made: []Playlist{{ID: LikedURI, Name: "我喜欢的音乐"}}}
	for offset := 0; ; {
		var res struct {
			Me struct {
				LibraryV3 struct {
					TotalCount int `json:"totalCount"`
					Items      []struct {
						Item struct {
							Data gqlEntity `json:"data"`
						} `json:"item"`
					} `json:"items"`
				} `json:"libraryV3"`
			} `json:"me"`
		}
		err := query("libraryV3", map[string]any{
			"filters": []string{}, "order": nil, "textFilter": "", "features": []string{"LIKED_SONGS"},
			"limit": 50, "offset": offset, "flatten": true, "expandedFolders": []string{}, "folderUri": nil,
			"includeFoldersWhenFlattening": false,
		}, &res)
		if err != nil {
			return lib, err
		}
		page := res.Me.LibraryV3
		for _, it := range page.Items {
			e := it.Item.Data
			if e.Typename == "PseudoPlaylist" && e.URI == LikedURI {
				lib.Made[0].TrackCount = e.Content.TotalCount
				continue
			}
			if ar, ok := e.artist(); ok {
				lib.Artists = append(lib.Artists, ar)
				continue
			}
			pl, ok := e.playlist()
			switch {
			case !ok:
			case e.Typename == "Playlist" && e.OwnerV2.Data.Username == user:
				lib.Made = append(lib.Made, pl)
			default:
				lib.Saved = append(lib.Saved, pl)
			}
		}
		offset += len(page.Items)
		if len(page.Items) == 0 || offset >= page.TotalCount {
			return lib, nil
		}
	}
}

// libraryTrack is a liked song.
type libraryTrack struct {
	Track struct {
		Data *gqlTrack `json:"data"`
		URI  string    `json:"_uri"`
	} `json:"track"`
}

func libraryPage(offset, limit int) ([]libraryTrack, error) {
	var res struct {
		Me struct {
			Library struct {
				Tracks struct {
					Items []libraryTrack `json:"items"`
				} `json:"tracks"`
			} `json:"library"`
		} `json:"me"`
	}
	err := query("fetchLibraryTracks", map[string]any{"offset": offset, "limit": limit}, &res)
	return res.Me.Library.Tracks.Items, err
}

// likedSongs returns the listener's liked songs, the latest first.
func likedSongs() (*Playlist, error) {
	pl := &Playlist{ID: LikedURI, Name: "我喜欢的音乐"}
	for offset := 0; ; {
		items, err := libraryPage(offset, 50)
		if err != nil {
			return nil, err
		}
		ts := make([]*gqlTrack, len(items))
		for i, it := range items {
			ts[i] = it.Track.Data
			if ts[i] != nil && ts[i].URI == "" {
				ts[i].URI = it.Track.URI
			}
		}
		pl.Tracks = append(pl.Tracks, tracks(ts, nil)...)
		offset += len(items)
		if len(items) < 50 {
			pl.TrackCount = len(pl.Tracks)
			if len(pl.Tracks) > 0 {
				pl.CoverURL = pl.Tracks[0].Album.PicURL
			}
			return pl, nil
		}
	}
}

// LikedIDs returns the IDs of the songs the listener likes.
func LikedIDs() ([]string, error) {
	pl, err := likedSongs()
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(pl.Tracks))
	for i, t := range pl.Tracks {
		ids[i] = t.ID
	}
	return ids, nil
}

// Like adds the track id to the listener's liked songs, or takes it out.
func Like(id string, like bool) error { return Save("spotify:track:"+id, like) }

// Save adds what uri stands for, a track or an artist followed, to the
// listener's library, or takes it out.
func Save(uri string, save bool) error {
	op := "addToLibrary"
	if !save {
		op = "removeFromLibrary"
	}
	var res json.RawMessage
	return query(op, map[string]any{"libraryItemUris": []string{uri}}, &res)
}
