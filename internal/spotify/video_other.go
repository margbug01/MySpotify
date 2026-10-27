//go:build !darwin

package spotify

import (
	"encoding/base64"
	"encoding/hex"
	"strings"
)

// videoKeySystem and videoFileType are what the web player plays a video
// from where Widevine decrypts: MP4 encrypted with cenc.
const (
	videoKeySystem = "widevine"
	videoFileType  = "mp4"
)

// videoKey names the video's key as the manifests of songs do for
// Widevine: its PSSH box, and the profile's key ID.
func videoKey(enc encryptionInfo, p videoProfile, encodingID string) string {
	kid, _ := base64.StdEncoding.DecodeString(p.KeyID)
	return `#EXT-X-KEY:METHOD=SAMPLE-AES,URI="data:text/plain;base64,` + enc.Data + `",KEYID=0x` + strings.ToUpper(hex.EncodeToString(kid)) + `,KEYFORMATVERSIONS="1",KEYFORMAT="urn:uuid:edef8ba9-79d6-4ace-a3c8-27dcd51d21ed"`
}

// videoLicense is where the manifest says Widevine's licence comes from.
func videoLicense(enc encryptionInfo) string { return enc.Endpoint }
