//go:build !windows && !darwin

package spotify

import "net/http"

var systemProxy = http.ProxyFromEnvironment
