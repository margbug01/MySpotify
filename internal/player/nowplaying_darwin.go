//go:build darwin

package player

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
)

type nativeVisibilityAPI struct {
	origin     func() uintptr
	eligible   func(uint8) uint8
	visibility func(uintptr, uint32)
}

var (
	nativeVisibilityOnce   sync.Once
	nativeVisibility       nativeVisibilityAPI
	nativeVisibilityErr    error
	nativeVisibilityChange atomic.Uint64
)

// loadNativeVisibility loads the same MediaRemote SPI WebKit uses when
// publishing or withdrawing its own media client. Keep the library loaded.
func loadNativeVisibility() (nativeVisibilityAPI, error) {
	nativeVisibilityOnce.Do(func() {
		lib, err := purego.Dlopen("/System/Library/PrivateFrameworks/MediaRemote.framework/MediaRemote", purego.RTLD_LOCAL|purego.RTLD_NOW)
		if err != nil {
			nativeVisibilityErr = err
			return
		}
		for _, symbol := range []struct {
			name string
			fn   any
		}{
			{"MRMediaRemoteGetLocalOrigin", &nativeVisibility.origin},
			{"MRMediaRemoteSetCanBeNowPlayingApplication", &nativeVisibility.eligible},
			{"MRMediaRemoteSetNowPlayingVisibility", &nativeVisibility.visibility},
		} {
			addr, err := purego.Dlsym(lib, symbol.name)
			if err != nil {
				nativeVisibilityErr = fmt.Errorf("%s: %w", symbol.name, err)
				return
			}
			purego.RegisterFunc(symbol.fn, addr)
		}
	})
	return nativeVisibility, nativeVisibilityErr
}

// setNativeNowPlayingVisible runs after MyGo's queued Set/Clear. Only the
// latest owner applies, so a late local update cannot hide remote playback.
// MediaRemote's client here is the app process; WebKit publishes from its
// GPU process and keeps its own eligibility and visibility.
func setNativeNowPlayingVisible(visible bool) {
	change := nativeVisibilityChange.Add(1)
	mygo.App.WhenReady(func() {
		if nativeVisibilityChange.Load() != change {
			return
		}
		api, err := loadNativeVisibility()
		if err != nil {
			log.Printf("player: native media session visibility unavailable: %v", err)
			return
		}
		if visible {
			api.eligible(1)
			api.visibility(api.origin(), 1) // AlwaysVisible
		} else {
			// Unknown also withdraws the default player. Stopped alone
			// leaves an empty, selectable player in Control Center.
			center := objc.ID(objc.GetClass("MPNowPlayingInfoCenter")).Send(objc.RegisterName("defaultCenter"))
			if center != 0 {
				center.Send(objc.RegisterName("setPlaybackState:"), uintptr(0))
			}
			api.visibility(api.origin(), 3) // NeverVisible
			api.eligible(0)
		}
	})
}
