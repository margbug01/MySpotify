//go:build !windows

package spotify

import "net/http"

var systemProxy = http.ProxyFromEnvironment
