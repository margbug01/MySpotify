package spotify

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func withTransferTestEnv(t *testing.T, retries int, base, capDelay time.Duration, fn func(*http.Request) (*http.Response, error)) {
	t.Helper()
	oldClient := Client
	oldRetries, oldBase, oldMax := transferMaxRetries, transferBackoffBase, transferBackoffMax
	SetIdentity(Identity{Token: "t", ClientToken: "c", Expires: time.Now().Add(time.Hour)})
	transferMaxRetries = retries
	transferBackoffBase = base
	transferBackoffMax = capDelay
	Client = &http.Client{Transport: lyricsTransport(fn)}
	t.Cleanup(func() {
		Client = oldClient
		transferMaxRetries, transferBackoffBase, transferBackoffMax = oldRetries, oldBase, oldMax
		SetIdentity(Identity{})
	})
}

// Transfer retries temporary 409 state conflicts with backoff and succeeds once Spotify accepts the transfer.
func TestConnectTransferRetries409ThenSucceeds(t *testing.T) {
	var attempts atomic.Int32
	withTransferTestEnv(t, 5, time.Millisecond, 4*time.Millisecond, func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost || req.URL.Path != "/connect-state/v1/connect/transfer/from/me-observer/to/speaker-1" {
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
		n := attempts.Add(1)
		if n <= 2 {
			return &http.Response{
				StatusCode: http.StatusConflict,
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader(`{"error_type":"STATE_CONFLICT","message":"version mismatch"}`)),
			}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	})

	if err := NewConnect("me", "MySpotify", nil).Transfer("speaker-1"); err != nil {
		t.Fatalf("expected nil error after transient 409s, got %v", err)
	}
	if got := attempts.Load(); got != 3 {
		t.Fatalf("expected 3 attempts, got %d", got)
	}
}

// Transfer stops after the configured 409 retry limit and returns the final 409 ConnectError.
func TestConnectTransferExhausts409Retries(t *testing.T) {
	var attempts atomic.Int32
	withTransferTestEnv(t, 3, time.Millisecond, 2*time.Millisecond, func(*http.Request) (*http.Response, error) {
		n := attempts.Add(1)
		return &http.Response{
			StatusCode: http.StatusConflict,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader(fmt.Sprintf(`{"error_type":"CONFLICT","message":"attempt %d"}`, n))),
		}, nil
	})

	err := NewConnect("me", "MySpotify", nil).Transfer("speaker-1")
	var ce *ConnectError
	if !errors.As(err, &ce) || ce.Status != http.StatusConflict || ce.Type != "CONFLICT" || ce.Message != "attempt 4" {
		t.Fatalf("expected final 409 ConnectError, got %#v (%v)", ce, err)
	}
	if got := attempts.Load(); got != 4 {
		t.Fatalf("expected 4 attempts (1 initial + 3 retries), got %d", got)
	}
}

// Transfer returns non-409 HTTP and transport errors on the first attempt without entering backoff.
func TestConnectTransferFailsImmediatelyOnNon409(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError} {
		var attempts atomic.Int32
		withTransferTestEnv(t, 5, 5*time.Second, 5*time.Second, func(*http.Request) (*http.Response, error) {
			attempts.Add(1)
			return &http.Response{
				StatusCode: status,
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader(`{"error_type":"ERROR","message":"nope"}`)),
			}, nil
		})

		err := NewConnect("me", "MySpotify", nil).Transfer("speaker-1")
		var ce *ConnectError
		if !errors.As(err, &ce) || ce.Status != status {
			t.Fatalf("status %d: expected ConnectError, got %v", status, err)
		}
		if got := attempts.Load(); got != 1 {
			t.Fatalf("status %d: expected 1 attempt without retry, got %d", status, got)
		}
	}

	var attempts atomic.Int32
	withTransferTestEnv(t, 5, 5*time.Second, 5*time.Second, func(*http.Request) (*http.Response, error) {
		attempts.Add(1)
		return nil, errors.New("dial failed")
	})
	if err := NewConnect("me", "MySpotify", nil).Transfer("speaker-1"); err == nil {
		t.Fatal("expected transport error, got nil")
	}
	if got := attempts.Load(); got != 1 {
		t.Fatalf("expected 1 attempt on transport error, got %d", got)
	}
}

// Transfer stops waiting and returns immediately when the Connect context is canceled during 409 backoff.
func TestConnectTransferCancelsDuringBackoff(t *testing.T) {
	var attempts atomic.Int32
	var once sync.Once
	inBackoff := make(chan struct{})
	withTransferTestEnv(t, 5, 5*time.Second, 5*time.Second, func(*http.Request) (*http.Response, error) {
		attempts.Add(1)
		once.Do(func() { close(inBackoff) })
		return &http.Response{
			StatusCode: http.StatusConflict,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader(`{"error_type":"STATE_CONFLICT","message":"busy"}`)),
		}, nil
	})

	client := NewConnect("me", "MySpotify", nil)
	done := make(chan error, 1)
	go func() {
		done <- client.Transfer("speaker-1")
	}()

	<-inBackoff
	client.Close()

	timer := time.NewTimer(500 * time.Millisecond)
	defer timer.Stop()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-timer.C:
		t.Fatal("Transfer did not return promptly after context cancellation")
	}
	if got := attempts.Load(); got != 1 {
		t.Fatalf("expected 1 attempt before cancel, got %d", got)
	}
}
