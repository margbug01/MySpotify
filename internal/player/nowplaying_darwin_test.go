//go:build darwin

package player

import (
	"testing"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// The command center's switch that keeps it from making the hidden native
// player eligible again must be there: without it, the empty card of #54
// comes back.
func TestCommandCenterAutomaticSwitchAvailable(t *testing.T) {
	if _, err := purego.Dlopen("/System/Library/Frameworks/MediaPlayer.framework/MediaPlayer", purego.RTLD_GLOBAL|purego.RTLD_NOW); err != nil {
		t.Fatal(err)
	}
	center := objc.ID(objc.GetClass("MPRemoteCommandCenter")).Send(objc.RegisterName("sharedCommandCenter"))
	if center == 0 || !responds(center, "setDisableAutomaticCanBeNowPlaying:") {
		t.Fatal("MPRemoteCommandCenter has no setDisableAutomaticCanBeNowPlaying:")
	}
}

// A missing or renamed MediaRemote symbol must be detected before the
// native client is hidden, rather than calling a nil function at runtime.
func TestNativeVisibilityAPIAvailable(t *testing.T) {
	api, err := loadNativeVisibility()
	if err != nil {
		t.Fatal(err)
	}
	if api.origin == nil || api.eligible == nil || api.visibility == nil {
		t.Fatal("native visibility API is incomplete")
	}
}
