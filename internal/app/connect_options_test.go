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

// A change Spotify would not take shows what it says again, and one it
// took but never says so is let go after a while.
func TestHeldLetsGo(t *testing.T) {
	var h held[bool]
	ask := h.set(true)
	h.set(false)
	h.answered(ask, false) // an answer to the change before
	if v, _ := h.show(true, true); v {
		t.Fatal("an earlier change's answer let go of the latest")
	}
	h.answered(ask+1, false)
	if v, _ := h.show(true, true); !v {
		t.Fatal("refused, still held")
	}
	ask = h.set(false)
	h.answered(ask, true)
	if v, ok := h.show(true, false); v || !ok {
		t.Fatal("not shown while Spotify says nothing")
	}
	h.until = time.Now().Add(-time.Second)
	if v, _ := h.show(true, true); !v {
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
