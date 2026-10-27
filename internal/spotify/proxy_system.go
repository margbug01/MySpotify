//go:build windows || darwin

package spotify

import (
	"net/http"
	"net/url"
	"sync"
	"time"
)

// systemProxy routes requests through the proxy the system is set to, read
// again every few seconds, so switching it needs no restart. Go reads only
// the environment, which wins here too.
func systemProxy(req *http.Request) (*url.URL, error) {
	if u, err := http.ProxyFromEnvironment(req); u != nil || err != nil {
		return u, err
	}
	if h := req.URL.Hostname(); h == "localhost" || h == "127.0.0.1" {
		return nil, nil
	}
	proxyMu.Lock()
	defer proxyMu.Unlock()
	if time.Since(proxyRead) > 5*time.Second {
		proxyRead, proxyURL = time.Now(), readSystemProxy()
	}
	return proxyURL, nil
}

var (
	proxyMu   sync.Mutex
	proxyRead time.Time
	proxyURL  *url.URL
)
