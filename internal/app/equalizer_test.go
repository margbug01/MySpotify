package app

import (
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// Dragging a point of the equalizer's curve sets the gain of its band, to a
// whole dB and within 12 either way, and leaves the other bands as they are.
func TestEqualizerCurveIsDragged(t *testing.T) {
	defer func(was bool) { eqHere = was }(eqHere)
	eqHere = true
	a, _ := updateApp(nil, nil)
	a.settings.EQ = true
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	tt := settingsTester(a)
	r, ok := tt.Find("均衡器曲线")
	if !ok {
		t.Fatal("the curve isn't shown")
	}
	// The fourth of six bands, from its top to the middle and past the foot.
	x := r.X + eqLabelW + (r.W-eqLabelW)*3.5/6
	tt.Press(x, r.Y+eqY(12, r.H))
	if got := a.settings.EQGains[3]; got != 12 {
		t.Fatalf("pressed at the top: %v", got)
	}
	tt.Move(x, r.Y+eqY(0, r.H))
	if got := a.settings.EQGains[3]; got != 0 {
		t.Fatalf("dragged to the middle: %v", got)
	}
	if _, err := os.Stat(a.settings.path); err == nil {
		t.Fatal("saved while the point was still held")
	}
	tt.Move(x+300, r.Y+r.H+200)
	tt.Release(x+300, r.Y+r.H+200)
	if a.settings.EQGains != [6]float64{0, 0, 0, -12, 0, 0} {
		t.Fatalf("dragged past the foot: %v", a.settings.EQGains)
	}
	// Let go, it is kept: a drag before quitting reaches the file.
	b, err := os.ReadFile(a.settings.path)
	if err != nil {
		t.Fatal(err)
	}
	var saved settings
	if err := json.Unmarshal(b, &saved); err != nil || saved.EQGains != a.settings.EQGains {
		t.Fatalf("saved gains %v, %v", saved.EQGains, err)
	}
}

// A band's name takes the focus and its arrow keys step its gain, kept at
// once, as the sliders the curve replaced did: the curve alone left keys and
// screen readers no way to set it.
func TestEqualizerBandKeys(t *testing.T) {
	defer func(was bool) { eqHere = was }(eqHere)
	eqHere = true
	a, _ := updateApp(nil, nil)
	a.settings.EQ = true
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	tt := settingsTester(a)
	click(t, tt, "1 kHz")
	for range 14 {
		tt.Key(0, ui.KeyUp)
	}
	tt.Key(0, ui.KeyLeft)
	tt.Frame()
	if a.settings.EQGains != [6]float64{0, 0, 0, 11, 0, 0} {
		t.Fatalf("gains %v, want the 1 kHz band at 12 less one", a.settings.EQGains)
	}
	b, err := os.ReadFile(a.settings.path)
	var saved settings
	if err != nil || json.Unmarshal(b, &saved) != nil || saved.EQGains != a.settings.EQGains {
		t.Fatalf("saved gains %v, %v", saved.EQGains, err)
	}
}

// With NM_FRAMES set, saves the equalizer's curve of a preset, light and dark.
func TestEqualizerFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	defer func(was bool) { eqHere = was }(eqHere)
	eqHere = true
	for _, dark := range []bool{false, true} {
		a, _ := updateApp(nil, nil)
		a.settings.EQ = true
		a.settings.EQGains = eqPresets[6].gains
		tt := settingsTester(a)
		tt.SetDark(dark)
		tt.Frame()
		name := "equalizer-light.png"
		if dark {
			name = "equalizer-dark.png"
		}
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		png.Encode(f, tt.Image())
		f.Close()
	}
}
