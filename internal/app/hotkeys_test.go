package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// swapHotkeys stands in for the system's global shortcuts, returning those
// registered.
func swapHotkeys(t *testing.T) map[string]func() {
	regs := map[string]func(){}
	reg, unreg := registerHotkey, unregisterHotkeys
	registerHotkey = func(acc string, fn func()) error { regs[acc] = fn; return nil }
	unregisterHotkeys = func() { clear(regs) }
	t.Cleanup(func() { registerHotkey, unregisterHotkeys = reg, unreg })
	return regs
}

// A shortcut's button, pressed, takes the keys pressed next as its
// shortcut, for the whole system: a key alone is passed over, and the
// shortcut given one action is taken from the one that had it.
func TestHotkeyRecord(t *testing.T) {
	regs := swapHotkeys(t)
	a := newTestApp()
	tt := settingsTester(a)
	click(t, tt, "下一首的快捷键")
	tt.Key(0, ui.KeyN)
	tt.Frame()
	if a.recording != "next" || len(a.settings.Hotkeys) != 0 {
		t.Fatalf("a key alone was taken: waiting for %q, shortcuts %v", a.recording, a.settings.Hotkeys)
	}
	tt.Key(ui.Ctrl|ui.Alt, ui.KeyRight)
	tt.Frame()
	if a.settings.Hotkeys["next"] != "Ctrl+Alt+Right" || a.recording != "" || regs["Ctrl+Alt+Right"] == nil {
		t.Fatalf("shortcuts %v, registered %d, waiting for %q", a.settings.Hotkeys, len(regs), a.recording)
	}
	click(t, tt, "上一首的快捷键")
	tt.Key(ui.Ctrl|ui.Alt, ui.KeyRight)
	tt.Frame()
	if a.settings.Hotkeys["prev"] != "Ctrl+Alt+Right" || a.settings.Hotkeys["next"] != "" || len(regs) != 1 {
		t.Fatalf("given to 上一首: shortcuts %v, registered %d", a.settings.Hotkeys, len(regs))
	}
}

// While a button waits for keys, the shortcuts registered let go of theirs,
// which would never reach the window; Esc, or leaving the button, gives up
// and has them back. Delete clears the shortcut.
func TestHotkeyRecordGivesUp(t *testing.T) {
	regs := swapHotkeys(t)
	a := newTestApp()
	a.settings.Hotkeys = map[string]string{"play": "Ctrl+Alt+Space"}
	a.applyHotkeys()
	tt := settingsTester(a)
	click(t, tt, "播放/暂停的快捷键")
	if len(regs) != 0 {
		t.Fatal("waiting for keys, the shortcuts still take theirs")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.recording != "" || regs["Ctrl+Alt+Space"] == nil || a.settings.Hotkeys["play"] != "Ctrl+Alt+Space" {
		t.Fatalf("Esc: waiting for %q, registered %d, shortcuts %v", a.recording, len(regs), a.settings.Hotkeys)
	}
	click(t, tt, "下一首的快捷键")
	tt.Key(0, ui.KeyTab)
	tt.Frame()
	if a.recording != "" || regs["Ctrl+Alt+Space"] == nil {
		t.Fatalf("left: waiting for %q, registered %d", a.recording, len(regs))
	}
	click(t, tt, "播放/暂停的快捷键")
	tt.Key(0, ui.KeyBackspace)
	tt.Frame()
	if len(a.settings.Hotkeys) != 0 || len(regs) != 0 || a.recording != "" {
		t.Fatalf("Delete: shortcuts %v, registered %d, waiting for %q", a.settings.Hotkeys, len(regs), a.recording)
	}
}
