package player

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"vinyl/internal/spotify"
)

// An exiting Chrome process must notify playback and remove its temporary profile.
func TestLinuxBrowserProcessExit(t *testing.T) {
	for _, crash := range []bool{false, true} {
		name := "close"
		if crash {
			name = "crash"
		}
		t.Run(name, func(t *testing.T) {
			bin, profiles := t.TempDir(), t.TempDir()
			t.Setenv("PATH", bin)
			t.Setenv("TMPDIR", profiles)
			t.Setenv("MYSPOTIFY_LINUX_CHROME", "1")
			script := "#!/bin/sh\nexec /bin/sleep 60\n"
			if crash {
				script = "#!/bin/sh\nexit 23\n"
			}
			if err := os.WriteFile(filepath.Join(bin, "google-chrome-stable"), []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			web, err := listen()
			if err != nil {
				t.Fatal(err)
			}
			defer web.Close()
			out := web.Open(&spotify.Media{Duration: time.Minute})
			failed := make(chan struct{}, 1)
			out.OnEnd(func() { failed <- struct{}{} })
			started, err := startLinuxBrowser(web)
			if err != nil || !started {
				t.Fatalf("started=%v err=%v", started, err)
			}
			if crash {
				select {
				case <-failed:
				case <-time.After(5 * time.Second):
					t.Fatal("browser exit did not reach playback")
				}
				if msg, _, _ := out.(*webOut).Fault(); msg == "" {
					t.Fatal("browser exit lost its playback fault")
				}
			}
			web.Close()
			entries, err := os.ReadDir(profiles)
			if err != nil || len(entries) != 0 {
				t.Fatalf("temporary profiles remain: %v, err=%v", entries, err)
			}
			if !crash && len(failed) != 0 {
				t.Fatal("normal shutdown reported a playback fault")
			}
		})
	}
}
