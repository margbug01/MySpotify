package app

import (
	"encoding/xml"
	"maps"
	"os"
	"slices"
	"testing"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"vinyl/internal/spotify"
)

// The keys MySpotify.sdef names are the ones serveScripts answers: one
// renamed on a side only, scripts get an error, or a record short a field.
func TestScriptKeys(t *testing.T) {
	b, err := os.ReadFile("../../resources/darwin/MySpotify.sdef")
	if err != nil {
		t.Fatal(err)
	}
	type props struct {
		Keys []struct {
			Key string `xml:"key,attr"`
		} `xml:"property>cocoa"`
	}
	var dict struct {
		Classes []props `xml:"suite>class"`
		Records []props `xml:"suite>record-type"`
	}
	if err := xml.Unmarshal(b, &dict); err != nil || len(dict.Classes) != 1 || len(dict.Records) != 1 {
		t.Fatalf("sdef: %v, %d classes, %d records", err, len(dict.Classes), len(dict.Records))
	}
	// The app has AppKit loaded by then; a test, not.
	if _, err := purego.Dlopen("/System/Library/Frameworks/AppKit.framework/AppKit", purego.RTLD_GLOBAL|purego.RTLD_NOW); err != nil {
		t.Fatal(err)
	}
	(&app{}).serveScripts()
	nsapp := objc.ID(objc.GetClass("NSApplication"))
	for _, k := range dict.Classes[0].Keys {
		if !objc.Send[bool](nsapp, objc.RegisterName("instancesRespondToSelector:"), objc.RegisterName(k.Key)) {
			t.Errorf("the application does not answer %s", k.Key)
		}
	}
	var want []string
	for _, k := range dict.Records[0].Keys {
		want = append(want, k.Key)
	}
	got := slices.Collect(maps.Keys(scriptTrack(&spotify.Track{})))
	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("track info: the sdef names %v, scriptTrack gives %v", want, got)
	}
}
