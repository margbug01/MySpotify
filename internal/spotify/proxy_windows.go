package spotify

import (
	"net/url"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// readSystemProxy reads the proxy Windows is set to.
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
