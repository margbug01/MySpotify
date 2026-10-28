package spotify

// tokenUserAgent asks for the identity as Safari does, which is the browser
// the macOS web player is: a token Spotify cannot name the client of is one
// that makes a Connect device for Premium only, and a free listener's phone
// or web player would not see Vinyl. It is what the web view says it is:
// WebKit's own agent, and the suffix the app gives it.
const tokenUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) " + WebViewAgentSuffix
