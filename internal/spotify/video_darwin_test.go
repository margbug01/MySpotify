package spotify

import (
	"strings"
	"testing"
)

// A video plays in MPEG-TS, its key FairPlay's asset under the encoding's
// ID, and its licence comes from where the manifest says, the asset's ID
// in hex: as the web player plays it in Safari.
func TestVideoHLSForFairPlay(t *testing.T) {
	v := fixtureVideo(t)
	key := `#EXT-X-KEY:METHOD=SAMPLE-AES,URI="skd://51920b90cf1311f186341dcda1c4ee21",KEYFORMATVERSIONS="1",KEYFORMAT="com.apple.streamingkeydelivery"` + "\n"
	for name, pl := range v.Playlists {
		if !strings.Contains(pl, key) || strings.Contains(pl, "#EXT-X-MAP") || !strings.Contains(pl, ".ts?") {
			t.Fatalf("%s:\n%s", name, pl)
		}
	}
	if want := "/fairplay-license/v2/video/license/51920b90cf1311f186341dcda1c4ee21?mpegTs=true&assetId=hex"; v.License != want {
		t.Fatalf("licence %s", v.License)
	}
}
