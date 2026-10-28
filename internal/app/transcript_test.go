package app

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"vinyl/internal/lyrics"
	"vinyl/internal/spotify"
)

// numberedLines are n lines of lyrics, one every 3 seconds, each named by
// its number.
func numberedLines(n int) []lyrics.Line {
	var lines []lyrics.Line
	for i := range n {
		at := time.Duration(i) * 3 * time.Second
		l := lyrics.Line{At: at, End: at + 3*time.Second, Text: fmt.Sprintf("line %04d, said as it plays", i)}
		l.Words = lyrics.SpreadWords(l.Text, l.At, l.End)
		lines = append(lines, l)
	}
	return lines
}

// settle lets the lyrics glide to the line said.
func settle(tt *ui.Tester) {
	for range 40 {
		tt.Advance(time.Second / 25)
		tt.Frame()
	}
}

// An episode's transcript, thousands of lines, builds only those about
// what shows: the line said comes a third of the way down, there and
// after a seek, as a song's does. A frame of all of them took a tenth of
// a second, and one came every 40 ms as the words lit up.
func TestLongLyricsBuildOnlyWhatShows(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.pl.Cue([]spotify.Track{{ID: "ep", Name: "Episode", Episode: true, Duration: 9000 * 1000}})
	a.pl.SetLyrics(numberedLines(3000))
	a.pl.Hold(1500*3*time.Second + 100*time.Millisecond)
	a.nowPlaying, a.npPane = true, paneLyrics
	tt := ui.NewTester(a.view, 1200, 800)
	settle(tt)
	said := func(i int) ui.Rect {
		t.Helper()
		r, ok := tt.Find(fmt.Sprintf("line %04d, said as it plays", i))
		if !ok {
			t.Fatalf("line %d is not shown", i)
		}
		return r
	}
	// The first line after Now Playing opens is placed as it still opens,
	// as a song's is: the next is where they all go.
	a.pl.Seek(1501*3*time.Second + 100*time.Millisecond)
	settle(tt)
	r := said(1501)
	for _, far := range []int{0, 1400, 1600, 2999} {
		if _, ok := tt.Find(fmt.Sprintf("line %04d, said as it plays", far)); ok {
			t.Fatalf("line %d is built, far from what shows", far)
		}
	}
	a.pl.Seek(10*3*time.Second + 100*time.Millisecond)
	settle(tt)
	if again := said(10); again.Y < r.Y-2 || again.Y > r.Y+2 {
		t.Fatalf("after the seek, the line said is at %v, was at %v", again, r)
	}
	if _, ok := tt.Find("line 1501, said as it plays"); ok {
		t.Fatal("the line left is still built")
	}
	if _, ok := tt.Find("暂无字幕"); ok {
		t.Fatal("says there is none")
	}
	if _, ok := tt.Find("字幕"); !ok {
		t.Fatal("the pane is not called 字幕")
	}
}

// A song's lyrics are built whole, as they were.
func TestSongLyricsBuildWhole(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.pl.Cue([]spotify.Track{{ID: "1", Name: "Song", Duration: 400000}})
	a.pl.SetLyrics(numberedLines(lyricsAll))
	a.pl.Hold(3*time.Second + 100*time.Millisecond)
	a.nowPlaying, a.npPane = true, paneLyrics
	tt := ui.NewTester(a.view, 1200, 800)
	settle(tt)
	if _, ok := tt.Find(fmt.Sprintf("line %04d, said as it plays", lyricsAll-1)); !ok {
		t.Fatal("the last line is not built")
	}
	if _, ok := tt.Find("歌词"); !ok {
		t.Fatal("the pane is not called 歌词")
	}
}

// Over a video podcast's video, its transcript shows as subtitles, called
// 字幕, with nothing to move it by: it keeps the episode's time.
func TestEpisodesTranscriptIsItsSubtitles(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.pl.Cue([]spotify.Track{{ID: "ep", Name: "Episode", Episode: true, Duration: 1174000}})
	a.pl.SetLyrics([]lyrics.Line{
		{At: 5 * time.Second, End: 6300 * time.Millisecond, Text: "Hello everyone."},
		{At: 6300 * time.Millisecond, End: 9 * time.Second, Text: "I hope you all are doing amazing today."},
	})
	a.pl.ShowVideo(&mygo.WebView{})
	a.pl.Seek(5500 * time.Millisecond)
	a.mvMode = mvTheater
	tt := ui.NewTester(a.view, 1200, 800)
	a.nowPlaying = true
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	tt.Move(600, 300)
	tt.Frame()
	for _, s := range []string{"Hello everyone.", "隐藏字幕"} {
		if _, ok := tt.Find(s); !ok {
			t.Fatalf("no %q", s)
		}
	}
	for _, s := range []string{"歌词提前 0.5 秒 ([)", "歌词延后 0.5 秒 (])", "隐藏歌词"} {
		if _, ok := tt.Find(s); ok {
			t.Fatalf("%q over an episode", s)
		}
	}
}
