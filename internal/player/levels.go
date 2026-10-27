package player

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// The page decrypts and plays the audio out of the app's reach, but it plays
// it through a Web Audio analyser, which sees the sound itself once it is
// decrypted. The page long-polls /levels with what the analyser read, and
// the app draws its bars from that rather than from a shape made up out of
// the position: real bass looks like bass.
//
// A page that cannot have Web Audio sends nothing, and the bars fall back to
// the shape they always had.

// spectrum is the latest the page sent, and who may take it.
type spectrum struct {
	mu    sync.Mutex
	bands [Bands]float32
	at    time.Time // when the page read them, before the poll waited
	gen   int       // the track they belong to
	live  bool      // the page sent some, recently enough to trust
}

var live spectrum

// levelsGap paces the poll: the page asks, is answered a frame later, and
// asks again.
const levelsGap = 28 * time.Millisecond

// postLevels takes what the page read, and answers once another poll is
// worth making: at once while the music moves, after a pause once it stops.
func (w *Web) postLevels(rw http.ResponseWriter, r *http.Request) {
	var v struct {
		Gen int       `json:"gen"`
		Lv  []float32 `json:"lv"`
	}
	if json.NewDecoder(http.MaxBytesReader(rw, r.Body, 1<<12)).Decode(&v) != nil {
		http.Error(rw, "bad levels", http.StatusBadRequest)
		return
	}
	if len(v.Lv) == Bands {
		live.mu.Lock()
		copy(live.bands[:], v.Lv)
		live.at, live.gen, live.live = time.Now(), v.Gen, true
		live.mu.Unlock()
	}
	// The page asks again the moment this answers, so it answers at once
	// the spectrum has moved: a wait here is felt at the pointer, and the
	// bars are meant to move with the music. Thirty a second is what the
	// poll comes to, which is a frame's worth.
	time.Sleep(levelsGap)
	rw.WriteHeader(http.StatusNoContent)
}

// LiveLevels returns the spectrum the page last read, and whether it is
// current enough to draw: the page sending nothing (no Web Audio, or a
// page that went away) says no, and the caller falls back.
func (p *Player) LiveLevels() ([Bands]float32, bool) {
	var lv [Bands]float32
	if p.out == nil || !p.playing {
		return lv, false
	}
	live.mu.Lock()
	defer live.mu.Unlock()
	// Two seconds is a few frames of a stalled page and no more: past that
	// the bars would sit still while the music played on.
	if !live.live || time.Since(live.at) > 2*time.Second {
		return lv, false
	}
	return live.bands, true
}

// forgetLevels drops what the page last sent, when a new one takes over.
func forgetLevels() {
	live.mu.Lock()
	live.live = false
	live.bands = [Bands]float32{}
	live.mu.Unlock()
}
