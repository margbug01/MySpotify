package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/egoist/mygo"
)

// runWebToken signs in the web player (dev probe) and writes its identity to path,
// so the data endpoints can be tried from the command line for the next hour.
func runWebToken(path string) error {
	var failed error
	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title: "Vinyl · 登录", Width: 960, Height: 720,
			URL:  "https://open.spotify.com/",
			Page: mygo.PageOptions{PreloadScript: captureJS},
		})
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
			defer cancel()
			web, err := waitWebIdentity(ctx, win.Page(), func() {
				fmt.Println("web player is signed out: log in in the Vinyl window")
			})
			if err == nil {
				b, _ := json.Marshal(web)
				err = os.WriteFile(path, b, 0o600)
			}
			failed = err
			mygo.RunOnMain(mygo.App.Quit)
		}()
	})
	if err := mygo.App.Run(); err != nil {
		return err
	}
	return failed
}
