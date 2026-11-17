package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"vinyl/internal/l10n"
	"vinyl/internal/spotify"
)

// In the narrowest window the player bar keeps the speaker in it, leaving
// out the bitrate's badge and the music video's button, which pushed it out;
// wider, they show (#43).
func TestPlayerBarKeepsTheSpeaker(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.pl.Cue([]spotify.Track{{ID: "7", Name: "那些你很冒险的梦 (Live)", Artists: []spotify.Artist{{Name: "林俊杰"}}, Duration: 199000}})
	a.pl.SetMedia(&spotify.Media{Bitrate: 256000, Video: "v"})
	for _, w := range []int{880, 1180} {
		tt := ui.NewTester(a.view, w, 600)
		tt.Frame()
		r, ok := tt.Find(l10n.T("音量"))
		if !ok || r.X+r.W > float32(w) {
			t.Fatalf("%d wide: the speaker at %+v (found %v), out of the window", w, r, ok)
		}
		_, badge := tt.Find("256k")
		if badge != (w > 880) {
			t.Errorf("%d wide: badge shown %v", w, badge)
		}
	}
}
