//go:build !darwin

package player

// keySystem decrypts what plays: Widevine, as in the web player.
const keySystem = "com.widevine.alpha"

// licenseHost is Spotify's licence service for it; tests swap it for their
// own.
var licenseHost = "https://spclient.wg.spotify.com/widevine-license/v1"

// licenseQuery is nothing more for Widevine.
const licenseQuery = ""

// playsScheme is what the key system here plays, "" where it plays whatever
// it is given: Widevine does both cenc and cbcs, so there is nothing to
// warn of.
const playsScheme = ""
