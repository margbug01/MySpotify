package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// MySpotify updates itself with MyGo's signed updates. mygo build signs each
// platform's archive and manifest with the release key, and they go up with
// the GitHub release; mygo.json names the repository and holds the public
// key. The app reads its platform's manifest in the latest release, and
// downloads, checks and swaps in a newer version, then starts it. A build of
// go build, or a copy where it cannot write (still in the disk image), can
// only point to the releases page.

const releasesPage = "https://github.com/margbug01/MySpotify/releases"

type updateStage int

const (
	updateIdle updateStage = iota // not asked yet, or up to date
	updateChecking
	updateAvailable
	updateDownloading
	updateReady // the new version is in place, to start
)

// release is a newer version, and how to install it.
type release struct {
	Version, Notes string
	install        func(ctx context.Context, progress func(done, total int64)) error
}

// updates is where updating is at. Like the rest of app it belongs to the
// main thread: checks and downloads run with spawn and come back with post.
type updates struct {
	enabled  bool
	check    func(context.Context) (*release, error) // nil when up to date
	relaunch func()
	spawn    func(func()) // runs off the main thread
	post     func(func()) // runs on it

	stage    updateStage
	found    *release
	progress float64 // of the download, 0–1
	err      string  // why the last check or download the listener asked for failed
	detail   string  // and what it said
	checked  time.Time
	latest   bool // the last check found nothing newer
}

// myGoUpdates is the updater of a build of mygo build.
func myGoUpdates(post func(func())) updates {
	return updates{
		enabled: mygo.Updater.Enabled(),
		check: func(ctx context.Context) (*release, error) {
			u, err := mygo.Updater.Check(ctx)
			if err != nil || u == nil {
				return nil, err
			}
			return &release{Version: u.Version, Notes: u.Notes, install: u.Install}, nil
		},
		relaunch: mygo.App.Relaunch,
		spawn:    func(fn func()) { go fn() },
		post:     post,
	}
}

// postSettings is post, with the settings window drawn again too: what
// updating is at shows there, and post only draws the main window and the
// notch.
func (a *app) postSettings(fn func()) {
	a.post(func() {
		fn()
		if a.settingsWin != nil {
			a.settingsWin.Invalidate()
		}
	})
}

// checkUpdate asks whether there is a newer version. manual is the listener
// asking, who is told why it failed; a check of the app's own fails quietly.
func (a *app) checkUpdate(manual bool) {
	u := &a.upd
	if !u.enabled || u.stage == updateChecking || u.stage == updateDownloading || u.stage == updateReady {
		return
	}
	before, check := u.stage, u.check
	u.stage, u.err, u.detail = updateChecking, "", ""
	u.spawn(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		found, err := check(ctx)
		u.post(func() {
			u.checked = time.Now()
			switch {
			case err != nil:
				u.stage = before // a version found before is still there
				if manual {
					u.err, u.detail = "检查更新失败："+updateError(err), err.Error()
				}
			case found == nil:
				u.stage, u.found, u.latest = updateIdle, nil, true
			default:
				u.stage, u.found, u.latest = updateAvailable, found, false
			}
		})
	})
}

// installUpdate downloads the update, checks its signature and puts it in
// place of this copy on the disk, then starts it.
func (a *app) installUpdate() {
	u := &a.upd
	found := u.found
	if u.stage != updateAvailable || found == nil {
		return
	}
	u.stage, u.progress, u.err, u.detail = updateDownloading, 0, "", ""
	u.spawn(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		shown := -1
		err := found.install(ctx, func(done, total int64) {
			if total <= 0 {
				return
			}
			if pct := int(done * 100 / total); pct != shown { // a frame a percent, not a chunk
				shown = pct
				u.post(func() {
					if u.stage == updateDownloading {
						u.progress = float64(pct) / 100
					}
				})
			}
		})
		u.post(func() {
			if err != nil {
				u.stage, u.err, u.detail = updateAvailable, "更新没装上："+updateError(err), err.Error()
				return
			}
			u.stage = updateReady
			// Quitting keeps the settings, where playback was, and closes
			// the player's page (OnBeforeQuit), as quitting from the menu
			// does; the new version picks up from there.
			u.relaunch()
		})
	})
}

// watchUpdates checks a while after opening and every 12 hours after,
// unless the settings say not to.
func (a *app) watchUpdates() {
	u := &a.upd
	if !u.enabled {
		return
	}
	u.spawn(func() {
		time.Sleep(10 * time.Second)
		for {
			u.post(func() {
				if a.settings.AutoUpdate {
					a.checkUpdate(false)
				}
			})
			time.Sleep(12 * time.Hour)
		}
	})
}

// updateError says why, for people.
func updateError(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "连接 GitHub 超时"
	case strings.Contains(err.Error(), "404"):
		return "GitHub 上的最新版本还没有更新包"
	}
	return "连不上 GitHub，看看网络或系统代理"
}

// noteLines puts the Markdown release notes in a few lines of plain text:
// no headings or emphasis, items as bullets, the rest cut.
func noteLines(md string, max int) []string {
	var out []string
	for _, line := range strings.Split(md, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		for _, bullet := range []string{"- ", "* "} {
			if rest, ok := strings.CutPrefix(line, bullet); ok {
				line = "· " + rest
			}
		}
		out = append(out, strings.NewReplacer("**", "", "`", "").Replace(line))
	}
	if max > 0 && len(out) > max {
		out = append(out[:max-1], "…")
	}
	return out
}

// appVersion is the version of mygo.json, which go build leaves out.
func appVersion() string {
	if v := mygo.App.Version(); v != "" {
		return v
	}
	return "开发版"
}

// aboutRows are the settings' 关于: the version, and updating. row is the
// settings' row: a label, what it says below it, and its control.
func (a *app) aboutRows(c *ui.Context, row func(label, desc string, control func())) {
	p := a.pal
	u := &a.upd
	row(appName+" "+appVersion(), "Windows 和 macOS 上的 Spotify 播放器，和 Spotify 没有关系", func() {
		ui.Link(c, "GitHub", "https://github.com/margbug01/MySpotify")
	})
	switch {
	case !u.enabled:
		row("这一份不能自己更新", "自己编译的，或者放在没有写入权限的地方（比如还在 dmg 里）。新版本到 GitHub 下载", func() {
			ui.Link(c, "打开下载页", releasesPage)
		})
	case u.stage == updateAvailable:
		row("有新版本 "+u.found.Version, "当前 "+appVersion()+"，更新时 MySpotify 会重新打开一下", func() {
			if ui.PrimaryButton(c.Key("install-update"), "更新并重启").Clicked() {
				a.installUpdate()
			}
		})
		if lines := noteLines(u.found.Notes, 8); len(lines) > 0 {
			ui.Column(c).Padding(10, 16).Gap(4).Children(func() {
				for _, line := range lines {
					ui.Text(c, line).FontSize(12).TextColor(p.muted).LineHeight(1.4)
				}
			})
		}
	case u.stage == updateDownloading:
		row("正在下载 "+u.found.Version, fmt.Sprintf("%d%%，装好会自己重新打开", int(u.progress*100)), func() {
			ui.Progress(c, u.progress).Width(140)
		})
	case u.stage == updateReady:
		row(u.found.Version+" 已经装好", "重新打开 MySpotify 后生效", func() {
			if ui.PrimaryButton(c.Key("restart-update"), "立即重启").Clicked() {
				u.relaunch()
			}
		})
	default: // not asked, up to date, asking
		title, desc := "检查新版本", "还没检查过"
		if u.latest {
			now := time.Now()
			at := u.checked.Format("15:04")
			if y, m, d := u.checked.Date(); y != now.Year() || m != now.Month() || d != now.Day() {
				at = u.checked.Format("01-02 15:04")
			}
			title, desc = "已是最新版本", at+" 检查过"
		}
		row(title, desc, func() {
			label := "检查更新"
			if u.stage == updateChecking {
				label = "检查中…"
			}
			if ui.Button(c.Key("check-update"), label).Disabled(u.stage == updateChecking).Clicked() {
				a.checkUpdate(true)
			}
		})
	}
	if u.err != "" {
		ui.Text(c, u.err).FontSize(12).TextColor(ui.RGB(229, 72, 77)).Padding(10, 16).Tooltip(u.detail)
	}
	if u.enabled {
		row("自动检查更新", "打开时和之后每 12 小时问一次 GitHub，只提示，不自己装", func() {
			if ui.Switch(c.Key("auto-update"), &a.settings.AutoUpdate).Label("自动检查更新").Changed() {
				a.settings.save()
				if a.settings.AutoUpdate {
					a.checkUpdate(false)
				}
			}
		})
	}
}

// updateBadge sits over the account row when there is a new version, and
// opens the settings, where it is installed.
func (a *app) updateBadge(c *ui.Context) {
	u := &a.upd
	if u.found == nil || u.stage != updateAvailable && u.stage != updateDownloading && u.stage != updateReady {
		return
	}
	p := a.pal
	text := "新版本 " + u.found.Version
	b := ui.ButtonBase(c.Key("update-badge")).Height(30).PaddingX(10).Radius(9).AlignItems(ui.Center).Justify(ui.Start).
		Label(text).Tooltip("打开设置，更新 MySpotify").FocusRing(false)
	if b.Hovered() {
		b.Background(p.hover)
	}
	b.Children(func() {
		ui.Box(c).Size(7, 7).Radius(3.5).Background(p.accent).Shrink(0)
		ui.Text(c, text).FontSize(12.5).FontWeight(600).TextColor(p.accent).SingleLine().Margin(0, 0, 0, 8)
	})
	if b.Clicked() {
		a.openSettings()
	}
}
