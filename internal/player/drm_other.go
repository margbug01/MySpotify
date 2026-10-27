//go:build !darwin

package player

// keySystem decrypts what plays: Widevine, as in the web player.
const keySystem = "com.widevine.alpha"

// licenseHost is Spotify's licence service for it; tests swap it for their
// own.
var licenseHost = "https://spclient.wg.spotify.com/widevine-license/v1"

// licenseQuery is nothing more for Widevine.
const licenseQuery = ""
