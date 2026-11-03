package app

import (
	"errors"
	"log"
	"net/url"
	"strings"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

const webProxyNote = ""

// webProxy has the web pages go through u, nil going back to the system's
// proxy: the sign-in page, which loads Spotify itself. The app's own
// pages, on 127.0.0.1, go direct. It takes macOS 14; before, the pages
// keep to the system's proxy. On the main thread.
var webProxy = func(u *url.URL) {
	if u == nil && !webProxied {
		return // as WebKit has it
	}
	store := objc.ID(objc.GetClass("WKWebsiteDataStore")).Send(objc.RegisterName("defaultDataStore"))
	if store == 0 || !objc.Send[bool](store, objc.RegisterName("respondsToSelector:"), objc.RegisterName("setProxyConfigurations:")) {
		return
	}
	configs := objc.ID(objc.GetClass("NSMutableArray")).Send(objc.RegisterName("array"))
	if u != nil {
		c, err := nwProxyConfig(u)
		if err != nil {
			log.Printf("proxy: the web pages keep to the system's: %v", err)
			return
		}
		configs.Send(objc.RegisterName("addObject:"), c)
		c.Send(objc.RegisterName("release"))
	}
	store.Send(objc.RegisterName("setProxyConfigurations:"), configs)
	webProxied = u != nil
}

// webProxied says the web pages were given a proxy.
var webProxied bool

// nwProxyConfig makes Network.framework's proxy configuration of u.
func nwProxyConfig(u *url.URL) (objc.ID, error) {
	lib, err := purego.Dlopen("/System/Library/Frameworks/Network.framework/Network", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return 0, err
	}
	if _, err := purego.Dlsym(lib, "nw_proxy_config_create_socksv5"); err != nil {
		return 0, err // before macOS 14
	}
	var (
		endpoint    func(host, port string) objc.ID
		httpConnect func(endpoint, tls objc.ID) objc.ID
		socks       func(endpoint objc.ID) objc.ID
		tls         func() objc.ID
		login       func(config objc.ID, user, password string)
		exclude     func(config objc.ID, domain string)
	)
	purego.RegisterLibFunc(&endpoint, lib, "nw_endpoint_create_host")
	purego.RegisterLibFunc(&httpConnect, lib, "nw_proxy_config_create_http_connect")
	purego.RegisterLibFunc(&socks, lib, "nw_proxy_config_create_socksv5")
	purego.RegisterLibFunc(&tls, lib, "nw_tls_create_options")
	purego.RegisterLibFunc(&login, lib, "nw_proxy_config_set_username_and_password")
	purego.RegisterLibFunc(&exclude, lib, "nw_proxy_config_add_excluded_domain")

	at := endpoint(u.Hostname(), u.Port())
	defer at.Send(objc.RegisterName("release"))
	var c objc.ID
	switch {
	case strings.HasPrefix(u.Scheme, "socks5"):
		c = socks(at)
	case u.Scheme == "https":
		over := tls()
		defer over.Send(objc.RegisterName("release"))
		c = httpConnect(at, over)
	default:
		c = httpConnect(at, 0)
	}
	if c == 0 {
		return 0, errors.New("Network.framework made no proxy of " + u.Redacted())
	}
	if user := u.User.Username(); user != "" {
		password, _ := u.User.Password()
		login(c, user, password)
	}
	for _, local := range []string{"localhost", "127.0.0.1"} {
		exclude(c, local)
	}
	return c, nil
}
