//go:build darwin

package player

import (
	"testing"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// The settings that keep the page running after a pause are WebKit's own
// and private: a macOS that drops or renames them has the page suspended
// again, and a system play after a pause of ten seconds never reaching it.
func TestPageKeptAwake(t *testing.T) {
	if _, err := purego.Dlopen("/System/Library/Frameworks/WebKit.framework/WebKit", purego.RTLD_GLOBAL|purego.RTLD_NOW); err != nil {
		t.Fatal(err)
	}
	var kept []string
	var suppressed, throttled bool
	onMain(func() {
		prefs := objc.ID(objc.GetClass("WKPreferences")).Send(objc.RegisterName("new"))
		defer prefs.Send(objc.RegisterName("release"))
		kept = keepPrefsAwake(prefs)
		suppressed = objc.Send[bool](prefs, objc.RegisterName("_pageVisibilityBasedProcessSuppressionEnabled"))
		throttled = objc.Send[bool](prefs, objc.RegisterName("_isEnabledForFeature:"), webFeature(throttling))
	})
	if len(kept) != 2 || suppressed || throttled {
		t.Fatalf("kept the page running with %q: suppression %v, throttling %v", kept, suppressed, throttled)
	}
}
