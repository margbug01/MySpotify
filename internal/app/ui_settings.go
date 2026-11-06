package app

import (
	"cmp"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// openSettings shows the settings window, opening it the first time.
func (a *app) openSettings() {
	a.measureCache()
	if a.settingsWin != nil {
		a.settingsWin.Show()
		a.settingsWin.Focus()
		return
	}
	a.proxyText, a.proxyErr = a.settings.Proxy, ""
	a.settingsWin = mygo.NewWindow(mygo.WindowOptions{
		Title:           "设置",
		Width:           560,
		Height:          720,
		MinWidth:        480,
		MinHeight:       420,
		StateKey:        "settings",
		DisableMaximize: true,
		DisableMinimize: true,
		// The app's menu has nothing for this window: on Linux and
		// Windows it stays out of sight until Alt shows it.
		AutoHideMenuBar: true,
		BackgroundColor: "light-dark(#f4f4f6, #1c1c1f)",
		Content:         ui.View(a.settingsView),
	})
	a.settingsWin.OnClosed(func() {
		a.settingsWin = nil
		if a.recording != "" {
			a.stopRecording()
		}
	})
}

// changed applies the settings everywhere and keeps them.
func (a *app) changed() {
	a.settings.save()
	a.settings.applyAppearance()
	if a.notch != nil {
		a.notch.setEnabled(a.settings.Notch)
	}
	a.redraw()
}

func (a *app) settingsView(c *ui.Context) {
	t := *c.Theme()
	p := paletteFor(&t)
	t.Accent, t.AccentText = p.accent, p.onAccent
	t.Font = appFont
	c.SetTheme(&t)
	c.Root().Background(p.sidebar)
	if a.recording == "" { // else ⌘W may be the shortcut wanted
		c.OnShortcut(ui.Cmd, ui.KeyW, func() { a.settingsWin.Close() })
	}
	s := &a.settings

	ui.Scroll(c).Fill().Padding(24, 28, 32, 28).Gap(8).Children(func() {
		group := func(title string, rows func()) {
			ui.Text(c, title).FontSize(12).FontWeight(600).TextColor(p.muted).Margin(14, 4, 4, 4)
			ui.Column(c).Radius(12).Background(p.bg).Border(1, p.hairline).Dividers(1, p.hairline).Children(rows)
		}
		row := func(label, desc string, control func()) {
			ui.Row(c).Padding(12, 16).Gap(16).AlignItems(ui.Center).MinHeight(48).Children(func() {
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(3).Children(func() {
					ui.Text(c, label).FontSize(13.5).FontWeight(500)
					if desc != "" {
						ui.Text(c, desc).FontSize(12).TextColor(p.muted).LineHeight(1.4)
					}
				})
				control()
			})
		}

		// Only on macOS, the one with a notch player.
		if a.notch != nil {
			group("刘海播放器", func() {
				row("显示刘海播放器", "在屏幕顶部的刘海处显示迷你播放器，鼠标悬停即可展开", func() {
					if ui.Switch(c.Key("notch"), &s.Notch).Label("显示刘海播放器").Changed() {
						a.changed()
					}
				})
				row("播放时显示实时活动", "在刘海两侧显示封面与音频波形，可能遮挡刘海旁的菜单栏图标", func() {
					if ui.Switch(c.Key("activity"), &s.NotchActivity).Label("播放时显示实时活动").Disabled(!s.Notch).Changed() {
						a.changed()
					}
				})
				row("在无刘海的屏幕上常驻显示", "关闭时，将鼠标移至屏幕顶部中央即可唤出，不会遮挡菜单栏", func() {
					if ui.Switch(c.Key("virtual"), &s.NotchVirtual).Label("在无刘海的屏幕上常驻显示").Disabled(!s.Notch).Changed() {
						a.changed()
					}
				})
			})
		}

		group("外观", func() {
			row("外观", "", func() {
				modes := []string{"system", "light", "dark"}
				i := 0
				for k, m := range modes {
					if m == s.Appearance {
						i = k
					}
				}
				if segmented(c.Key("appearance"), &i, "跟随系统", "浅色", "深色") {
					s.Appearance = modes[i]
					a.changed()
				}
			})
			row("侧栏歌单视图", "", func() {
				i := 0
				if s.MyPlaylistGrid {
					i = 1
				}
				if segmented(c.Key("my-playlist-grid"), &i, "列表", "网格") {
					s.MyPlaylistGrid = i == 1
					a.changed()
				}
			})
		})

		group("窗口", func() {
			row("关闭窗口时退出", "关掉后音乐停止；不打开时，关闭窗口只是收起，音乐接着放", func() {
				if ui.Switch(c.Key("quit-on-close"), &s.QuitOnClose).Label("关闭窗口时退出").Changed() {
					a.changed()
				}
			})
		})

		group("全局快捷键", func() {
			for _, act := range hotkeyActions {
				desc := ""
				if a.hotkeyErr[act.id] {
					desc = "被占用了，换一个"
				}
				row(act.label, desc, func() { a.hotkeyButton(c, act.id, act.label) })
			}
		})
		ui.Text(c, "在其他应用里也能用。点按钮后按下组合键（要带 "+hotkeyLabel("Ctrl")+"、"+hotkeyLabel("Alt")+" 或 "+hotkeyLabel("Super")+"），Esc 取消").
			FontSize(12).TextColor(p.muted).LineHeight(1.4).Margin(2, 4, 0, 4)

		group("网络", func() {
			desc := "留空跟随系统代理；可填 http、https 或 socks5 代理，如 127.0.0.1:7890" + webProxyNote
			if a.proxyErr != "" {
				desc = a.proxyErr
			}
			row("代理", desc, func() {
				in := ui.TextInput(c.Key("proxy"), &a.proxyText).Placeholder("跟随系统").Label("代理地址").Width(200).Error(a.proxyErr)
				// Set as Enter is pressed or the field is left, not as each
				// key is typed, a port half written.
				if in.Submitted() || a.proxyTyping && !in.Focused() {
					a.setProxy()
				}
				a.proxyTyping = in.Focused()
			})
		})

		group("实验性功能", func() {
			row("进度条烟雾特效", "播放时进度条化作拖着烟雾的光点，随音乐起伏；会多用一些电量", func() {
				if ui.Switch(c.Key("smoke"), &s.Smoke).Label("进度条烟雾特效").Changed() {
					a.changed()
				}
			})
			row("烟雾背景", "播放页的背景化作封面色调的烟雾，缓缓升起，随低音起伏；仍在打磨，会多用一些电量", func() {
				if ui.Switch(c.Key("fog"), &s.Fog).Label("烟雾背景").Changed() {
					a.changed()
				}
			})
		})

		group("缓存", func() {
			row("歌曲缓存", "听过的歌曲（加密的）保存在本机，再次播放更快、不耗流量，但仍要联网授权；"+a.cacheUsage(), func() {
				labels := make([]string, len(cacheLimits))
				chosen := cacheLimits[2].label
				for i, l := range cacheLimits {
					labels[i] = l.label
					if l.mb == s.CacheMB {
						chosen = l.label
					}
				}
				if ui.Select(c.Key("cache"), &chosen, labels).Label("歌曲缓存").Width(150).Changed() {
					for _, l := range cacheLimits {
						if l.label == chosen {
							s.CacheMB = l.mb
						}
					}
					s.save()
					a.applyCacheLimit()
				}
			})
			row("清除缓存", "删除保存在本机的全部歌曲", func() {
				if ui.Button(c.Key("clear-cache"), "清除").Disabled(a.cacheUsed <= 0).Clicked() {
					a.clearCache()
				}
			})
		})

		group("账号", func() {
			if a.profile == nil {
				row("未登录", "登录 Spotify 后才能听歌，免费账号也能完整播放", func() {
					if ui.PrimaryButton(c.Key("login"), "登录").Clicked() {
						a.showMain()
						a.openLogin()
						a.win.Invalidate()
					}
				})
				return
			}
			prof := a.profile
			ui.Row(c).Padding(12, 16).Gap(12).AlignItems(ui.Center).Children(func() {
				a.profileAvatar(c, prof, 36)
				ui.Column(c).Grow(1).Gap(2).Children(func() {
					ui.Text(c, prof.Nickname).FontSize(13.5).FontWeight(600)
					ui.Text(c, cmp.Or(prof.Email, "已登录 Spotify")).FontSize(12).TextColor(p.muted)
				})
				if ui.Button(c.Key("logout"), "退出登录").Clicked() {
					a.signOut()
				}
			})
		})

		group("关于", func() { a.aboutRows(c, row) })
	})
}
