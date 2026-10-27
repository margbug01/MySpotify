package spotify

import "strings"

// mediaFormat is the files the web player asks for in Safari: AAC
// encrypted with cbcs, which FairPlay, the one key system WebKit has,
// decrypts.
const mediaFormat = "file_ids_mp4_cbcs"

// adaptManifest names FairPlay as the key system, as the web player writes
// the manifest it plays in Safari: the service writes the key line of cbcs
// files for Widevine all the same. The asset in the skd:// URI is the
// file's ID in hex, which WebKit asks the licence for.
func adaptManifest(m, fileID string) string {
	lines := strings.Split(m, "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, "#EXT-X-KEY:") {
			continue
		}
		end := ""
		if strings.HasSuffix(line, "\r") {
			end = "\r"
		}
		lines[i] = `#EXT-X-KEY:METHOD=SAMPLE-AES,URI="skd://` + strings.ToLower(fileID) + `",KEYFORMATVERSIONS="1",KEYFORMAT="com.apple.streamingkeydelivery"` + end
	}
	return strings.Join(lines, "\n")
}
