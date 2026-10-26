package app

import (
	_ "embed"
	"log"

	"github.com/egoist/mygo"
)

// trayIcon is the app's icon, as small as the notification area shows it.
//
//go:embed tray.png
var trayIcon []byte

// newTray puts the app in the notification area, where it stays while its
// window is closed: a click shows the window, a right click the player's
// controls and a way to quit. It returns nil where there is none.
func (a *app) newTray() *mygo.Tray {
	name := mygo.App.Name()
	t, err := mygo.NewTray(mygo.TrayOptions{ID: "main", Icon: trayIcon, ToolTip: name})
	if err != nil {
		log.Printf("tray: %v", err)
		return nil
	}
	t.OnClick(a.showMain)
	t.OnRightClick(func() {
		run := func(fn func()) func(*mygo.MenuItem, *mygo.Window) {
			return func(*mygo.MenuItem, *mygo.Window) {
				fn()
				a.redraw()
			}
		}
		toggle := "播放"
		if a.pl.Playing() {
			toggle = "暂停"
		}
		idle := a.pl.Current() == nil
		t.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
			{Label: "打开 " + name, Click: run(a.showMain)},
			mygo.Separator(),
			{Label: toggle, Disabled: idle, Click: run(func() { a.pl.TogglePlay() })},
			{Label: "上一首", Disabled: idle, Click: run(func() { a.pl.Prev() })},
			{Label: "下一首", Disabled: idle, Click: run(func() { a.pl.Next() })},
			mygo.Separator(),
			{Label: "退出 " + name, Click: run(mygo.App.Quit)},
		}))
		// Without a menu, a click reaches OnClick.
		defer t.SetMenu(nil)
		t.PopUpMenu()
	})
	return t
}
