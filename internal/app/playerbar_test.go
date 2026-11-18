// @author denovochen

package app

import (
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"vinyl/internal/l10n"
	"vinyl/internal/spotify"
)

type playerBarCase struct {
	name                             string
	connected, video, episode, empty bool
}

var playerBarCases = []playerBarCase{
	{name: "song"},
	{name: "connect", connected: true},
	{name: "mv", connected: true, video: true},
	{name: "podcast", connected: true, episode: true},
	{name: "video-podcast", connected: true, video: true, episode: true},
	{name: "empty", connected: true, empty: true},
}

func playerBarTestApp(tc playerBarCase) *app {
	a := newTestApp()
	a.settings = defaultSettings()
	a.setVolume(0.5)
	if tc.connected {
		a.connect = &connectState{client: &spotify.Connect{ID: "local"}}
	}
	if !tc.empty {
		a.pl.Cue([]spotify.Track{{ID: "7", Name: "那些你很冒险的梦 (Live)", Artists: []spotify.Artist{{Name: "林俊杰"}, {Name: "JJ Lin"}}, Duration: 200000, Episode: tc.episode}})
		a.pl.SetLyrics(nil)
		media := &spotify.Media{Bitrate: 256000}
		if tc.video {
			media.Video = "video"
		}
		a.pl.SetMedia(media)
		if tc.episode {
			a.pl.SetRate(2.5)
		}
	}
	return a
}

// Resizing keeps the full controls apart and inside the window: the device and video buttons used to push half the speaker out.
func TestPlayerBarResizeKeepsControlsVisible(t *testing.T) {
	for _, locale := range []string{"zh-Hans", "en", "it"} {
		t.Run(locale, func(t *testing.T) {
			testLanguage(t, locale)
			for _, tc := range playerBarCases {
				t.Run(tc.name, func(t *testing.T) {
					a := playerBarTestApp(tc)
					tt := ui.NewTester(a.view, 1180, 600)
					labels := []string{l10n.T("上一首"), l10n.T("播放/暂停"), l10n.T("下一首"), l10n.T("循环"), lyricsName(a.pl.Current()), l10n.T("播放队列"), l10n.T("音量")}
					if tc.connected {
						labels = append(labels, l10n.T("播放设备"))
					}
					if deskHere {
						labels = append(labels, l10n.T("桌面歌词"))
					}
					if !tc.empty && !tc.episode {
						labels = append(labels, l10n.T("喜欢"))
					}
					if tc.episode {
						labels = append(labels, l10n.T("播放速度"))
					} else {
						labels = append(labels, l10n.T("随机播放"))
					}
					var optional []string
					if !tc.empty {
						optional = append(optional, "256k")
					}
					if tc.video {
						optional = append(optional, mvAction(l10n.T("播放"), mvName(a.pl.Current())))
					}
					wide := make(map[string]ui.Rect, len(labels)+len(optional))
					for _, label := range append(append([]string{}, labels...), optional...) {
						r, ok := tt.Find(label)
						if !ok {
							t.Fatalf("no control %q", label)
						}
						wide[label] = r
					}
					for _, width := range []int{1000, 935, 880, 1180} {
						tt.SetSize(width, 600)
						visible := append([]string{}, labels...)
						for _, label := range optional {
							_, shown := tt.Find(label)
							if width == 880 && shown || width == 1180 && !shown {
								t.Fatalf("width %d: optional control %q shown %v", width, label, shown)
							}
							if shown {
								visible = append(visible, label)
							}
						}
						bounds := make([]ui.Rect, 0, len(visible))
						for _, label := range visible {
							r, ok := tt.Find(label)
							ref := wide[label]
							if !ok || r.W < ref.W-0.01 || r.H < ref.H-0.01 || r.X < sidebarWidth+20 || r.X+r.W > float32(width)-20+0.01 {
								t.Fatalf("width %d: %q clipped or outside the player bar: %v, wide %v", width, label, r, ref)
							}
							for j, other := range bounds {
								if r.X < other.X+other.W && r.X+r.W > other.X && r.Y < other.Y+other.H && r.Y+r.H > other.Y {
									t.Fatalf("width %d: %q overlaps %q: %v, %v", width, label, visible[j], r, other)
								}
							}
							bounds = append(bounds, r)
						}
						if err := tt.Click(l10n.T("音量")); err != nil {
							t.Fatal(err)
						}
						if !a.volumeOpen {
							t.Fatal("the volume panel did not open after resizing")
						}
						tt.Key(0, ui.KeyEscape)
						if a.volumeOpen {
							t.Fatal("the volume panel did not close")
						}
					}
				})
			}
		})
	}
}

// At the minimum window width, video podcast controls still change volume and seek only when the progress drag is released.
func TestPlayerBarNarrowVolumeAndSeek(t *testing.T) {
	testLanguage(t, "zh-Hans")
	a := playerBarTestApp(playerBarCase{connected: true, video: true, episode: true})
	a.pl.Hold(0)
	tt := ui.NewTester(a.view, 880, 600)
	if err := tt.Click(l10n.T("音量")); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyDown)
	if math.Abs(a.volume-0.49) > 0.001 || math.Abs(a.pl.Volume()-0.49) > 0.001 {
		t.Fatalf("volume control did not reach the player: app %v, player %v", a.volume, a.pl.Volume())
	}
	tt.Key(0, ui.KeyEscape)
	bar, ok := tt.Find(l10n.T("播放进度"))
	if !ok || bar.W <= 0 {
		t.Fatal("no usable progress bar")
	}
	y := bar.Y + bar.H/2
	tt.Press(bar.X+bar.W/4, y)
	tt.Move(bar.X+bar.W*0.75, y)
	if pos := a.pl.Position(); pos != 0 {
		t.Fatalf("seeked while dragging: %v", pos)
	}
	tt.Release(bar.X+bar.W*0.75, y)
	tt.Frame()
	if pos := a.pl.Position(); pos < 149*time.Second || pos > 151*time.Second {
		t.Fatalf("released at 75%%, seeked to %v", pos)
	}
}
