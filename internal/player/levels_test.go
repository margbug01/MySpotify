package player

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"vinyl/internal/spotify"
)

// levelPlayer is the player these tests read, playing.
var levelPlayer *Player

// playingPlayer returns a player playing a long song, and puts its own out
// of the way after the test.
func playingPlayer(t *testing.T) func() {
	t.Helper()
	_, p := posted()
	p.playing = true
	prev := levelPlayer
	levelPlayer = p
	return func() {
		levelPlayer = prev
		p.out.Close()
		forgetLevels()
	}
}

// livePlayer is the player the levels tests read.
func livePlayer(t *testing.T) *Player {
	t.Helper()
	if levelPlayer == nil {
		t.Fatal("no player")
	}
	return levelPlayer
}

// testMedia is a song of a minute, its manifest naming one file.
func testMedia(t *testing.T) *spotify.Media {
	t.Helper()
	return &spotify.Media{
		Manifest: "#EXTM3U\n#EXT-X-MAP:URI=\"https://cdn.test/a\",BYTERANGE=\"100@0\"\n#EXT-X-BYTERANGE:100@100\nhttps://cdn.test/a\n#EXT-X-ENDLIST\n",
		Duration: time.Minute,
	}
}

// The page's spectrum is what the bars draw: the music itself, not a shape
// made up out of the position.
func TestThePageSetsTheBars(t *testing.T) {
	restore := playingPlayer(t)
	defer restore()
	forgetLevels()

	w := &Web{addr: "127.0.0.1:1", cmds: make(chan string, 8), vol: make(chan float64, 1), outs: map[int]*webOut{}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://"+w.addr+"/levels",
		strings.NewReader(`{"gen":1,"lv":[0.1,0.2,0.3,0.4,0.5,0.6,0.7,0.8,0.9,1,0.9,0.8,0.7,0.6,0.5,0.4,0.3,0.2,0.1,0,0.2,0.4,0.6,0.8]}`))
	w.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("post levels: %d", rec.Code)
	}

	lv, ok := livePlayer(t).Levels()
	if !ok {
		t.Fatal("no levels while playing")
	}
	if lv[0] != 0.1 || lv[9] != 1 || lv[23] != 0.8 {
		t.Fatalf("bars are the made-up ones, not the page's: %v", lv)
	}
}

// A spectrum older than a couple of seconds is not the music any more: the
// bars fall back rather than sitting still while it plays on.
func TestAStaleSpectrumIsNotUsed(t *testing.T) {
	restore := playingPlayer(t)
	defer restore()
	forgetLevels()

	live.mu.Lock()
	copy(live.bands[:], make([]float32, Bands))
	live.bands[0] = 0.99
	live.at, live.live = time.Now().Add(-3*time.Second), true
	live.mu.Unlock()

	lv, ok := livePlayer(t).Levels()
	if !ok {
		t.Fatal("no levels while playing")
	}
	if lv[0] == 0.99 {
		t.Fatal("drew a spectrum three seconds old")
	}
}

// Paused, there is no spectrum to draw, whatever the page last sent.
func TestNoLevelsWhilePaused(t *testing.T) {
	restore := playingPlayer(t)
	defer restore()
	forgetLevels()

	live.mu.Lock()
	live.bands[0], live.at, live.live = 1, time.Now(), true
	live.mu.Unlock()

	p := livePlayer(t)
	p.playing = false
	if lv, ok := p.Levels(); ok || lv[0] != 0 {
		t.Fatalf("levels while paused: %v %v", lv, ok)
	}
}

// A new track starts with no bars rather than the last one's.
func TestANewTrackForgetsTheBars(t *testing.T) {
	forgetLevels()
	live.mu.Lock()
	live.bands[0], live.at, live.live = 0.7, time.Now(), true
	live.mu.Unlock()

	w := &Web{addr: "127.0.0.1:1", cmds: make(chan string, 8), vol: make(chan float64, 1), outs: map[int]*webOut{}}
	w.Open(testMedia(t))

	live.mu.Lock()
	defer live.mu.Unlock()
	if live.live || live.bands[0] != 0 {
		t.Fatal("the last track's bars carried over")
	}
}

// A page without Web Audio sends nothing, and the bars are drawn the way
// they always were.
func TestWithoutThePageTheBarsAreDrawnAsBefore(t *testing.T) {
	restore := playingPlayer(t)
	defer restore()
	forgetLevels()

	p := livePlayer(t)
	p.Hold(30 * time.Second)
	lv, ok := p.Levels()
	if !ok {
		t.Fatal("no levels while playing")
	}
	moved := false
	for _, v := range lv {
		if v > 0 && v < 1 {
			moved = true
		}
	}
	if !moved {
		t.Fatalf("the stand-in bars are flat: %v", lv)
	}
}
