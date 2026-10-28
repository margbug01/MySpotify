package spotify

// tokenUserAgent asks for the identity as Safari does, which is the browser
// the macOS web player is: a token Spotify cannot name the client of is one
// that makes a Connect device for Premium only, and a free listener's phone
// or web player would not see Vinyl (see WebViewAgentSuffix).
const tokenUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/27.0 Safari/605.1.15"
