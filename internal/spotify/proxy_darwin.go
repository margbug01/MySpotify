package spotify

import (
	"bufio"
	"net"
	"net/url"
	"os/exec"
	"strings"
)

// readSystemProxy reads the proxy macOS is set to, as scutil --proxy shows
// it: what apps opened from the Finder get no environment for.
func readSystemProxy() *url.URL {
	out, err := exec.Command("/usr/sbin/scutil", "--proxy").Output()
	if err != nil {
		return nil
	}
	return parseScutilProxy(string(out))
}

// parseScutilProxy picks the HTTPS proxy, else the HTTP one, else SOCKS,
// whichever is enabled; a proxy auto-configuration file it leaves alone.
func parseScutilProxy(text string) *url.URL {
	fields := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		if k, v, ok := strings.Cut(sc.Text(), " : "); ok {
			fields[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	for _, p := range []struct{ key, scheme string }{{"HTTPS", "http"}, {"HTTP", "http"}, {"SOCKS", "socks5"}} {
		host, port := fields[p.key+"Proxy"], fields[p.key+"Port"]
		if fields[p.key+"Enable"] != "1" || host == "" || port == "" {
			continue
		}
		return &url.URL{Scheme: p.scheme, Host: net.JoinHostPort(host, port)}
	}
	return nil
}
