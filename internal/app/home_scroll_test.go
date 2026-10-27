package app

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"vinyl/internal/spotify"
)

// Shelf arrows and horizontal wheel input expose later cards without moving another shelf or opening a card.
func TestHomeShelvesScrollIndependently(t *testing.T) {
	a := newTestApp()
	sections := testSections(2)
	for index := range sections {
		sections[index].Items = nil
		for card := 0; card < 12; card++ {
			list := &spotify.Playlist{ID: fmt.Sprintf("spotify:playlist:%d-%d", index, card), Name: fmt.Sprintf("歌单 %d-%d", index, card)}
			sections[index].Items = append(sections[index].Items, spotify.HomeItem{List: list})
		}
		sections[index].Total = 12
	}
	var bounds ui.Rect
	var page ui.ScrollState
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Scroll(c).TrackScroll(&page).Padding(20, 20, 20, 260).Children(func() {
			for index := range sections {
				a.shelf(c.Key(index), &sections[index])
			}
			ui.Box(c).Height(500)
		})
		if state := a.homeShelves[":"+sectionKey(&sections[0])]; state != nil {
			bounds = state.Bounds(c)
		}
	}, 1100, 800)
	tt.Frame()
	tt.Frame()
	first := a.homeShelves[":"+sectionKey(&sections[0])]
	second := a.homeShelves[":"+sectionKey(&sections[1])]
	if first.MaxX <= 0 || second.MaxX <= 0 {
		t.Fatal("cards were truncated instead of placed in a scrollable row")
	}
	if err := tt.Click("分区 0：向右滑动"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	if first.X <= 0 || second.X != 0 || a.next != nil {
		t.Fatal("arrow did not scroll just its own shelf")
	}
	if err := tt.Click("分区 0：向左滑动"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	if first.X > 1 {
		t.Fatalf("left arrow did not return: %v", first.X)
	}
	tt.Scroll(bounds.X+bounds.W/2, bounds.Y+40, 200, 0)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	if first.X <= 0 || second.X != 0 || a.next != nil {
		t.Fatal("horizontal wheel did not scroll independently")
	}
	first.X = first.MaxX
	tt.Frame()
	before := first.X
	tt.Click("分区 0：向右滑动")
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	if first.X > first.MaxX || first.X != before {
		t.Fatal("right boundary was not respected")
	}
	if err := tt.Click("歌单 0-11"); err != nil {
		t.Fatal(err)
	}
	if a.next == nil {
		t.Fatal("later card did not open after scrolling")
	}
	a.next = nil
	tt.Scroll(bounds.X+bounds.W/2, bounds.Y+40, 0, 200)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	if page.Y <= 0 || first.X != before {
		t.Fatal("vertical wheel did not continue scrolling the page")
	}
}
