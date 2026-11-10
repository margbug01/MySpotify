package player

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"os"
	"os/exec"
	"time"

	"vinyl/internal/l10n"
	"vinyl/internal/qtweb"
)

func startLinuxBrowser(w *Web) (bool, error) {
	if qtweb.Enabled() {
		return startQtBrowser(w)
	}
	if os.Getenv("MYSPOTIFY_LINUX_CHROME") != "1" {
		return false, nil
	}
	binary, err := exec.LookPath("google-chrome-stable")
	if err != nil {
		return false, err
	}
	profile, err := os.MkdirTemp("", "myspotify-audio-")
	if err != nil {
		return false, err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		os.RemoveAll(profile)
		return false, err
	}
	controlKey := hex.EncodeToString(key)
	w.mu.Lock()
	w.browserKey = controlKey
	w.mu.Unlock()
	command := exec.CommandContext(w.ctx, binary,
		"--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check",
		"--disable-extensions", "--disable-component-extensions-with-background-pages", "--disable-background-networking",
		"--autoplay-policy=no-user-gesture-required", "--user-data-dir="+profile,
		// An audio-only app needs no browser chrome or component extensions.
		"--app=http://"+w.addr+"/?ks="+keySystem+"#control="+controlKey)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		os.RemoveAll(profile)
		return false, err
	}
	exited := make(chan struct{})
	go func() {
		err := command.Wait()
		os.RemoveAll(profile)
		close(exited)
		if w.ctx.Err() == nil {
			log.Printf("player: experimental Chrome audio backend exited: %v", err)
			w.browserFailed()
		}
	}()
	w.stopBrowser = func() {
		w.cancel()
		<-exited
	}
	return true, nil
}

// The helper starts off the UI thread and is shared with account capture.
func startQtBrowser(w *Web) (bool, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return false, err
	}
	controlKey := hex.EncodeToString(key)
	w.mu.Lock()
	w.browserKey = controlKey
	w.mu.Unlock()
	w.stopBrowser = w.cancel
	go func() {
		client, release, err := qtweb.Acquire()
		if err != nil {
			if w.ctx.Err() == nil {
				w.backendFailed(l10n.T("backendError Qt WebEngine 启动失败，请检查音频组件和显示服务"))
			}
			return
		}
		defer release()
		unwatch := client.OnExit(func() {
			if w.ctx.Err() == nil {
				w.backendFailed("backendError Qt WebEngine audio backend exited")
			}
		})
		defer unwatch()
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			client.Call(ctx, "audio.close", map[string]any{"id": controlKey})
		}()
		if w.ctx.Err() != nil {
			return
		}
		ctx, cancel := context.WithTimeout(w.ctx, 10*time.Second)
		_, err = client.Call(ctx, "audio.open", map[string]any{"id": controlKey, "url": "http://" + w.addr + "/?ks=" + keySystem + "#control=" + controlKey})
		cancel()
		if err != nil && w.ctx.Err() == nil {
			w.backendFailed(l10n.T("backendError Qt WebEngine 无法打开音频页"))
			return
		}
		<-w.ctx.Done()
	}()
	return true, nil
}
