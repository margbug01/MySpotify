package spotify

import (
	"net/http"
	"testing"
)

// ParseProxy takes a proxy as a listener would write it, and turns down one
// no request could go through.
func TestParseProxy(t *testing.T) {
	for in, want := range map[string]string{
		"":                          "",
		" 127.0.0.1:7890 ":          "http://127.0.0.1:7890",
		"SOCKS5://127.0.0.1:1080/":  "socks5://127.0.0.1:1080",
		"http://u:p@proxy.lan:3128": "http://u:p@proxy.lan:3128",
		"https://[::1]:8443":        "https://[::1]:8443",
		"socks5h://proxy.lan:1080":  "socks5h://proxy.lan:1080",
	} {
		u, err := ParseProxy(in)
		got := ""
		if u != nil {
			got = u.String()
		}
		if err != nil || got != want {
			t.Errorf("ParseProxy(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"127.0.0.1", "proxy.lan", ":7890", "ftp://proxy.lan:21", "http://proxy.lan:80/pac", "socks5://", "http://proxy.lan:port"} {
		if u, err := ParseProxy(in); err == nil {
			t.Errorf("ParseProxy(%q) = %v, want an error", in, u)
		}
	}
}

// The proxy the listener set wins over the system's, though not for the
// app's own pages on 127.0.0.1; cleared, the system's is back.
func TestProxySet(t *testing.T) {
	defer SetProxy(nil)
	set, _ := ParseProxy("socks5://10.9.8.7:1080")
	SetProxy(set)
	req, _ := http.NewRequest(http.MethodGet, "https://api-partner.spotify.com/x", nil)
	if u, _ := Proxy(req); u != set {
		t.Fatalf("with a proxy set, requests go through %v", u)
	}
	local, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:4321/m/1", nil)
	if u, _ := Proxy(local); u != nil {
		t.Fatalf("the player's page on 127.0.0.1 goes through %v", u)
	}
	SetProxy(nil)
	if u, _ := Proxy(req); u != nil && u.Host == set.Host {
		t.Fatal("cleared, requests still go through the proxy that was set")
	}
}
