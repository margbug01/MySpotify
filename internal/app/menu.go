package app

import (
	"fmt"
	"runtime"

	"github.com/egoist/mygo"

	"vinyl/internal/l10n"
)

// syncDockMenu gives the Dock icon the player's controls (macOS), to pause
// without bringing up the window, saying 播放 or 暂停 as the player is.
// The Dock asks for no menu as it opens one, so it is set again whenever
// that changes.
func (a *app) syncDockMenu() {
	if runtime.GOOS != "darwin" {
		return
	}
	toggle := l10n.T("播放")
	if a.pl.Playing() {
		toggle = l10n.T("暂停")
	}
	idle := a.pl.Current() == nil
	sig := toggle + fmt.Sprint(idle)
	if sig == a.dockSig {
		return
	}
	a.dockSig = sig
	run := func(fn func()) func(*mygo.MenuItem, *mygo.Window) {
		return func(*mygo.MenuItem, *mygo.Window) {
			fn()
			a.redraw()
		}
	}
	mygo.App.Dock.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
		{Label: toggle, Disabled: idle, Click: run(func() { a.pl.TogglePlay() })},
		{Label: l10n.T("下一首"), Disabled: idle, Click: run(func() { a.pl.Next() })},
		{Label: l10n.T("上一首"), Disabled: idle, Click: run(func() { a.pl.Prev() })},
	}))
}

// setMenu gives the app its menu bar, localized and named as the app
// is, with the settings in the app menu and the player's controls in a
// menu of their own.
func (a *app) setMenu() {
	run := func(fn func()) func(*mygo.MenuItem, *mygo.Window) {
		return func(*mygo.MenuItem, *mygo.Window) {
			fn()
			a.redraw()
		}
	}
	name := appName
	appMenu := []*mygo.MenuItem{
		{Role: mygo.RoleAbout, Label: fmt.Sprintf(l10n.T("关于%s"), name)},
		{Label: l10n.T("检查更新…"), Click: run(func() {
			a.openSettings()
			a.checkUpdate(true)
		})},
		mygo.Separator(),
		{Label: l10n.T("设置…"), Accelerator: "CmdOrCtrl+,", Click: run(a.openSettings)},
		mygo.Separator(),
	}
	// Services and hiding are macOS's alone. Elsewhere MyGo leaves their
	// items out but keeps the separators around them, which then stack.
	if runtime.GOOS == "darwin" {
		appMenu = append(appMenu,
			&mygo.MenuItem{Role: mygo.RoleServices, Label: l10n.T("服务")},
			mygo.Separator(),
			&mygo.MenuItem{Role: mygo.RoleHide, Label: fmt.Sprintf(l10n.T("隐藏%s"), name)},
			&mygo.MenuItem{Role: mygo.RoleHideOthers, Label: l10n.T("隐藏其他")},
			&mygo.MenuItem{Role: mygo.RoleUnhide, Label: l10n.T("全部显示")},
			mygo.Separator(),
		)
	}
	appMenu = append(appMenu, &mygo.MenuItem{Role: mygo.RoleQuit, Label: fmt.Sprintf(l10n.T("退出%s"), name)})
	mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
		{Label: name, Submenu: appMenu},
		{Label: l10n.T("文件"), Submenu: []*mygo.MenuItem{
			{Label: l10n.T("新建歌单…"), Accelerator: "CmdOrCtrl+N", Click: run(func() {
				a.showMain()
				a.nowPlaying = false
				a.newPlaylist(nil)
			})},
		}},
		{Label: l10n.T("编辑"), Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleUndo, Label: l10n.T("撤销")},
			{Role: mygo.RoleRedo, Label: l10n.T("重做")},
			mygo.Separator(),
			{Role: mygo.RoleCut, Label: l10n.T("剪切")},
			{Role: mygo.RoleCopy, Label: l10n.T("拷贝")},
			{Role: mygo.RolePaste, Label: l10n.T("粘贴")},
			{Role: mygo.RoleSelectAll, Label: l10n.T("全选")},
		}},
		{Label: l10n.T("控制"), Submenu: []*mygo.MenuItem{
			{Label: l10n.T("播放/暂停"), Click: run(func() { a.pl.TogglePlay() })},
			{Label: l10n.T("下一首"), Accelerator: "CmdOrCtrl+Right", Click: run(func() { a.pl.Next() })},
			{Label: l10n.T("上一首"), Accelerator: "CmdOrCtrl+Left", Click: run(func() { a.pl.Prev() })},
			mygo.Separator(),
			{Label: l10n.T("喜欢/取消喜欢"), Accelerator: "CmdOrCtrl+D", Click: run(a.likeCurrent)},
			mygo.Separator(),
			{Label: l10n.T("随机播放"), Click: run(func() { a.pl.ToggleShuffle() })},
			{Label: l10n.T("切换循环模式"), Click: run(func() { a.pl.CycleRepeat() })},
			mygo.Separator(),
			{Label: l10n.T("正在播放"), Accelerator: "CmdOrCtrl+L", Click: run(func() {
				a.showMain()
				a.nowPlaying = !a.nowPlaying && a.pl.Current() != nil
			})},
		}},
		{Label: l10n.T("窗口"), Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleMinimize, Label: l10n.T("最小化")},
			{Role: mygo.RoleZoom, Label: l10n.T("缩放")},
			{Role: mygo.RoleClose, Label: l10n.T("关闭")},
			mygo.Separator(),
			{Label: name, Accelerator: "CmdOrCtrl+0", Click: run(a.showMain)},
			{Role: mygo.RoleFront, Label: l10n.T("前置全部窗口")},
		}},
	}))
}
