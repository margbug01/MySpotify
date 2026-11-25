package app

import (
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/egoist/mygo/audio"
	"github.com/egoist/mygo/ui"

	"vinyl/internal/spotify"
)

// The notch, turned off, shows itself as the first song plays, and only
// then; its button keeps it on.
func TestNotchGreetsOnce(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the notch is macOS's")
	}
	a := newTestApp()
	a.settings = defaultSettings()
	n := &notch{a: a, w0: 200, h0: 32, winW: notchOpenMax + 60, winH: 240}
	a.notch = n
	a.pl.Cue([]spotify.Track{{ID: "7", Name: "宝船", Duration: 200000}})
	a.syncWindows()
	if n.hello {
		t.Fatal("greeted before the song played")
	}
	out := audio.NewPlayer(&silence{})
	out.SetVolume(0)
	a.pl.Attach(out)
	defer out.Close()
	a.syncWindows()
	if !n.hello || !a.settings.NotchGreeted {
		t.Fatal("did not greet as the song played")
	}

	tt := ui.NewTester(n.view, int(n.winW), int(n.winH))
	tt.Advance(time.Second)
	tt.Frame()
	if err := tt.Click("开启刘海播放器"); err != nil {
		t.Fatal(err)
	}
	if !a.settings.Notch || n.hello {
		t.Fatalf("kept: notch %v, greeting %v", a.settings.Notch, n.hello)
	}

	// Turned off again, it does not greet the next song.
	a.settings.Notch = false
	a.pl.Cue([]spotify.Track{{ID: "8", Name: "下一首", Duration: 200000}})
	a.pl.Attach(out)
	a.syncWindows()
	if n.hello {
		t.Error("greeted twice")
	}
}

// With NM_FRAMES set, saves the notch as it greets, growing and grown, and
// the player bar holding a song a login picked up.
func TestFirstUseFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	save := func(name string, tt *ui.Tester) {
		f, _ := os.Create(filepath.Join(dir, name+".png"))
		png.Encode(f, tt.Image())
		f.Close()
	}
	a := newTestApp()
	a.settings = defaultSettings()
	n := &notch{a: a, real: true, w0: 185, h0: 32, winW: notchOpenMax + 60, winH: 240}
	a.notch = n
	a.pl.Cue([]spotify.Track{{ID: "7", Name: "NAINAINAI", Artists: []spotify.Artist{{Name: "ATARASHII GAKKO!"}}, Duration: 193877}})
	main := ui.NewTester(a.view, 1200, 800) // for the app's colors
	main.Frame()
	tt := ui.NewTester(n.view, int(n.winW), int(n.winH))
	tt.Frame()
	n.greet()
	for _, ms := range []int{120, 600} {
		tt.Advance(time.Duration(ms) * time.Millisecond)
		tt.Frame()
		save(fmt.Sprintf("notch-hello-%d", ms), tt)
	}

}

// The expanded-width slider in the settings sets how wide the open notch
// player grows, and keeps it.
func TestNotchWidthSlider(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.Notch = true
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	n := &notch{a: a, real: true, w0: 185, h0: 32, winW: notchOpenMax + 60, winH: 240, open: true}
	a.notch = n
	a.pl.Cue([]spotify.Track{{ID: "7", Name: "宝船", Duration: 200000}})
	tt := settingsTester(a)
	// The row says the slider's name too: find the slider itself.
	s, ok := tt.AccessByRole(ui.RoleSlider, "展开宽度")
	if !ok {
		t.Fatal("no width slider")
	}
	tt.ClickAt(s.Bounds.X+s.Bounds.W/2, s.Bounds.Y+s.Bounds.H/2)
	tt.Frame()
	clicked := a.settings.NotchWidth
	if clicked == 0 {
		t.Fatal("clicking the slider set no width")
	}
	for range 3 {
		tt.Key(0, ui.KeyLeft)
	}
	tt.Frame()
	if w, _ := n.target(); w != float32(clicked-30) {
		t.Fatalf("open notch %v wide, want %v", w, clicked-30)
	}
	b, err := os.ReadFile(a.settings.path)
	var saved settings
	if err != nil || json.Unmarshal(b, &saved) != nil || saved.NotchWidth != clicked-30 {
		t.Fatalf("saved width %v, %v", saved.NotchWidth, err)
	}
}

// The resting-width slider in the settings sets how far the notch reaches
// out beside the housing as music plays, and keeps it.
func TestNotchWingSlider(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.Notch = true
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	n := &notch{a: a, real: true, w0: 185, h0: 32, winW: notchOpenMax + 60, winH: 240}
	a.notch = n
	a.pl.Cue([]spotify.Track{{ID: "7", Name: "宝船", Duration: 200000}})
	tt := settingsTester(a)
	s, ok := tt.AccessByRole(ui.RoleSlider, "常驻宽度")
	if !ok {
		t.Fatal("no resting-width slider")
	}
	tt.ClickAt(s.Bounds.X+s.Bounds.W/2, s.Bounds.Y+s.Bounds.H/2)
	tt.Frame()
	clicked := a.settings.NotchWing
	if clicked == 0 {
		t.Fatal("clicking the slider set no width")
	}
	for range 3 {
		tt.Key(0, ui.KeyLeft)
	}
	tt.Frame()
	if w, _ := n.target(); w != 185+2*float32(clicked-6) {
		t.Fatalf("resting notch %v wide, want %v", w, 185+2*(clicked-6))
	}
	b, err := os.ReadFile(a.settings.path)
	var saved settings
	if err != nil || json.Unmarshal(b, &saved) != nil || saved.NotchWing != clicked-6 {
		t.Fatalf("saved wing %v, %v", saved.NotchWing, err)
	}
}
