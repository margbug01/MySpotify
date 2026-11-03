package spotify

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
)

// custom is the proxy the listener set, nil to go by the system's.
var custom atomic.Pointer[url.URL]

// SetProxy has the requests from now on go through u; nil goes back to the
// system's proxy.
func SetProxy(u *url.URL) { custom.Store(u) }

// Proxy is the proxy Client's requests take: the listener's, else the
// system's. The app's requests that go through another client take it too:
// MyGo's updater asks GitHub with http.DefaultClient.
func Proxy(req *http.Request) (*url.URL, error) {
	if h := req.URL.Hostname(); h == "localhost" || h == "127.0.0.1" {
		return nil, nil
	}
	if u := custom.Load(); u != nil {
		return u, nil
	}
	return systemProxy(req)
}

// ErrBadProxy is a proxy ParseProxy cannot read.
var ErrBadProxy = errors.New("proxy: want http://, https:// or socks5:// with a host and a port")

// ParseProxy reads a proxy as the listener writes it: a URL of http, https
// or socks5, or a bare host:port, an HTTP proxy. Empty is none.
func ParseProxy(s string) (*url.URL, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Hostname() == "" || u.Port() == "" || u.Path != "" && u.Path != "/" {
		return nil, ErrBadProxy
	}
	switch u.Scheme = strings.ToLower(u.Scheme); u.Scheme {
	case "http", "https", "socks5", "socks5h":
		u.Path = ""
		return u, nil
	}
	return nil, ErrBadProxy
}
