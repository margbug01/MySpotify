package qtweb

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

// Active pages keep the host alive; idle cleanup cannot close a later process.
func TestSharedLeaseKeepsActiveHostAndReclaimsIdleHost(t *testing.T) {
	Shutdown()
	previousFactory := makeHostCommand
	makeHostCommand = func() (*exec.Cmd, error) {
		command := exec.Command(os.Args[0], "-test.run=^TestQtWebHelper$")
		command.Env = append(os.Environ(), "MYSPOTIFY_QTWEB_TEST_CHILD=1")
		return command, nil
	}
	t.Cleanup(func() { Shutdown(); makeHostCommand = previousFactory })
	first, releaseFirst, err := Acquire()
	if err != nil {
		t.Fatal(err)
	}
	second, releaseSecond, err := Acquire()
	if err != nil || second != first {
		t.Fatal("owners did not share a host")
	}
	releaseFirst()
	reclaimIdle(first)
	if !first.alive() {
		t.Fatal("idle cleanup closed an actively owned host")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := second.Call(ctx, "echo", true); err != nil {
		t.Fatal("remaining owner lost its host")
	}
	releaseSecond()
	reclaimIdle(first)
	if first.alive() {
		t.Fatal("idle host was not reclaimed")
	}
	releaseFirst() // stale/double release must not affect the next process.
	third, releaseThird, err := Acquire()
	if err != nil || third == first {
		t.Fatal("idle close prevented a fresh host")
	}
	defer releaseThird()
	reclaimIdle(first)
	if _, err := third.Call(ctx, "echo", true); err != nil {
		t.Fatal("new host did not answer")
	}
}
