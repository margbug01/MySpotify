package app

import (
	"runtime"

	"github.com/egoist/mygo"
)

// setMenu gives the app its menu bar, in Chinese and named as the app
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
		{Role: mygo.RoleAbout, Label: "关于" + name},
		{Label: "检查更新…", Click: run(func() {
			a.openSettings()
			a.checkUpdate(true)
		})},
		mygo.Separator(),
		{Label: "设置…", Accelerator: "CmdOrCtrl+,", Click: run(a.openSettings)},
		mygo.Separator(),
	}
	// Services and hiding are macOS's alone. Elsewhere MyGo leaves their
	// items out but keeps the separators around them, which then stack.
	if runtime.GOOS == "darwin" {
		appMenu = append(appMenu,
			&mygo.MenuItem{Role: mygo.RoleServices, Label: "服务"},
			mygo.Separator(),
			&mygo.MenuItem{Role: mygo.RoleHide, Label: "隐藏" + name},
			&mygo.MenuItem{Role: mygo.RoleHideOthers, Label: "隐藏其他"},
			&mygo.MenuItem{Role: mygo.RoleUnhide, Label: "全部显示"},
			mygo.Separator(),
		)
	}
	appMenu = append(appMenu, &mygo.MenuItem{Role: mygo.RoleQuit, Label: "退出" + name})
	mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
		{Label: name, Submenu: appMenu},
		{Label: "文件", Submenu: []*mygo.MenuItem{
			{Label: "新建歌单…", Accelerator: "CmdOrCtrl+N", Click: run(func() {
				a.showMain()
				a.nowPlaying = false
				a.newPlaylist(nil)
			})},
		}},
		{Label: "编辑", Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleUndo, Label: "撤销"},
			{Role: mygo.RoleRedo, Label: "重做"},
			mygo.Separator(),
			{Role: mygo.RoleCut, Label: "剪切"},
			{Role: mygo.RoleCopy, Label: "拷贝"},
			{Role: mygo.RolePaste, Label: "粘贴"},
			{Role: mygo.RoleSelectAll, Label: "全选"},
		}},
		{Label: "控制", Submenu: []*mygo.MenuItem{
			{Label: "播放/暂停", Click: run(func() { a.pl.TogglePlay() })},
			{Label: "下一首", Accelerator: "CmdOrCtrl+Right", Click: run(func() { a.pl.Next() })},
			{Label: "上一首", Accelerator: "CmdOrCtrl+Left", Click: run(func() { a.pl.Prev() })},
			mygo.Separator(),
			{Label: "喜欢/取消喜欢", Accelerator: "CmdOrCtrl+D", Click: run(a.likeCurrent)},
			mygo.Separator(),
			{Label: "随机播放", Click: run(func() { a.pl.ToggleShuffle() })},
			{Label: "切换循环模式", Click: run(func() { a.pl.CycleRepeat() })},
			mygo.Separator(),
			{Label: "正在播放", Accelerator: "CmdOrCtrl+L", Click: run(func() {
				a.showMain()
				a.nowPlaying = !a.nowPlaying && a.pl.Current() != nil
			})},
		}},
		{Label: "窗口", Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleMinimize, Label: "最小化"},
			{Role: mygo.RoleZoom, Label: "缩放"},
			{Role: mygo.RoleClose, Label: "关闭"},
			mygo.Separator(),
			{Label: name, Accelerator: "CmdOrCtrl+0", Click: run(a.showMain)},
			{Role: mygo.RoleFront, Label: "前置全部窗口"},
		}},
	}))
}
