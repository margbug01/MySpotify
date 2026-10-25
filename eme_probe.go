package main

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"time"

	librespot "github.com/devgianlu/go-librespot"
	"github.com/egoist/mygo"
)

//go:embed web/capture.js
var captureJS string

// runEME plays one track through the WebView's own Widevine module (dev probe): the
// window first loads open.spotify.com to pick up the web identity, then our player page.
func runEME(uri string) error {
	// WebView2 reads this when it starts; without it audio waits for a click.
	os.Setenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", "--autoplay-policy=no-user-gesture-required")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	s, err := Connect(ctx, nil)
	if err != nil {
		return err
	}
	defer s.Close()
	id, err := librespot.SpotifyIdFromUri(uri)
	if err != nil {
		return err
	}
	m, err := s.ResolveMedia(ctx, *id)
	if err != nil {
		return err
	}
	ps, err := newPlayerServer(s, m)
	if err != nil {
		return err
	}
	fmt.Printf("media: %s, %d kbps, %d source(s); player at %s\n", m.Name, m.File.Bitrate/1000, len(ps.sources), ps.URL())

	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title: "Vinyl · 播放测试", Width: 960, Height: 720,
			URL:  "https://open.spotify.com/",
			Page: mygo.PageOptions{PreloadScript: captureJS},
		})
		go func() {
			wctx, wcancel := context.WithTimeout(context.Background(), 20*time.Minute)
			defer wcancel()
			t0 := time.Now()
			web, err := waitWebIdentity(wctx, win.Page(), func() {
				fmt.Println("web player is signed out: log in in the Vinyl window")
			})
			if err != nil {
				fmt.Println("FAIL: web identity:", err)
				mygo.App.Quit()
				return
			}
			src := "token endpoint"
			if web.accessToken() == "" {
				src = "request header"
			}
			ct := "captured"
			if web.ClientToken == "" {
				ct = "desktop fallback"
			}
			fmt.Printf("web identity from %s, client token %s (%v)\n", src, ct, time.Since(t0).Round(time.Millisecond))
			ps.setWeb(web)
			win.Page().LoadURL(ps.URL())
		}()
	})
	return mygo.App.Run()
}
