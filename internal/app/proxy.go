package app

import "vinyl/internal/spotify"

// applyProxy has the app's requests, and the web pages' that load Spotify
// itself, go through the proxy the listener set, else the system's.
func (a *app) applyProxy() {
	u, _ := spotify.ParseProxy(a.settings.Proxy) // kept only once it reads
	spotify.SetProxy(u)
	webProxy(u)
}

// setProxy sets the proxy typed in the settings, or says why it cannot.
func (a *app) setProxy() {
	u, err := spotify.ParseProxy(a.proxyText)
	if err != nil {
		a.proxyErr = "看不懂这个地址：要像 127.0.0.1:7890、http://… 或 socks5://…，带端口"
		return
	}
	a.proxyErr, a.settings.Proxy = "", ""
	if u != nil {
		a.settings.Proxy = u.String()
	}
	a.proxyText = a.settings.Proxy
	a.settings.save()
	a.applyProxy()
}
