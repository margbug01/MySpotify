package player

import (
	"testing"
	"time"
)

func TestWebEndRetryDoesNotAdvanceTwice(t *testing.T) {
	// Retrying an acknowledged end must not end a repeat or advance the queue twice.
	output := &webOut{dur: 250 * time.Second, playing: true, running: true}
	calls := 0
	output.OnEnd(func() { calls++ })
	output.event("ended", 249*time.Second, "tail stalled", 1)
	if calls != 1 || output.Playing() || output.Position() != output.Duration() {
		t.Fatal("first end did not finish playback")
	}
	output.mu.Lock()
	output.playing, output.running, output.pos, output.at = true, true, 0, time.Now()
	output.mu.Unlock()
	output.event("ended", 250*time.Second, "media ended", 1)
	if calls != 1 || !output.Playing() {
		t.Fatal("duplicate end interrupted the repeat")
	}
	output.event("ended", 250*time.Second, "media ended", 2)
	if calls != 2 || output.Playing() {
		t.Fatal("next repeat did not finish")
	}
}
