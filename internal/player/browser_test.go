package player

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"vinyl/internal/spotify"
)

// Chrome exiting before a track or its callback existed used to leave playback waiting forever.
func TestBrowserExitReachesLateTracks(t *testing.T) {
	for _, phase := range []string{"before-open", "before-callback", "playing"} {
		t.Run(phase, func(t *testing.T) {
			web, err := listen()
			if err != nil {
				t.Fatal(err)
			}
			defer web.Close()
			if phase == "before-open" {
				web.browserFailed()
			}
			out := web.Open(&spotify.Media{Duration: time.Minute})
			if phase == "before-callback" {
				web.browserFailed()
			}
			ends := 0
			out.OnEnd(func() { ends++ })
			out.Play()
			if phase == "playing" {
				web.browserFailed()
			}
			msg, _, _ := out.(*webOut).Fault()
			if ends != 1 || out.Playing() || msg == "" {
				t.Fatalf("ends=%d playing=%v fault=%q", ends, out.Playing(), msg)
			}
		})
	}
}

// Chrome control must reject other pages without consuming queued commands.
func TestBrowserControlAuthorization(t *testing.T) {
	web := &Web{addr: "127.0.0.1:12345", browserKey: "secret", cmds: make(chan string, 2), vol: make(chan float64, 1), done: make(chan struct{})}
	web.cmds <- "vinyl.play(1)"
	for _, key := range []string{"", "wrong"} {
		request := httptest.NewRequest(http.MethodGet, "http://"+web.addr+"/commands", nil)
		request.Header.Set("X-Vinyl-Key", key)
		response := httptest.NewRecorder()
		web.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden || len(web.cmds) != 1 {
			t.Fatalf("unauthorized control consumed command: status=%d queued=%d", response.Code, len(web.cmds))
		}
	}
	request := httptest.NewRequest(http.MethodGet, "http://"+web.addr+"/commands", nil)
	request.Header.Set("X-Vinyl-Key", "secret")
	response := httptest.NewRecorder()
	web.ServeHTTP(response, request)
	var command string
	if err := json.Unmarshal(response.Body.Bytes(), &command); err != nil || command != "vinyl.play(1)" {
		t.Fatalf("command=%q error=%v", command, err)
	}
	web.vol <- .5
	response = httptest.NewRecorder()
	web.ServeHTTP(response, request)
	if err := json.Unmarshal(response.Body.Bytes(), &command); err != nil || command != "vinyl.volume(0.5000)" {
		t.Fatalf("volume command=%q error=%v", command, err)
	}
	close(web.done)
	response = httptest.NewRecorder()
	web.ServeHTTP(response, request)
	if response.Code != http.StatusGone {
		t.Fatalf("closed control status=%d", response.Code)
	}
}
