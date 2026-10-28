//go:build !darwin

package spotify

// tokenUserAgent asks for the identity as the browser the app's other
// requests name, which is the one the web player's own token comes from.
const tokenUserAgent = UserAgent
