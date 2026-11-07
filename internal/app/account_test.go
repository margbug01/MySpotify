package app

import (
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"vinyl/internal/spotify"
)

// Clicking the account row drops its menu, which takes the focus so the
// arrows walk its items, and Escape or a second click puts it away.
func TestAccountMenu(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.profile = &spotify.Profile{UserID: "1", Nickname: "小明"}
	tt := ui.NewTester(a.view, 1000, 700)
	tt.Frame()

	if err := tt.Click("小明"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.accountOpen || !tt.HasText("退出登录") {
		t.Fatalf("menu not open: %v", a.accountOpen)
	}
	if !tt.Focused("个人主页") {
		t.Fatal("the first item has no focus")
	}
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	if !tt.Focused("设置…") {
		t.Fatal("Down did not move to the settings")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.accountOpen {
		t.Fatal("Escape left the menu open")
	}

	tt.Click("小明")
	tt.Frame()
	tt.Click("小明")
	tt.Frame()
	if a.accountOpen {
		t.Fatal("a second click left the menu open")
	}
}

// The account menu's separator stays on screen once the menu has settled,
// at a fractional scale: it was a half-DIP box, which snaps to no device
// pixel where the line lands between pixels.
func TestAccountMenuDividerStays(t *testing.T) {
	for _, scale := range []float32{1.25, 1.5} {
		a := newTestApp()
		a.settings = defaultSettings()
		a.profile = &spotify.Profile{UserID: "1", Nickname: "小明"}
		tt := ui.NewTester(a.view, 1000, 700)
		tt.SetScale(scale)
		tt.SetDark(true)
		tt.Frame()
		tt.Click("小明")
		tt.Advance(time.Second)
		tt.Frame()

		// The rows a little above the sign-out item, halfway across the
		// menu: the separator's, a step lighter than the panel's fill.
		signOut, ok := tt.Find("退出登录")
		if !ok {
			t.Fatal("no sign-out item")
		}
		img := tt.Image()
		x := int((signOut.X + signOut.W/2) * scale)
		top := int(signOut.Y * scale)
		found := false
		for y := top - 24; y < top; y++ {
			if c := img.RGBAAt(x, y); c.R >= 58 {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("no separator shows above the sign-out item at scale %v", scale)
		}
	}
}

// With NM_FRAMES set, saves the foot of the sidebar with the account menu
// open, light and dark: the first letter of a name with Spotify's color,
// and of one without.
func TestAccountMenuFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	for _, dark := range []bool{false, true} {
		a := newTestApp()
		a.settings = defaultSettings()
		a.profile = &spotify.Profile{UserID: "1", Nickname: "Alex Chen", AvatarColor: 0x19e68c}
		if dark {
			a.profile = &spotify.Profile{UserID: "1", Nickname: "小明"}
		}
		tt := ui.NewTester(a.view, 1000, 600)
		tt.SetDark(dark)
		tt.Frame()
		tt.Click(a.profile.Nickname)
		tt.Advance(time.Second)
		tt.Frame()
		img := tt.Image()
		s := float32(img.Bounds().Dx()) / 1000
		crop := image.Rect(0, int(400*s), int((sidebarWidth+20)*s), img.Bounds().Dy())
		out := image.NewRGBA(image.Rect(0, 0, crop.Dx(), crop.Dy()))
		draw.Draw(out, out.Bounds(), img, crop.Min, draw.Src)
		name := "account-light.png"
		if dark {
			name = "account-dark.png"
		}
		f, _ := os.Create(filepath.Join(dir, name))
		png.Encode(f, out)
		f.Close()
	}
}
