package app

import (
	"net/url"
	"os"
	"strings"
)

const webProxyNote = "；登录页面重启后生效"

// webProxy has WebView2 go through u. It reads its arguments as the first
// web page opens: a proxy set later takes the pages at the next start.
var webProxy = func(u *url.URL) {
	args := os.Getenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS")
	if u == nil || strings.Contains(args, "--proxy-server") {
		return
	}
	// Chromium takes no password here, and knows socks5 alone.
	scheme := u.Scheme
	if scheme == "socks5h" {
		scheme = "socks5"
	}
	os.Setenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", args+" --proxy-server="+scheme+"://"+u.Host)
}
