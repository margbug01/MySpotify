//go:build !darwin

package spotify

import (
	"strings"
	"testing"
)

// A video plays in MP4 from its init section, its key Widevine's PSSH
// box and the profile's key ID, as a song's manifest names it.
func TestVideoHLSForWidevine(t *testing.T) {
	v := fixtureVideo(t)
	key := `#EXT-X-KEY:METHOD=SAMPLE-AES,URI="data:text/plain;base64,AAAAVXBzc2g=",KEYID=0xC07728DD3D5617EBAB4A3F58FEAE95A3,KEYFORMATVERSIONS="1",KEYFORMAT="urn:uuid:edef8ba9-79d6-4ace-a3c8-27dcd51d21ed"` + "\n"
	for name, pl := range v.Playlists {
		if !strings.Contains(pl, key) || !strings.Contains(pl, `#EXT-X-MAP:URI="https://video-fa.scdn.co/v1/e/`) || !strings.Contains(pl, ".mp4?") {
			t.Fatalf("%s:\n%s", name, pl)
		}
	}
	if want := "/widevine-license/v1/video/license"; v.License != want {
		t.Fatalf("licence %s", v.License)
	}
}
