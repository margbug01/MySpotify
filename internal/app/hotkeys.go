package app

import (
	"fmt"
	"log"
	"runtime"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"vinyl/internal/l10n"
)

// hotkeyActions are what global shortcuts do, as the settings list them.
var hotkeyActions = []struct {
	id, label string
	do        func(*app)
}{
	{"play", "播放/暂停", func(a *app) { a.pl.TogglePlay() }},
	{"next", "下一首", func(a *app) { a.pl.Next() }},
	{"prev", "上一首", func(a *app) { a.pl.Prev() }},
}

// MyGo's global shortcuts, which need the app running: tests swap them.
var (
	registerHotkey    = mygo.GlobalShortcut.Register
	unregisterHotkeys = mygo.GlobalShortcut.UnregisterAll
)

// applyHotkeys registers the global shortcuts the listener set, anew, and
// notes those the system would not take.
func (a *app) applyHotkeys() {
	unregisterHotkeys()
	a.hotkeyErr = map[string]bool{}
	for _, act := range hotkeyActions {
		acc := a.settings.Hotkeys[act.id]
		if acc == "" {
			continue
		}
		if err := registerHotkey(acc, func() { act.do(a); a.redraw() }); err != nil {
			log.Printf("hotkey %s, %s: %v", act.id, acc, err)
			a.hotkeyErr[act.id] = true
		}
	}
}

// hotkeyButton shows the global shortcut of the action id; pressed, it
// waits for the keys of a new one, which Esc gives up and Delete clears.
func (a *app) hotkeyButton(c *ui.Context, id, label string) {
	acc, rec := a.settings.Hotkeys[id], a.recording == id
	text := l10n.T("未设置")
	switch {
	case rec:
		text = l10n.T("请按下快捷键…")
	case acc != "":
		text = hotkeyLabel(acc)
	}
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		if acc != "" && !rec {
			if ui.Button(c.Key("hotkey-clear-"+id), l10n.T("清除")).Label(fmt.Sprintf(l10n.T("清除%s的快捷键"), label)).Clicked() {
				a.setHotkey(id, "")
			}
		}
		b := ui.Button(c.Key("hotkey-"+id), text).Label(fmt.Sprintf(l10n.T("%s的快捷键"), label)).MinWidth(128)
		b.HandleInput(func(ev ui.InputEvent) bool { return a.hotkeyKey(id, ev) })
		if rec {
			// Focused as the press left it; elsewhere, it gives up.
			if b.Focused() {
				a.recFocused = true
			} else if a.recFocused {
				a.stopRecording()
			}
		}
		if b.Clicked() {
			if rec {
				a.stopRecording()
			} else {
				a.startRecording(id)
				b.Focus()
			}
		}
	})
}

// hotkeyKey takes a key pressed while the settings wait for the shortcut
// of the action id.
func (a *app) hotkeyKey(id string, ev ui.InputEvent) bool {
	if a.recording != id || ev.Kind != ui.InputKeyDown {
		return false
	}
	switch {
	case ev.Mods == 0 && ev.Key == ui.KeyTab:
		return false // on to the next control, which gives up
	case ev.Mods == 0 && ev.Key == ui.KeyEscape:
		a.stopRecording()
	case ev.Mods == 0 && (ev.Key == ui.KeyBackspace || ev.Key == ui.KeyDelete):
		a.setHotkey(id, "")
	default:
		if acc := accelerator(ev); acc != "" {
			a.setHotkey(id, acc)
		}
	}
	return true
}

// setHotkey keeps acc as the shortcut of the action id, "" for none, and
// registers the shortcuts again. A shortcut does one thing: another action
// that had it loses it.
func (a *app) setHotkey(id, acc string) {
	if a.settings.Hotkeys == nil {
		a.settings.Hotkeys = map[string]string{}
	}
	delete(a.settings.Hotkeys, id)
	if acc != "" {
		for other, had := range a.settings.Hotkeys {
			if had == acc {
				delete(a.settings.Hotkeys, other)
			}
		}
		a.settings.Hotkeys[id] = acc
	}
	a.settings.save()
	a.stopRecording()
}

func (a *app) startRecording(id string) {
	// Registered, the shortcuts would take their keys before the window.
	unregisterHotkeys()
	a.recording, a.recFocused = id, false
}

func (a *app) stopRecording() {
	a.recording = ""
	a.applyHotkeys()
}

// accelerator names the keys of ev as GlobalShortcut takes them, by where
// they are on the keyboard, as macOS registers them. Keys alone, or with
// Shift alone, are what is typed everywhere: "", save the F keys.
func accelerator(ev ui.InputEvent) string {
	key := ev.PhysicalKey
	if key == ui.KeyUnknown {
		key = ev.Key
	}
	name := keyName(key)
	if name == "" || ev.Mods&(ui.Ctrl|ui.Alt|ui.Super) == 0 && !(key >= ui.KeyF1 && key <= ui.KeyF12) {
		return ""
	}
	var parts []string
	for _, m := range []struct {
		mod  ui.Modifiers
		name string
	}{{ui.Ctrl, "Ctrl"}, {ui.Alt, "Alt"}, {ui.Shift, "Shift"}, {ui.Super, "Super"}} {
		if ev.Mods&m.mod != 0 {
			parts = append(parts, m.name)
		}
	}
	return strings.Join(append(parts, name), "+")
}

func keyName(k ui.Key) string {
	switch {
	case k >= ui.KeyA && k <= ui.KeyZ:
		return string(rune('A' + k - ui.KeyA))
	case k >= ui.Key0 && k <= ui.Key9:
		return string(rune('0' + k - ui.Key0))
	case k >= ui.KeyF1 && k <= ui.KeyF12:
		return fmt.Sprintf("F%d", k-ui.KeyF1+1)
	}
	return keyNames[k]
}

var keyNames = map[ui.Key]string{
	ui.KeySpace: "Space", ui.KeyEnter: "Enter", ui.KeyTab: "Tab", ui.KeyBackspace: "Backspace", ui.KeyDelete: "Delete",
	ui.KeyLeft: "Left", ui.KeyRight: "Right", ui.KeyUp: "Up", ui.KeyDown: "Down",
	ui.KeyHome: "Home", ui.KeyEnd: "End", ui.KeyPageUp: "PageUp", ui.KeyPageDown: "PageDown", ui.KeyInsert: "Insert",
	ui.KeyMinus: "-", ui.KeyEqual: "=", ui.KeyComma: ",", ui.KeyPeriod: ".", ui.KeySlash: "/", ui.KeySemicolon: ";",
	ui.KeyQuote: "'", ui.KeyBracketLeft: "[", ui.KeyBracketRight: "]", ui.KeyBackslash: "\\", ui.KeyBackquote: "`",
}

// hotkeyLabel shows an accelerator as the system's menus do: ⌃⌥⇧⌘→ on
// macOS, Ctrl+Alt+→ elsewhere.
func hotkeyLabel(acc string) string {
	parts := strings.Split(acc, "+")
	for i, p := range parts {
		if s := hotkeySymbols[p]; s != "" {
			parts[i] = s
		}
	}
	if runtime.GOOS == "darwin" {
		return strings.Join(parts, "")
	}
	return strings.Join(parts, "+")
}

var hotkeySymbols = func() map[string]string {
	m := map[string]string{"Left": "←", "Right": "→", "Up": "↑", "Down": "↓", "Super": "Win"}
	if runtime.GOOS == "darwin" {
		for k, v := range map[string]string{"Ctrl": "⌃", "Alt": "⌥", "Shift": "⇧", "Super": "⌘", "Enter": "↩", "Tab": "⇥", "Backspace": "⌫", "Delete": "⌦"} {
			m[k] = v
		}
	}
	return m
}()
