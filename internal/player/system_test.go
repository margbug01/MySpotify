package player

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

// systemEvent reports, as the page does, what the system did to out.
func systemEvent(p *Player, out *webOut, kind string, at float64, seq int) {
	page(p, http.MethodPost, "/event", fmt.Sprintf(`{"gen":%d,"type":%q,"t":%g,"seq":%d}`, out.gen, kind, at, seq))
}

// A pause from the system (Mami pausing what plays while one talks, the
// media keys, Control Center), which WebKit carries out on the page itself,
// left Vinyl showing the song playing and never letting the page go: the
// player now follows the page, and plays again with it.
func TestSystemPauseAndPlay(t *testing.T) {
	ch, p, _ := licensed(t, func(http.ResponseWriter, *http.Request) {})
	defer p.CloseWeb()
	out := p.out.(*webOut)
	if !p.Playing() {
		t.Fatal("not playing to begin with")
	}
	changed := 0
	p.Changed = func() { changed++ }
	systemEvent(p, out, "playing", 10, 0)
	systemEvent(p, out, "system-pause", 12.5, out.asked)
	next(t, ch)
	if p.Playing() || out.Playing() || p.Position() != 12500*time.Millisecond || changed == 0 || p.idle == nil {
		t.Fatalf("after the system's pause: playing %v, page playing %v, at %v, changed %d, idle timer %v", p.Playing(), out.Playing(), p.Position(), changed, p.idle != nil)
	}
	asked := out.asked
	systemEvent(p, out, "system-play", 12.5, out.asked)
	next(t, ch)
	if !p.Playing() || !out.Playing() || p.idle != nil {
		t.Fatal("did not follow the system playing again")
	}
	if out.asked != asked {
		t.Fatal("asked the page to play what already plays")
	}
}

// Vinyl's own pause fades the song out first: played again by the system,
// it played on silent.
func TestSystemPlayAfterVinylsPause(t *testing.T) {
	ch, p, _ := licensed(t, func(http.ResponseWriter, *http.Request) {})
	defer p.CloseWeb()
	out := p.out.(*webOut)
	p.TogglePlay()
	for out.Playing() {
		next(t, ch)
	}
	if !p.fade.silent() {
		t.Fatal("Vinyl's pause did not fade out")
	}
	systemEvent(p, out, "system-play", 3, out.asked)
	next(t, ch)
	p.fade.mu.Lock()
	to := p.fade.to
	p.fade.mu.Unlock()
	if !p.Playing() || to != 1 {
		t.Fatalf("played by the system: playing %v, fading to %v", p.Playing(), to)
	}
}

// What the page reports of the system after Vinyl asked it to play or pause
// is stale: Vinyl's ask stands.
func TestSystemPauseAfterVinylAsked(t *testing.T) {
	_, p, _ := licensed(t, func(http.ResponseWriter, *http.Request) {})
	defer p.CloseWeb()
	out := p.out.(*webOut)
	stale := out.asked
	out.Pause()
	out.Play()
	systemEvent(p, out, "system-pause", 4, stale)
	if !out.Playing() {
		t.Fatal("a stale pause of the system's stopped what Vinyl asked to play")
	}
}

// The media keys' and Control Center's skips, which WebKit passes the page,
// skip.
func TestSystemSkips(t *testing.T) {
	ch, p, _ := licensed(t, func(http.ResponseWriter, *http.Request) {})
	defer p.CloseWeb()
	systemEvent(p, p.out.(*webOut), "system-next", 1, 0)
	next(t, ch)
	for p.Loading() {
		next(t, ch)
	}
	if p.at != 1 {
		t.Fatalf("next: at %d", p.at)
	}
	systemEvent(p, p.out.(*webOut), "system-previous", 1, 0)
	next(t, ch)
	for p.Loading() {
		next(t, ch)
	}
	if p.at != 0 {
		t.Fatalf("previous: at %d", p.at)
	}
}
