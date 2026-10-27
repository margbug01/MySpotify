package spotify

import "strings"

// videoKeySystem and videoFileType are what the web player plays a video
// in Safari from: MPEG-TS encrypted with cbcs, which FairPlay decrypts.
const (
	videoKeySystem = "fairplay"
	videoFileType  = "ts"
)

// videoKey names the video's key as the web player does in Safari: the
// asset in the skd:// URI is the encoding's ID, which WebKit asks the
// licence for.
func videoKey(enc encryptionInfo, p videoProfile, encodingID string) string {
	return `#EXT-X-KEY:METHOD=SAMPLE-AES,URI="skd://` + strings.ToLower(encodingID) + `",KEYFORMATVERSIONS="1",KEYFORMAT="com.apple.streamingkeydelivery"`
}

// videoLicense is where the manifest says FairPlay's licence comes from,
// told, as for a song's, that the asset's ID is in hex.
func videoLicense(enc encryptionInfo) string {
	sep := "?"
	if strings.Contains(enc.Endpoint, "?") {
		sep = "&"
	}
	return enc.Endpoint + sep + "assetId=hex"
}
