package qtweb

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"
)

// This child is a protocol peer, not a WebEngine or Spotify substitute.
func TestQtWebHelper(t *testing.T) {
	if os.Getenv("MYSPOTIFY_QTWEB_TEST_CHILD") != "1" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4096), 2<<20)
	var held []reply
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var request struct {
			ID     int             `json:"id"`
			Method string          `json:"method"`
			Args   json.RawMessage `json:"args"`
		}
		if json.Unmarshal(scanner.Bytes(), &request) != nil {
			os.Exit(2)
		}
		r := reply{ID: request.ID, Value: request.Args}
		switch request.Method {
		case "crash":
			os.Exit(23)
		case "hold":
			held = append(held, r)
			continue
		case "release":
			for _, late := range held {
				encoder.Encode(late)
			}
			held = nil
		}
		encoder.Encode(r)
	}
	os.Exit(0)
}

func protocolClient(t *testing.T) *Client {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run=^TestQtWebHelper$")
	command.Env = append(os.Environ(), "MYSPOTIFY_QTWEB_TEST_CHILD=1")
	client, err := start(command)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	return client
}

// Concurrent callers receive only their own replies.
func TestConcurrentRequestsKeepTheirReplies(t *testing.T) {
	client := protocolClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var group sync.WaitGroup
	for i := range 32 {
		group.Add(1)
		go func() {
			defer group.Done()
			value, err := client.Call(ctx, "echo", i)
			var got int
			if err != nil || json.Unmarshal(value, &got) != nil || got != i {
				t.Errorf("request %d got %s, err %v", i, value, err)
			}
		}()
	}
	group.Wait()
}

// A late reply to a canceled request cannot satisfy a later request.
func TestCancelledRequestDoesNotStealLaterReply(t *testing.T) {
	client := protocolClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := client.Call(ctx, "hold", 11); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("held request: %v", err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	value, err := client.Call(ctx, "release", 22)
	if err != nil || string(value) != "22" {
		t.Fatalf("late response corrupted new request: %s, %v", value, err)
	}
	value, err = client.Call(ctx, "echo", 33)
	if err != nil || string(value) != "33" {
		t.Fatalf("channel did not survive cancellation: %s, %v", value, err)
	}
}

// Process exit wakes pending callers and both existing and late subscribers.
func TestExitWakesRequestsAndLateSubscribers(t *testing.T) {
	client := protocolClient(t)
	exited := make(chan struct{}, 2)
	client.OnExit(func() { exited <- struct{}{} })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := client.Call(ctx, "crash", nil); err == nil {
		t.Fatal("crashed peer returned a successful reply")
	}
	select {
	case <-exited:
	case <-ctx.Done():
		t.Fatal("exit subscriber was not notified")
	}
	client.OnExit(func() { exited <- struct{}{} })
	select {
	case <-exited:
	case <-ctx.Done():
		t.Fatal("late exit subscriber was not notified")
	}
	if client.alive() {
		t.Fatal("crashed process still appeared alive")
	}
}
