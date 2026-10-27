package player

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Long pauses release the page without losing the queue or resume position.
func TestIdleReleaseAndResume(t *testing.T) {
	ch, p, _ := licensed(t, func(http.ResponseWriter, *http.Request) { t.Error("unexpected licence request") })
	defer p.CloseWeb()
	old := p.Web
	created := 0
	p.NewWeb = func() (*Web, error) {
		created++
		return &Web{cmds: make(chan string, 256), vol: make(chan float64, 1), outs: map[int]*webOut{}}, nil
	}
	p.out.Seek(23 * time.Second)
	p.out.Pause()
	p.playing = false
	p.sync()
	generation := p.idleGen
	p.releaseIdle(generation)
	if p.Web != nil || p.Ready() || p.Position() != 23*time.Second || old.out(1) != nil {
		t.Fatal("idle player kept resources or lost its position")
	}
	if created != 0 {
		t.Fatal("created a page while idle")
	}
	p.TogglePlay()
	for p.Loading() {
		next(t, ch)
	}
	if created != 1 || !p.Playing() || !p.Ready() || p.Position() != 23*time.Second {
		t.Fatal("resume did not lazily rebuild at the saved position")
	}
	current := p.Web
	p.releaseIdle(generation)
	if p.Web != current {
		t.Fatal("stale timeout closed the new player")
	}
}

// Resuming invalidates a pause timeout even when it has already been posted.
func TestIdleTimeoutCancelled(t *testing.T) {
	_, p, _ := licensed(t, func(http.ResponseWriter, *http.Request) {})
	defer p.CloseWeb()
	p.playing = false
	p.updateIdle()
	generation := p.idleGen
	p.playing = true
	p.updateIdle()
	p.playing = false
	p.updateIdle()
	p.releaseIdle(generation)
	if p.Web == nil {
		t.Fatal("old timeout closed a newer pause")
	}
}

// Closing before page readiness stops the command loop and HTTP listener.
func TestWebCloseBeforeReady(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	web := &Web{ready: make(chan struct{}), done: make(chan struct{}), ctx: ctx, cancel: cancel, outs: map[int]*webOut{}}
	server := httptest.NewUnstartedServer(web)
	server.Start()
	defer server.Close()
	web.server = server.Config
	exited := make(chan struct{})
	go func() { web.run(); close(exited) }()
	web.Close()
	web.Close()
	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatal("command loop remained alive")
	}
	if ctx.Err() == nil {
		t.Fatal("in-flight work was not cancelled")
	}
	response, err := server.Client().Get(server.URL)
	if err == nil {
		response.Body.Close()
		t.Fatal("HTTP listener remained alive")
	}
}
