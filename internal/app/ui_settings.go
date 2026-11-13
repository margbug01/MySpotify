package app

import (
	"cmp"
	"fmt"

	"github.com/egoist/mygo/ui"

	"vinyl/internal/l10n"
)

// openSettings goes to the settings page. The main window comes up first,
// for the menu's sake: its accelerators reach the app while it lies hidden.
func (a *app) openSettings() {
	a.showMain()
	a.open(route{page: pageSettings})
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

// settingsPage is the settings, a page of the main window like the others.
// The theme and the palette are the window's, which view set.
func (a *app) settingsPage(c *ui.Context) {
	p := a.pal
	s := &a.settings

	// Tinted as the window the settings had to themselves, so their cards
	// stand from the page as they did there.
	top, bottom := a.insets(c)
	ui.Scroll(c.Key("settings")).TrackScroll(&a.pageScroll).Grow(1).
		Padding(top+pageGap, pagePad, bottom+48, pagePad).ScrollbarInsets(top, 0, bottom, 0).
		Background(p.sidebar).Gap(8).Children(func() {
		a.startMark(c)
		defer a.endMark(c)
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

		group(l10n.T("账号"), func() {
			if a.profile == nil {
				row(l10n.T("未登录"), l10n.T("登录 Spotify 后才能听歌，免费账号也能完整播放"), func() {
					// In the main window already, which the login
					// window opens over.
					if ui.PrimaryButton(c.Key("login"), l10n.T("登录")).Clicked() {
						a.openLogin()
					}
				})
				return
			}
			prof := a.profile
			ui.Row(c).Padding(12, 16).Gap(12).AlignItems(ui.Center).Children(func() {
				a.profileAvatar(c, prof, 36)
				ui.Column(c).Grow(1).Gap(2).Children(func() {
					ui.Text(c, prof.Nickname).FontSize(13.5).FontWeight(600)
					ui.Text(c, cmp.Or(prof.Email, l10n.T("已登录 Spotify"))).FontSize(12).TextColor(p.muted)
				})
				if ui.Button(c.Key("logout"), l10n.T("退出登录")).Clicked() {
					a.signOut()
				}
			})
		})

		// Only on macOS, the one with a notch player.
		if a.notch != nil {
			group(l10n.T("刘海播放器"), func() {
				row(l10n.T("显示刘海播放器"), l10n.T("在屏幕顶部的刘海处显示迷你播放器，鼠标悬停即可展开"), func() {
					if ui.Switch(c.Key("notch"), &s.Notch).Label(l10n.T("显示刘海播放器")).Changed() {
						a.changed()
					}
				})
				row(l10n.T("播放时显示实时活动"), l10n.T("在刘海两侧显示封面与音频波形，可能遮挡刘海旁的菜单栏图标"), func() {
					if ui.Switch(c.Key("activity"), &s.NotchActivity).Label(l10n.T("播放时显示实时活动")).Disabled(!s.Notch).Changed() {
						a.changed()
					}
				})
				row(l10n.T("在无刘海的屏幕上常驻显示"), l10n.T("关闭时，将鼠标移至屏幕顶部中央即可唤出，不会遮挡菜单栏"), func() {
					if ui.Switch(c.Key("virtual"), &s.NotchVirtual).Label(l10n.T("在无刘海的屏幕上常驻显示")).Disabled(!s.Notch).Changed() {
						a.changed()
					}
				})
			})
		}

		group(l10n.T("外观"), func() {
			row(l10n.T("语言"), l10n.T("立即切换界面语言"), func() {
				labels := []string{l10n.T("跟随系统"), "简体中文", "English", "Italiano"}
				tags := []string{"", "zh-Hans", "en", "it"}
				chosen := labels[0]
				for i, tag := range tags {
					if tag == s.Language {
						chosen = labels[i]
					}
				}
				if ui.Select(c.Key("language"), &chosen, labels).Label(l10n.T("界面语言")).Width(160).Changed() {
					for i, label := range labels {
						if label == chosen {
							a.chooseLanguage(tags[i])
							break
						}
					}
				}
			})
			row(l10n.T("外观"), "", func() {
				modes := []string{"system", "light", "dark"}
				i := 0
				for k, m := range modes {
					if m == s.Appearance {
						i = k
					}
				}
				if segmented(c.Key("appearance"), &i, l10n.T("跟随系统"), l10n.T("浅色"), l10n.T("深色")) {
					s.Appearance = modes[i]
					a.changed()
				}
			})
			row(l10n.T("侧栏歌单视图"), "", func() {
				i := 0
				if s.MyPlaylistGrid {
					i = 1
				}
				if segmented(c.Key("my-playlist-grid"), &i, l10n.T("列表"), l10n.T("网格")) {
					s.MyPlaylistGrid = i == 1
					a.changed()
				}
			})
		})

		group(l10n.T("窗口"), func() {
			row(l10n.T("关闭窗口时退出"), l10n.T("关掉后音乐停止；不打开时，关闭窗口只是收起，音乐接着放"), func() {
				if ui.Switch(c.Key("quit-on-close"), &s.QuitOnClose).Label(l10n.T("关闭窗口时退出")).Changed() {
					a.changed()
				}
			})
		})

		group(l10n.T("全局快捷键"), func() {
			for _, act := range hotkeyActions {
				desc := ""
				if a.hotkeyErr[act.id] {
					desc = l10n.T("被占用了，换一个")
				}
				row(l10n.T(act.label), desc, func() { a.hotkeyButton(c, act.id, l10n.T(act.label)) })
			}
		})
		ui.Text(c, fmt.Sprintf(l10n.T("快捷键说明"), hotkeyLabel("Ctrl"), hotkeyLabel("Alt"), hotkeyLabel("Super"))).
			FontSize(12).TextColor(p.muted).LineHeight(1.4).Margin(2, 4, 0, 4)

		group(l10n.T("网络"), func() {
			desc := l10n.T("留空跟随系统代理；可填 http、https 或 socks5 代理，如 127.0.0.1:7890") + l10n.T(webProxyNote)
			if a.proxyErr != "" {
				desc = a.proxyErr
			}
			row(l10n.T("代理"), desc, func() {
				in := ui.TextInput(c.Key("proxy"), &a.proxyText).Placeholder(l10n.T("跟随系统")).Label(l10n.T("代理地址")).Width(200).Error(a.proxyErr)
				// Set as Enter is pressed or the field is left, not as each
				// key is typed, a port half written.
				if in.Submitted() || a.proxyTyping && !in.Focused() {
					a.setProxy()
				}
				a.proxyTyping = in.Focused()
			})
		})

		group(l10n.T("实验性功能"), func() {
			row(l10n.T("进度条烟雾特效"), l10n.T("播放时进度条化作拖着烟雾的光点，随音乐起伏；会多用一些电量"), func() {
				if ui.Switch(c.Key("smoke"), &s.Smoke).Label(l10n.T("进度条烟雾特效")).Changed() {
					a.changed()
				}
			})
			row(l10n.T("烟雾背景"), l10n.T("播放页的背景化作封面色调的烟雾，缓缓升起，随低音起伏；仍在打磨，会多用一些电量"), func() {
				if ui.Switch(c.Key("fog"), &s.Fog).Label(l10n.T("烟雾背景")).Changed() {
					a.changed()
				}
			})
		})

		group(l10n.T("缓存"), func() {
			row(l10n.T("歌曲缓存"), l10n.T("听过的歌曲（加密的）保存在本机，再次播放更快、不耗流量，但仍要联网授权；")+a.cacheUsage(), func() {
				labels := make([]string, len(cacheLimits))
				chosen := l10n.T(cacheLimits[2].label)
				for i, l := range cacheLimits {
					labels[i] = l10n.T(l.label)
					if l.mb == s.CacheMB {
						chosen = l10n.T(l.label)
					}
				}
				if ui.Select(c.Key("cache"), &chosen, labels).Label(l10n.T("歌曲缓存")).Width(150).Changed() {
					for _, l := range cacheLimits {
						if l10n.T(l.label) == chosen {
							s.CacheMB = l.mb
						}
					}
					s.save()
					a.applyCacheLimit()
				}
			})
			row(l10n.T("清除缓存"), l10n.T("删除保存在本机的全部歌曲"), func() {
				if ui.Button(c.Key("clear-cache"), l10n.T("清除")).Disabled(a.cacheUsed <= 0).Clicked() {
					a.clearCache()
				}
			})
		})

		group(l10n.T("关于"), func() { a.aboutRows(c, row) })
	})
}
