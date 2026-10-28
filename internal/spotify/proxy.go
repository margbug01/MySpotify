package spotify

import (
	"net/http"
	"net/url"
)

// SystemProxy is the proxy Client's requests take, the system's, for the
// app's requests that go through another client: MyGo's updater asks GitHub
// with http.DefaultClient.
func SystemProxy(req *http.Request) (*url.URL, error) { return systemProxy(req) }
