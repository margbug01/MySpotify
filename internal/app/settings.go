package app

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
)

// settings are the listener's choices, kept in settings.json in the user
// data directory.
type settings struct {
	// Notch shows the quick player around the notch: the listener turns
	// it on.
	Notch bool `json:"notch"`
	// NotchActivity shows the cover and the music's levels beside the
	// notch while music plays, which hides the menu bar there.
	NotchActivity bool `json:"notchActivity"`
	// NotchVirtual keeps a notch of the app's own showing at the top of a
	// display without one; else it shows only while the pointer asks.
	NotchVirtual bool `json:"notchVirtual"`
	// NotchGreeted says the notch showed itself, once, as the first song
	// played (greet).
	NotchGreeted bool `json:"notchGreeted"`
	// Appearance is light, dark, or system to follow the system.
	Appearance string `json:"appearance"`
	// Smoke makes the progress bars comets trailing smoke as music plays,
	// for a few percent of a core: the listener turns it on.
	Smoke bool `json:"smoke"`
	// LyricsStage shows lyrics on the stage, Folia's style, in Now
	// Playing, rather than scrolling.
	LyricsStage bool `json:"lyricsStage"`
	// Volume is where the volume was left, from 0 to 1.
	Volume float64 `json:"volume"`
	// Fog fills Now Playing's backdrop with fog of the cover's color,
	// rising and swelling with the bass: an experiment the listener turns
	// on.
	Fog bool `json:"fog"`
	// MVFill has music videos fill Now Playing, rather than show where
	// the cover goes; and MVLyrics shows the lines sung over them then.
	MVFill   bool `json:"mvFill"`
	MVLyrics bool `json:"mvLyrics"`
	// MVLeads is how many milliseconds the listener has the music videos
	// of songs, by their IDs, run ahead of them: the lyrics follow.
	MVLeads map[string]int64 `json:"mvLeads,omitempty"`
	// MVQueue shows the queue beside music videos filling Now Playing.
	MVQueue bool `json:"mvQueue"`

	// CacheMB is how many megabytes the parts of tracks kept on disk may
	// take; 0 keeps none.
	CacheMB int `json:"cacheMB"`

	// AutoUpdate looks for a new version on opening and twice a day.
	AutoUpdate bool `json:"autoUpdate"`

	path string // where they were loaded from, and are saved to
}

func defaultSettings() settings {
	return settings{NotchActivity: true, Appearance: "system", Volume: 0.8, CacheMB: 1024, MVLyrics: true, MVQueue: true, AutoUpdate: true}
}

func settingsPath() (string, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}

// loadSettings reads the settings, or returns the defaults.
func loadSettings() settings {
	s := defaultSettings()
	if path, err := settingsPath(); err == nil {
		if b, err := os.ReadFile(path); err == nil {
			json.Unmarshal(b, &s)
		}
		s.path = path
	}
	return s
}

// save keeps the settings where they were loaded from; defaults that were
// never loaded, as in tests, are not saved.
func (s settings) save() error {
	path := s.path
	if path == "" {
		return nil
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// applyAppearance makes the windows light, dark, or follow the system.
func (s settings) applyAppearance() {
	switch s.Appearance {
	case "light":
		mygo.Theme.SetSource(mygo.ThemeLight)
	case "dark":
		mygo.Theme.SetSource(mygo.ThemeDark)
	default:
		mygo.Theme.SetSource(mygo.ThemeSystem)
	}
}
