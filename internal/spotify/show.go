package spotify

// A podcast shows as a list: its episodes are its tracks, which play as
// songs do. It is known by its URI, spotify:show:<ID>.

// showEpisodes is how many of a podcast's episodes, the latest first, its
// page lists.
//
// shortcut: the latest 200 only; revisit when listeners want the older
// ones, by loading more as the list scrolls to its end.
const showEpisodes = 200

// show returns the podcast uri, with its latest episodes as its tracks.
func show(uri string) (*Playlist, error) {
	var meta struct {
		PodcastUnionV2 struct {
			Typename    string   `json:"__typename"`
			URI         string   `json:"uri"`
			Name        string   `json:"name"`
			Description string   `json:"description"`
			CoverArt    gqlImage `json:"coverArt"`
			Publisher   struct {
				Name string `json:"name"`
			} `json:"publisher"`
		} `json:"podcastUnionV2"`
	}
	err := query("queryShowMetadataV2", map[string]any{"uri": uri, "includeContentCapabilityTrait": false, "includeEpisodeContentRatingsV2": false}, &meta)
	if err != nil {
		return nil, err
	}
	m := &meta.PodcastUnionV2
	if m.Typename != "Podcast" {
		return nil, ErrNotFound
	}
	pl := &Playlist{ID: uri, Name: m.Name, CoverURL: m.CoverArt.best(), Description: m.Description}
	pl.Creator.Nickname = m.Publisher.Name
	by := []Artist{{Name: m.Publisher.Name}}
	for offset := 0; offset < showEpisodes; {
		var res struct {
			PodcastUnionV2 struct {
				EpisodesV2 struct {
					TotalCount int `json:"totalCount"`
					Items      []struct {
						Entity struct {
							URI  string `json:"_uri"`
							Data struct {
								Name     string   `json:"name"`
								Duration millis   `json:"duration"`
								CoverArt gqlImage `json:"coverArt"`
								// Absent where the episode does not play
								// to this listener.
								Playability *struct {
									Playable bool `json:"playable"`
								} `json:"playability"`
							} `json:"data"`
						} `json:"entity"`
					} `json:"items"`
				} `json:"episodesV2"`
			} `json:"podcastUnionV2"`
		}
		err := query("queryPodcastEpisodes", map[string]any{"uri": uri, "offset": offset, "limit": 50, "includeEpisodeContentRatingsV2": false}, &res)
		if err != nil {
			return nil, err
		}
		eps := res.PodcastUnionV2.EpisodesV2
		pl.TrackCount = eps.TotalCount
		for _, it := range eps.Items {
			e := it.Entity
			cover := e.Data.CoverArt.best()
			if cover == "" {
				cover = pl.CoverURL
			}
			pl.Tracks = append(pl.Tracks, Track{
				ID: idOf(e.URI), Episode: true, Name: e.Data.Name, Duration: e.Data.Duration.TotalMilliseconds,
				Artists:    by,
				Album:      Album{ID: uri, Name: m.Name, PicURL: cover},
				Unplayable: e.Data.Playability == nil || !e.Data.Playability.Playable,
			})
		}
		offset += len(eps.Items)
		if len(eps.Items) == 0 || offset >= eps.TotalCount {
			break
		}
	}
	return pl, nil
}
