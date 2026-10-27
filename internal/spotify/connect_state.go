package spotify

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

type ConnectDevice struct {
	Name         string   `json:"name"`
	Type         string   `json:"device_type"`
	Volume       looseInt `json:"volume"`
	Capabilities struct {
		Hidden  bool  `json:"hidden"`
		CanPlay *bool `json:"can_be_player"`
	} `json:"capabilities"`
}

type ConnectCluster struct {
	Active    string                   `json:"active_device_id"`
	Timestamp looseInt                 `json:"server_timestamp_ms"`
	Devices   map[string]ConnectDevice `json:"devices"`
	Player    struct {
		Track ConnectTrack `json:"track"`
		// Next is what plays after Track, as Spotify's apps list it.
		Next      []ConnectTrack `json:"next_tracks"`
		Playing   bool           `json:"is_playing"`
		Paused    bool           `json:"is_paused"`
		Position  looseInt       `json:"position_as_of_timestamp"`
		Timestamp looseInt       `json:"timestamp"`
		Duration  looseInt       `json:"duration"`
		Options   ConnectOptions `json:"options"`
	} `json:"player_state"`
}

func ParseConnectCluster(raw []byte) (ConnectCluster, error) {
	var wrapper struct {
		Cluster json.RawMessage `json:"cluster"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return ConnectCluster{}, err
	}
	if len(wrapper.Cluster) > 0 {
		raw = wrapper.Cluster
	}
	var result ConnectCluster
	err := json.Unmarshal(raw, &result)
	return result, err
}

// ConnectTrack is a track of the cluster's player state: the one playing,
// or one of those after it, which the state's metadata may only name.
type ConnectTrack struct {
	URI      string            `json:"uri"`
	UID      string            `json:"uid"`
	Metadata map[string]string `json:"metadata"`
}

func (c ConnectCluster) Track() Track {
	track := c.Player.Track.track()
	if c.Player.Duration > 0 {
		track.Duration = int64(c.Player.Duration)
	}
	return track
}

// Queue is the track playing and those after it as Spotify's apps show
// them: without the context's hidden repeats, and the delimiters between
// them.
func (c ConnectCluster) Queue() []Track {
	if connectTrackID(c.Player.Track.URI) == "" {
		return nil
	}
	queue := []Track{c.Track()}
	for _, next := range c.Player.Next {
		if next.Metadata["hidden"] != "true" && connectTrackID(next.URI) != "" {
			queue = append(queue, next.track())
		}
	}
	return queue
}

func (t ConnectTrack) track() Track {
	meta := t.Metadata
	cover := meta["image_large_url"]
	if cover == "" {
		cover = meta["image_url"]
	}
	if strings.HasPrefix(cover, "spotify:image:") {
		cover = "https://i.scdn.co/image/" + strings.TrimPrefix(cover, "spotify:image:")
	}
	duration, _ := strconv.ParseInt(meta["duration"], 10, 64)
	track := Track{ID: connectTrackID(t.URI), UID: t.UID, Episode: strings.HasPrefix(t.URI, "spotify:episode:"), Name: meta["title"], Duration: duration, Album: Album{ID: meta["album_uri"], Name: meta["album_title"], PicURL: cover}}
	// The artists after the first are artist_name:1, artist_name:2...
	for i := 0; ; i++ {
		key := ""
		if i > 0 {
			key = ":" + strconv.Itoa(i)
		}
		if meta["artist_name"+key] == "" {
			break
		}
		track.Artists = append(track.Artists, Artist{Name: meta["artist_name"+key], ID: meta["artist_uri"+key]})
	}
	return track
}

func connectTrackID(uri string) string {
	for _, prefix := range []string{"spotify:track:", "spotify:episode:"} {
		if strings.HasPrefix(uri, prefix) {
			return strings.TrimPrefix(uri, prefix)
		}
	}
	return ""
}

type ConnectRef struct {
	Index  int  `json:"state_index"`
	Paused bool `json:"paused"`
}

type ConnectOptions struct {
	Shuffle       *bool `json:"shuffling_context"`
	RepeatContext *bool `json:"repeating_context"`
	RepeatTrack   *bool `json:"repeating_track"`
}

type ConnectMachine struct {
	Attributes struct {
		Options ConnectOptions `json:"options"`
	} `json:"attributes"`
	ID     string `json:"state_machine_id"`
	Tracks []struct {
		Metadata struct {
			URI      string   `json:"uri"`
			Name     string   `json:"name"`
			Album    string   `json:"group_name"`
			AlbumURI string   `json:"group_uri"`
			Duration looseInt `json:"duration"`
			Authors  []struct {
				Name string `json:"name"`
				URI  string `json:"uri"`
			} `json:"authors"`
			Images []struct {
				URL string `json:"url"`
			} `json:"images"`
		} `json:"metadata"`
	} `json:"tracks"`
	States []struct {
		ID          string `json:"state_id"`
		Track       int    `json:"track"`
		NoSeek      bool   `json:"disallow_seeking"`
		Transitions struct {
			Advance *ConnectRef `json:"advance"`
		} `json:"transitions"`
	} `json:"states"`
}

type ConnectCommand struct {
	Type     string          `json:"type"`
	Ref      *ConnectRef     `json:"state_ref"`
	Previous json.RawMessage `json:"prev_state_ref"`
	Machine  ConnectMachine  `json:"state_machine"`
	Seek     *int64          `json:"seek_to"`
	Volume   *int            `json:"volume"`
}

func (m ConnectMachine) Queue(index int) []Track {
	var tracks []Track
	seen := map[int]bool{}
	for len(tracks) < 50 && !seen[index] {
		track, err := m.Track(index)
		if err != nil {
			break
		}
		seen[index] = true
		tracks = append(tracks, track)
		next := m.States[index].Transitions.Advance
		if next == nil {
			break
		}
		index = next.Index
	}
	return tracks
}

func (m ConnectMachine) Track(index int) (Track, error) {
	if index < 0 || index >= len(m.States) {
		return Track{}, errors.New("Connect: 无效播放状态")
	}
	at := m.States[index].Track
	if at < 0 || at >= len(m.Tracks) {
		return Track{}, errors.New("Connect: 无效曲目")
	}
	meta := m.Tracks[at].Metadata
	id := connectTrackID(meta.URI)
	if id == "" {
		return Track{}, errors.New("Connect: 此内容类型暂不支持，已暂停本机")
	}
	track := Track{ID: id, Episode: strings.HasPrefix(meta.URI, "spotify:episode:"), Name: meta.Name, Duration: int64(meta.Duration), Album: Album{ID: meta.AlbumURI, Name: meta.Album}}
	if len(meta.Images) > 0 {
		track.Album.PicURL = meta.Images[0].URL
	}
	for _, artist := range meta.Authors {
		track.Artists = append(track.Artists, Artist{Name: artist.Name, ID: artist.URI})
	}
	return track, nil
}
