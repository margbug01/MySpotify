package qtweb

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	imagepng "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Real WebEngine, private temporary profile, no Spotify licence or account required.
func TestQtWebProfileAndWindow(t *testing.T) {
	if os.Getenv("LIVE_LINUX_QTWEB") != "1" {
		t.Skip("set LIVE_LINUX_QTWEB=1 with the Qt runtime installed")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var diagnostics bytes.Buffer
	previousFactory := makeHostCommand
	makeHostCommand = func() (*exec.Cmd, error) {
		command, err := hostCommand()
		if err == nil {
			command.Stderr = &diagnostics
		}
		return command, err
	}
	t.Cleanup(func() {
		makeHostCommand = previousFactory
		if t.Failed() {
			for _, line := range strings.Split(diagnostics.String(), "\n") {
				lower := strings.ToLower(line)
				if strings.Contains(lower, "cookie") || strings.Contains(lower, "crypt") {
					t.Log(line)
				}
			}
		}
	})
	defer Shutdown()
	client, release, err := Acquire()
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	call := func(method string, args any) json.RawMessage {
		t.Helper()
		result, err := client.Call(ctx, method, args)
		if err != nil {
			if method == "cookies.import" {
				metadata, _ := client.Call(ctx, "cookies.metadata", nil)
				t.Logf("fixture cookie metadata (no values): %s", metadata)
			}
			t.Fatalf("%s: %v", method, err)
		}
		return result
	}
	expires := time.Now().Add(time.Hour).Unix()
	call("cookies.import", map[string]any{"cookies": []map[string]any{
		{"name": "fixture_root", "value": "test", "domain": ".spotify.com", "path": "/", "secure": true, "httpOnly": true, "sameSite": "lax", "expires": expires},
		{"name": "fixture_host", "value": "test", "domain": "open.spotify.com", "path": "/account", "secure": true, "httpOnly": true, "sameSite": "strict", "expires": expires},
		{"name": "fixture_none", "value": "test", "domain": ".spotify.com", "path": "/account", "secure": true, "sameSite": "none", "expires": expires},
	}})
	var metadata []struct {
		Name, Domain, Path, SameSite string
		Secure, HTTPOnly             bool
	}
	if err := json.Unmarshal(call("cookies.metadata", nil), &metadata); err != nil {
		t.Fatal(err)
	}
	for _, cookie := range metadata {
		want := map[string]string{"fixture_root": "Lax", "fixture_host": "Strict", "fixture_none": "None_"}[cookie.Name]
		if want != "" && (!cookie.Secure || cookie.Name != "fixture_none" && !cookie.HTTPOnly) {
			t.Fatalf("fixture cookie security attributes changed (no values): %+v", cookie)
		}
	}
	cookiesAt := func(url string) string {
		t.Helper()
		var value string
		if json.Unmarshal(call("cookies", map[string]any{"url": url}), &value) != nil {
			t.Fatal("invalid cookie response")
		}
		return value
	}
	if !strings.Contains(cookiesAt("https://open.spotify.com/account/settings"), "fixture_host=") {
		t.Fatal("host cookie missing")
	}
	for _, url := range []string{"https://accounts.spotify.com/account", "https://sub.open.spotify.com/account", "https://open.spotify.com/accounting"} {
		if strings.Contains(cookiesAt(url), "fixture_host=") {
			t.Fatal("host or path scope widened")
		}
	}
	if cookiesAt("http://open.spotify.com/account") != "" || cookiesAt("https://evilspotify.com/") != "" {
		t.Fatal("secure/domain filtering failed")
	}
	call("audio.open", map[string]any{"id": "fixture-audio", "url": "http://127.0.0.1:1/"})
	call("auth.open", map[string]any{"id": 1, "url": "about:blank", "show": true, "preload": "window.fixtureReady = true"})
	for {
		if string(call("auth.eval", map[string]any{"id": 1, "script": "window.fixtureReady === true"})) == "true" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	// The current host and its real Qt window render this isolated login fixture.
	call("auth.eval", map[string]any{"id": 1, "script": `document.open();document.write('<html><body style="margin:0;background:#121212;color:white;font:24px sans-serif"><div style="padding:40px"><h1>MySpotify</h1><p>Sign in to Spotify</p><input placeholder="Email" style="padding:16px;width:280px"><p><button style="padding:18px;background:#1ed760;border:0;border-radius:30px">Continue</button></p></div></body></html>');document.close();true`})
	// Save a native frame for manual inspection; wording and pixels are not assertions.
	time.Sleep(400 * time.Millisecond)
	var encoded string
	if json.Unmarshal(call("auth.snapshot", map[string]any{"id": 1}), &encoded) != nil {
		t.Fatal("invalid snapshot")
	}
	png, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(png) < 1000 {
		t.Fatal("login view did not render")
	}
	if _, err := imagepng.DecodeConfig(bytes.NewReader(png)); err != nil {
		t.Fatal("invalid PNG snapshot")
	}
	if dir := os.Getenv("QTWEB_TEST_SHOTS"); dir != "" {
		if err := os.WriteFile(filepath.Join(dir, "qt-login.png"), png, 0600); err != nil {
			t.Fatal(err)
		}
	}
	call("auth.close", map[string]any{"id": 1})
	if _, err := client.Call(ctx, "auth.eval", map[string]any{"id": 1, "script": "true"}); err == nil {
		t.Fatal("closed auth still accepted evaluation")
	}
	current, releaseCheck, err := Acquire()
	if err != nil {
		t.Fatal(err)
	}
	releaseCheck()
	if current != client {
		t.Fatal("closing login restarted shared audio host")
	}
	call("audio.close", map[string]any{"id": "fixture-audio"})
	Shutdown()
	// Qt 6.11's cookieAdded conversion omits SameSite; check Chromium's
	// persisted metadata instead. No cookie values are queried or printed.
	profile, err := ProfileDir()
	if err != nil {
		t.Fatal(err)
	}
	query := `import json,sqlite3,sys
c=sqlite3.connect('file:'+sys.argv[1]+'?mode=ro',uri=True)
print(json.dumps(dict(c.execute("SELECT name,samesite FROM cookies WHERE name IN ('fixture_root','fixture_host','fixture_none')"))))`
	stored, err := exec.CommandContext(ctx, "python3", "-c", query, filepath.Join(profile, "Cookies")).Output()
	if err != nil {
		t.Fatalf("read fixture cookie metadata: %v", err)
	}
	var policies map[string]int
	if json.Unmarshal(stored, &policies) != nil || len(policies) != 3 || policies["fixture_root"] != 1 || policies["fixture_host"] != 2 || policies["fixture_none"] != 0 {
		t.Fatal("persisted SameSite attributes changed")
	}
	var releaseRestored func()
	client, releaseRestored, err = Acquire()
	if err != nil {
		t.Fatal(err)
	}
	defer releaseRestored()
	// Cookie-change signals need not enumerate an already loaded cold store.
	// The browser-token path verifies reuse; logout must still delete disk data.
	call("cookies.clear", nil)
	if cookiesAt("https://open.spotify.com/account") != "" {
		t.Fatal("logout retained cookies")
	}
	Shutdown()
	stored, err = exec.CommandContext(ctx, "python3", "-c", query, filepath.Join(profile, "Cookies")).Output()
	if err != nil {
		t.Fatal("could not verify logout persistence")
	}
	policies = nil
	if json.Unmarshal(stored, &policies) != nil || len(policies) != 0 {
		t.Fatal("logout retained persisted cookies after restart")
	}
}
