package app

import (
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"vinyl/internal/spotify"
)

// npTabsApp opens Now Playing on its lyrics, at w×h, the pointer in it so
// its tabs show.
func npTabsApp(t *testing.T, w, h float32) (*app, *ui.Tester) {
	a := newTestApp()
	a.pl.Cue([]spotify.Track{{ID: "7", Name: "海屿你", Artists: []spotify.Artist{{Name: "马也_Crabbit"}}, Duration: 200000}})
	a.nowPlaying, a.npPane = true, paneLyrics
	tt := ui.NewTester(a.view, int(w), int(h))
	tt.SetDark(true)
	tt.Move(w/2, h/2)
	tt.Advance(time.Second)
	return a, tt
}

/** Verifies that the Now Playing pane tabs stay centered in the top bar without shifting across pane switches, while the pane content remains below them. */
func TestPaneTabsStayPut(t *testing.T) {
	for _, size := range [][2]float32{{760, 800}, {1200, 800}, {760, 480}} {
		w := size[0]
		_, tt := npTabsApp(t, w, size[1])
		where := func() ui.Rect {
			t.Helper()
			r, ok := tt.Find("律动")
			if !ok {
				t.Fatal("no tabs")
			}
			return r
		}
		first := where()
		if first.X >= w/2 || first.X+first.W <= w/2 || first.Y+first.H > topBarHeight {
			t.Fatalf("%v: tabs (%v) are not centered in the top bar", size, first)
		}
		for _, tab := range []string{"律动", "列表", "律动", "歌词"} {
			if err := tt.Click(tab); err != nil {
				t.Fatal(err)
			}
			tt.Advance(time.Second)
			if r := where(); r != first {
				t.Fatalf("%v, on %s: tabs at %v, were at %v", size, tab, r, first)
			}
			if tab == "列表" {
				if row, ok := tt.Find("海屿你"); !ok || row.Y < first.Y+first.H {
					t.Fatalf("%v: queue row (%v) starts above the tabs (%v)", size, row, first)
				}
			}
		}
	}
}

// With NM_FRAMES set, saves Now Playing's panes at window widths, to look
// at where the tabs are.
func TestPaneTabsFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	for _, size := range [][2]float32{{760, 800}, {910, 800}, {1200, 800}, {760, 560}} {
		_, tt := npTabsApp(t, size[0], size[1])
		for _, tab := range []string{"歌词", "律动", "列表"} {
			tt.Click(tab)
			tt.Advance(time.Second)
			f, _ := os.Create(filepath.Join(dir, "np-"+strconv.Itoa(int(size[0]))+"x"+strconv.Itoa(int(size[1]))+"-"+tab+".png"))
			png.Encode(f, tt.Image())
			f.Close()
		}
	}
}
