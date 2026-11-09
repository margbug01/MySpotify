package player

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"vinyl/internal/spotify"
)

// The real Chrome backend must decode local audio and initialize Widevine without a Spotify licence.
func TestLinuxBrowserLocalAudio(t *testing.T) {
	if os.Getenv("LIVE_LINUX_CHROME") != "1" {
		t.Skip("set LIVE_LINUX_CHROME=1 with official Chrome installed")
	}
	t.Setenv("MYSPOTIFY_LINUX_CHROME", "1")
	const rate, seconds = 22050, 60
	data := make([]byte, 44+rate*seconds*2)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 1)
	binary.LittleEndian.PutUint32(data[24:], rate)
	binary.LittleEndian.PutUint32(data[28:], rate*2)
	binary.LittleEndian.PutUint16(data[32:], 2)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], uint32(len(data)-44))
	for i := range rate * seconds {
		binary.LittleEndian.PutUint16(data[44+i*2:], uint16(int16(math.Sin(2*math.Pi*440*float64(i)/rate)*1000)))
	}
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/wav")
		http.ServeContent(w, r, "tone.wav", time.Time{}, bytes.NewReader(data))
	}))
	defer source.Close()
	web, err := listen()
	if err != nil {
		t.Fatal(err)
	}
	defer web.Close()
	if started, err := startLinuxBrowser(web); err != nil || !started {
		t.Fatalf("started=%v err=%v", started, err)
	}
	select {
	case <-web.ready:
	case <-time.After(15 * time.Second):
		t.Fatal("Chrome did not load the audio page")
	}
	out := web.Open(&spotify.Media{File: source.URL, Duration: seconds * time.Second}).(*webOut)
	defer out.Close()
	keys := make(chan struct{}, 1)
	out.OnSystem(func(command string) {
		if command == "next" {
			keys <- struct{}{}
		}
	})
	out.SetVolume(0)
	out.Play()
	// A test marker uses the existing page event channel; no media-key session or licence is requested.
	web.send(fmt.Sprintf(`navigator.requestMediaKeySystemAccess('com.widevine.alpha', [{initDataTypes:['cenc'], audioCapabilities:[{contentType:'audio/mp4; codecs="mp4a.40.2"'}], sessionTypes:['temporary']}]).then(a => a.createMediaKeys()).then(k => { window.probeKeys=k; return fetch('/event', {method:'POST',body:JSON.stringify({gen:%d,type:'system-next'})}); }).catch(e => console.error('probe EME failed', e.name))`, out.gen))
	select {
	case <-keys:
	case <-time.After(15 * time.Second):
		t.Fatal("Widevine did not initialize")
	}
	for i := range 10 {
		time.Sleep(time.Second)
		out.mu.Lock()
		running := out.running
		out.mu.Unlock()
		if msg, _, _ := out.Fault(); msg != "" {
			t.Fatal(msg)
		}
		if i >= 2 && (!running || out.Position() < time.Second) {
			t.Fatal("local audio did not advance")
		}
		t.Logf("MEMORY_PROBE playing=%v position=%v", running, out.Position().Round(time.Millisecond))
	}
}
