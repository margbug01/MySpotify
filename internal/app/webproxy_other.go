//go:build !darwin && !windows

package app

import "net/url"

const webProxyNote = "；登录页面仍走系统代理"

// webProxy leaves WebKitGTK's pages to the system's proxy.
var webProxy = func(*url.URL) {}
