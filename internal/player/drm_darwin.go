package player

// keySystem is the one Safari's web player plays with: FairPlay through
// WebKit's prefixed API, as WebKit's own HLS asks for keys (see the page's
// nativeFairPlay).
const keySystem = "com.apple.fps.1_0"

// licenseHost is Spotify's licence service for it; tests swap it for their
// own.
var licenseHost = "https://spclient.wg.spotify.com/fairplay-license/v1"

// licenseQuery says the asset in FairPlay's requests is named in hex, as
// the web player tells the service.
const licenseQuery = "?assetId=hex"

// playsScheme is what the key system here plays: FairPlay is SAMPLE-AES
// (cbcs) only, and a track in cenc would be a still picture with no error
// anywhere to say why.
const playsScheme = "cbcs"
