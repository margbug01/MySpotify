//go:build darwin

package player

import "testing"

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
