package spotify

import (
	"strings"
	"testing"
)

// adaptManifest turns the Widevine key line of a cbcs file into FairPlay's,
// its asset the file's ID, and leaves every other line as it was.
func TestAdaptManifestForFairPlay(t *testing.T) {
	in := "#EXTM3U\n#EXT-X-VERSION:6\n" +
		`#EXT-X-KEY:METHOD=SAMPLE-AES,URI="data:text/plain;base64,AAAAV3Bzc2g=",KEYID=0x24BBF7DC5917B8B3E95EA25E5CFEF6FE,KEYFORMATVERSIONS="1",KEYFORMAT="urn:uuid:edef8ba9-79d6-4ace-a3c8-27dcd51d21ed"` + "\n" +
		`#EXT-X-MAP:URI="https://audio4-ak.spotifycdn.com/audio/24bb?x=1",BYTERANGE="1396@0"` + "\n#EXTINF:10.0,\nhttps://audio4-ak.spotifycdn.com/audio/24bb?x=1\n#EXT-X-ENDLIST\n"
	out := adaptManifest(in, "24BBF7DC5917B8B3E95EA25E5CFEF6FE90149578")
	want := `#EXT-X-KEY:METHOD=SAMPLE-AES,URI="skd://24bbf7dc5917b8b3e95ea25e5cfef6fe90149578",KEYFORMATVERSIONS="1",KEYFORMAT="com.apple.streamingkeydelivery"`
	if !strings.Contains(out, want+"\n") {
		t.Fatalf("no FairPlay key line in\n%s", out)
	}
	if strings.Contains(out, "edef8ba9") {
		t.Fatal("Widevine key line left in")
	}
	if strings.Replace(out, want, "", 1) != strings.Replace(in, strings.Split(in, "\n")[2], "", 1) {
		t.Fatalf("other lines changed:\n%s", out)
	}
}
