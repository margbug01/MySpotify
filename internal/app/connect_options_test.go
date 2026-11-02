package app

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"vinyl/internal/player"
	"vinyl/internal/spotify"
)

// cluster has Spotify say the shuffle and repeat of a, as it does after
// anything changes.
func cluster(a *app, shuffle, all, one bool) {
	o, _ := json.Marshal(map[string]bool{"shuffling_context": shuffle, "repeating_context": all, "repeating_track": one})
	a.connectEvent(spotify.ConnectEvent{Kind: "cluster", Data: json.RawMessage(`{"active_device_id":"local","player_state":{"options":` + string(o) + `}}`)})
}

// took has Spotify take the latest shuffle and repeat asked of it, as the
// answers the jobs post would: posts go nowhere in tests.
func took(a *app) {
	a.connect.shuffle.answered(a.connect.shuffle.ask, true)
	a.connect.repeat.answered(a.connect.repeat.ask, true)
	a.connectOptions(spotify.ConnectOptions{})
}

// Shuffle and repeat pressed show at once, and what Spotify says from
// before it took them doesn't undo them: the buttons went back, then
// forward again, as it caught up.
func TestConnectOptionsHoldTillSpotifySays(t *testing.T) {
	defer func(old func(*spotify.Connect, string, string, map[string]any) error) { connectCommand = old }(connectCommand)
	var asked []string
	connectCommand = func(_ *spotify.Connect, _, endpoint string, _ map[string]any) error {
		asked = append(asked, endpoint)
		return nil
	}
	a := connectTestApp()
	cluster(a, false, false, false)
	a.pl.ToggleShuffle()
	a.pl.CycleRepeat()
	if !a.pl.Shuffle() || a.pl.Repeat() != player.RepeatAll || len(a.connect.jobs) != 2 {
		t.Fatalf("pressed: shuffle %v, repeat %v, %d asked", a.pl.Shuffle(), a.pl.Repeat(), len(a.connect.jobs))
	}
	(<-a.connect.jobs)()
	(<-a.connect.jobs)()
	if len(asked) != 2 || asked[0] != "set_shuffling_context" || asked[1] != "set_options" {
		t.Fatalf("asked %v", asked)
	}
	took(a)
	cluster(a, false, false, false) // from before
	if !a.pl.Shuffle() || a.pl.Repeat() != player.RepeatAll {
		t.Fatalf("undone: shuffle %v, repeat %v", a.pl.Shuffle(), a.pl.Repeat())
	}
	cluster(a, true, true, false)  // caught up
	cluster(a, false, false, true) // changed elsewhere since
	if a.pl.Shuffle() || a.pl.Repeat() != player.RepeatOne {
		t.Fatalf("another device's change was not shown: shuffle %v, repeat %v", a.pl.Shuffle(), a.pl.Repeat())
	}
}

// Pressed twice, shuffle asks for what Spotify said before the first
// press, which let go of it at once: Spotify's answer to the first press
// then set the button back, live on Spotify's servers. Spotify may also
// take the presses out of order, for seconds saying what the first asked
// after it took the second: that doesn't set it back either.
func TestConnectOptionsPressedBack(t *testing.T) {
	defer func(old func(*spotify.Connect, string, string, map[string]any) error) { connectCommand = old }(connectCommand)
	connectCommand = func(*spotify.Connect, string, string, map[string]any) error { return nil }
	a := connectTestApp()
	cluster(a, true, true, false)
	a.pl.ToggleShuffle()
	a.pl.ToggleShuffle()
	cluster(a, false, true, false) // the first press's answer
	if !a.pl.Shuffle() {
		t.Fatal("the first press's answer set it back")
	}
	for len(a.connect.jobs) > 0 {
		(<-a.connect.jobs)()
	}
	took(a)
	cluster(a, false, true, false) // the first's still, the second taken
	if !a.pl.Shuffle() {
		t.Fatal("set back after the second was taken")
	}
	cluster(a, true, true, false)  // the second's
	cluster(a, false, true, false) // changed elsewhere since
	if a.pl.Shuffle() {
		t.Fatal("another device's change was not shown")
	}

	// Again, the second press taken before the first's answer comes.
	a.pl.ToggleShuffle()
	a.pl.ToggleShuffle()
	for len(a.connect.jobs) > 0 {
		(<-a.connect.jobs)()
	}
	took(a)
	cluster(a, true, true, false) // the first press's answer
	if a.pl.Shuffle() {
		t.Fatal("the first press's answer, after the second was taken, set it back")
	}
	cluster(a, false, true, false) // the second's
	cluster(a, true, true, false)  // changed elsewhere since
	if !a.pl.Shuffle() {
		t.Fatal("another device's change was not shown, the second time")
	}

	// Repeat, pressed round to where it was.
	cluster(a, true, false, false)
	a.pl.CycleRepeat()
	a.pl.CycleRepeat()
	a.pl.CycleRepeat()
	for len(a.connect.jobs) > 0 {
		(<-a.connect.jobs)()
	}
	took(a)
	cluster(a, true, true, false) // the first press's answer
	if a.pl.Repeat() != player.RepeatOff {
		t.Fatalf("the first press's answer set repeat to %v", a.pl.Repeat())
	}
}

// A change Spotify would not take shows what it says again; one it took is
// let go once the latest it said is the same, or after a while.
func TestHeldLetsGo(t *testing.T) {
	var h held[bool]
	ask := h.set(true)
	h.set(false)
	h.answered(ask, false) // an answer to the change before
	if v, _ := h.show(true, true, true); v {
		t.Fatal("an earlier change's answer let go of the latest")
	}
	h.answered(ask+1, false)
	if v, _ := h.show(true, true, false); !v {
		t.Fatal("refused, still held")
	}

	ask = h.set(false)
	h.show(false, true, true) // said before it was taken
	if !h.on {
		t.Fatal("let go before Spotify took it")
	}
	h.answered(ask, true)
	h.show(false, true, false)
	if h.on {
		t.Fatal("taken and said, still held")
	}

	ask = h.set(false)
	h.show(false, true, true)
	h.show(true, true, true) // and otherwise since
	h.answered(ask, true)
	if v, _ := h.show(true, true, false); v {
		t.Fatal("let go, Spotify last saying otherwise")
	}

	ask = h.set(false)
	h.answered(ask, true)
	if v, ok := h.show(true, false, false); v || !ok || !h.until.After(time.Now()) {
		t.Fatalf("taken: shown %v, %v, held till %v", v, ok, h.until)
	}
	h.until = time.Now().Add(-time.Second)
	if v, _ := h.show(true, true, false); !v {
		t.Fatal("held past its time")
	}
}

// A press Spotify can't be asked of, its queue full, shows what it said.
func TestConnectOptionsUnsentGoBack(t *testing.T) {
	a := connectTestApp()
	cluster(a, false, false, false)
	for len(a.connect.jobs) < cap(a.connect.jobs) {
		a.connect.jobs <- func() error { return errors.New("busy") }
	}
	a.pl.ToggleShuffle()
	if a.pl.Shuffle() {
		t.Fatal("shuffle shown on, never asked")
	}
}
