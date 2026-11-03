package app

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/egoist/mygo/ui"

	"vinyl/internal/spotify"
)

// A proxy typed in the settings takes the requests, and the web pages, as
// Enter is pressed, written in full; one that cannot be read says so and
// changes nothing; emptied and left, it is the system's again.
func TestProxySetting(t *testing.T) {
	var web []*url.URL
	was := webProxy
	webProxy = func(u *url.URL) { web = append(web, u) }
	defer func() { webProxy = was; spotify.SetProxy(nil) }()
	req, _ := http.NewRequest(http.MethodGet, "https://api-partner.spotify.com/pathfinder/v2/query", nil)
	through := func() string {
		if u, _ := spotify.Proxy(req); u != nil {
			return u.String()
		}
		return ""
	}

	a := newTestApp()
	tt := settingsTester(a)
	click(t, tt, "代理地址")
	tt.Type("127.0.0.1:7890")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if a.settings.Proxy != "http://127.0.0.1:7890" || a.proxyText != a.settings.Proxy || through() != a.settings.Proxy || len(web) != 1 || web[0].String() != a.settings.Proxy {
		t.Fatalf("set %q, shown %q, requests through %q, web pages given %v", a.settings.Proxy, a.proxyText, through(), web)
	}

	tt.Command("selectAll")
	tt.Type("socks5://proxy.lan")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if a.proxyErr == "" || a.settings.Proxy != "http://127.0.0.1:7890" || through() != a.settings.Proxy {
		t.Fatalf("no port: told %q, set %q, requests through %q", a.proxyErr, a.settings.Proxy, through())
	}

	tt.Command("selectAll")
	tt.Key(0, ui.KeyBackspace)
	tt.Key(0, ui.KeyTab)
	tt.Frame()
	if a.settings.Proxy != "" || a.proxyErr != "" || web[len(web)-1] != nil || through() == "http://127.0.0.1:7890" {
		t.Fatalf("emptied: set %q, told %q, requests through %q", a.settings.Proxy, a.proxyErr, through())
	}
}
