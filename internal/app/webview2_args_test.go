package app

import "testing"

// An existing WebView2 disable-features list used to be overwritten by MySpotify's SMTC switches.
func TestWebView2BrowserArgumentsMergesDisabledFeatures(t *testing.T) {
	got := webView2BrowserArguments(`--user-data-dir="C:\Profiles\My Spotify" --disable-features=ExistingFeature,HardwareMediaKeyHandling --disable-features="AnotherFeature,MediaSessionService"`)
	want := `--user-data-dir="C:\Profiles\My Spotify" --autoplay-policy=no-user-gesture-required --in-process-gpu --disable-gpu --disable-features=ExistingFeature,HardwareMediaKeyHandling,AnotherFeature,MediaSessionService`
	if got != want {
		t.Fatalf("webView2BrowserArguments() = %q, want %q", got, want)
	}
}
