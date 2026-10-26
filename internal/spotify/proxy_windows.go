package spotify

import (
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows/registry"
)

// systemProxy routes requests through the proxy Windows is set to, read
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

func readSystemProxy() *url.URL {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer k.Close()
	if on, _, _ := k.GetIntegerValue("ProxyEnable"); on == 0 {
		return nil
	}
	server, _, _ := k.GetStringValue("ProxyServer")
	addr := pickProxy(server)
	if addr == "" {
		return nil
	}
	u, err := url.Parse("http://" + addr)
	if err != nil {
		return nil
	}
	return u
}

// pickProxy reads ProxyServer: "host:port", or "http=a:1;https=b:2;socks=c:3".
func pickProxy(server string) string {
	server = strings.TrimSpace(server)
	if !strings.Contains(server, "=") {
		return server
	}
	entries := map[string]string{}
	for _, part := range strings.Split(server, ";") {
		if k, v, ok := strings.Cut(part, "="); ok {
			entries[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
		}
	}
	if v := entries["https"]; v != "" {
		return v
	}
	return entries["http"]
}
