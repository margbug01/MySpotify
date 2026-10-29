//go:build darwin

package player

import (
	"log"
	"strings"

	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
)

// throttling is WebKit's feature that suspends the process of a hidden page.
const throttling = "BackgroundWebContentRunningBoardThrottlingEnabled"

// keepAwake has WebKit leave the pages in win running while they are hidden
// and nothing plays. WebKit suspends a hidden page's process ten seconds
// after its audio stops, as a pause leaves it: a play from the system (the
// media keys, Control Center, Mami playing again what it paused while one
// talked) then waits in the suspended process until something wakes it, as
// the window showing does, and the song stays paused.
func keepAwake(win *mygo.Window) {
	if win == nil || win.NativeHandle() == 0 {
		return
	}
	for _, web := range webViews(objc.ID(win.NativeHandle()).Send(objc.RegisterName("contentView"))) {
		prefs := web.Send(objc.RegisterName("configuration")).Send(objc.RegisterName("preferences"))
		if kept := keepPrefsAwake(prefs); len(kept) > 0 {
			log.Printf("player: WebKit keeps the page running (%s)", strings.Join(kept, ", "))
		} else {
			log.Print("player: WebKit may suspend the page while it is paused: no setting to keep it running")
		}
	}
}

// webViews returns the web views in view, itself included.
func webViews(view objc.ID) []objc.ID {
	if view == 0 {
		return nil
	}
	if objc.Send[bool](view, objc.RegisterName("isKindOfClass:"), objc.GetClass("WKWebView")) {
		return []objc.ID{view}
	}
	var webs []objc.ID
	subviews := view.Send(objc.RegisterName("subviews"))
	for i := range objc.Send[uint](subviews, objc.RegisterName("count")) {
		webs = append(webs, webViews(subviews.Send(objc.RegisterName("objectAtIndex:"), i))...)
	}
	return webs
}

// keepPrefsAwake turns off in prefs what suspends a hidden page, and returns
// what it turned off. Both settings are WebKit's own and private, and either
// keeps the page running: both, as a later macOS may drop one.
func keepPrefsAwake(prefs objc.ID) []string {
	var kept []string
	if f := webFeature(throttling); f != 0 && responds(prefs, "_setEnabled:forFeature:") {
		prefs.Send(objc.RegisterName("_setEnabled:forFeature:"), false, f)
		kept = append(kept, "background throttling off")
	}
	if responds(prefs, "_setPageVisibilityBasedProcessSuppressionEnabled:") {
		prefs.Send(objc.RegisterName("_setPageVisibilityBasedProcessSuppressionEnabled:"), false)
		kept = append(kept, "process suppression off")
	}
	return kept
}

// webFeature returns WebKit's feature named key, 0 if it has none.
func webFeature(key string) objc.ID {
	class := objc.ID(objc.GetClass("WKPreferences"))
	if class == 0 || !responds(class, "_features") {
		return 0
	}
	name := objc.ID(objc.GetClass("NSString")).Send(objc.RegisterName("stringWithUTF8String:"), key+"\x00")
	features := class.Send(objc.RegisterName("_features"))
	for i := range objc.Send[uint](features, objc.RegisterName("count")) {
		f := features.Send(objc.RegisterName("objectAtIndex:"), i)
		if objc.Send[bool](f.Send(objc.RegisterName("key")), objc.RegisterName("isEqualToString:"), name) {
			return f
		}
	}
	return 0
}

func responds(o objc.ID, selector string) bool {
	return objc.Send[bool](o, objc.RegisterName("respondsToSelector:"), objc.RegisterName(selector))
}
