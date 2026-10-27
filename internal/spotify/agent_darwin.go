package spotify

// WebViewAgentSuffix ends the user agent of the web player's window as
// Safari's ends: WKWebView leaves out "Version/… Safari/…", and the token
// the web player gets in a browser Spotify can't name makes a Connect device
// for Premium only (track-playback/v1/devices turns it down with
// premium_only_device), so a free listener's phone or web player wouldn't
// see Vinyl.
const WebViewAgentSuffix = "Version/27.0 Safari/605.1.15"
