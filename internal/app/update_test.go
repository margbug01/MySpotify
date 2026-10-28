package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"
)

// updateApp is newTestApp with an updater that answers found, or fails with
// fail, without the network, and runs what it would run off the main thread
// at once.
func updateApp(found *release, fail error) (*app, *int) {
	a := newTestApp()
	a.settings = defaultSettings()
	relaunched := new(int)
	a.upd = updates{
		enabled:  true,
		check:    func(context.Context) (*release, error) { return found, fail },
		relaunch: func() { *relaunched++ },
		spawn:    func(f func()) { f() },
		post:     func(f func()) { f() },
	}
	return a, relaunched
}

func settingsTester(a *app) *ui.Tester {
	tt := ui.NewTester(a.settingsView, 560, 1600)
	tt.Frame()
	return tt
}

func click(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	if err := tt.Click(label); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
}

// A new version shows over the account row; in the settings, 更新并重启
// installs it, showing how far it got, and starts it.
func TestUpdateInstalls(t *testing.T) {
	var reports []float64
	found := &release{Version: "9.9.0", Notes: "## 9.9.0\n\n- **更快**的换歌\n- 新的 `MV` 小标"}
	a, relaunched := updateApp(found, nil)
	found.install = func(_ context.Context, progress func(done, total int64)) error {
		for _, n := range []int64{0, 10, 11, 50, 100} {
			progress(n, 100)
			reports = append(reports, a.upd.progress)
		}
		return nil
	}
	main := ui.NewTester(a.view, 1000, 700)
	main.Frame()
	if main.HasText("新版本 9.9.0") {
		t.Fatal("a new version shows before any check")
	}
	a.checkUpdate(false)
	main.Frame()
	if !main.HasText("新版本 9.9.0") {
		t.Fatal("no 新版本 9.9.0 over the account row")
	}

	tt := settingsTester(a)
	for _, s := range []string{"有新版本 9.9.0", "· 更快的换歌", "· 新的 MV 小标"} {
		if !tt.HasText(s) {
			t.Fatalf("the settings show no %q", s)
		}
	}
	click(t, tt, "更新并重启")
	if want := []float64{0, 0.1, 0.11, 0.5, 1}; !reflect.DeepEqual(reports, want) {
		t.Fatalf("progress %v, want %v", reports, want)
	}
	if a.upd.stage != updateReady || *relaunched != 1 {
		t.Fatalf("stage %v, relaunched %d times", a.upd.stage, *relaunched)
	}
}

// A failed download says so and offers the update again.
func TestUpdateInstallFails(t *testing.T) {
	found := &release{Version: "9.9.0"}
	found.install = func(context.Context, func(int64, int64)) error { return errors.New("signature mismatch") }
	a, relaunched := updateApp(found, nil)
	a.checkUpdate(false)
	tt := settingsTester(a)
	click(t, tt, "更新并重启")
	if a.upd.stage != updateAvailable || *relaunched != 0 {
		t.Fatalf("stage %v, relaunched %d times", a.upd.stage, *relaunched)
	}
	if !tt.HasText("更新没装上：连不上 GitHub，看看网络或系统代理") || !tt.HasText("更新并重启") {
		t.Fatal("the failure is not told, or the update not offered again")
	}
}

// Asking finds it up to date, or says why it could not tell; a check of
// the app's own that fails says nothing.
func TestUpdateCheck(t *testing.T) {
	a, _ := updateApp(nil, nil)
	tt := settingsTester(a)
	click(t, tt, "检查更新")
	if !tt.HasText("已是最新版本") || !a.upd.latest || a.upd.checked.IsZero() {
		t.Fatal("not up to date after checking")
	}

	a, _ = updateApp(nil, context.DeadlineExceeded)
	a.checkUpdate(false)
	if a.upd.err != "" {
		t.Fatalf("a check of its own told: %q", a.upd.err)
	}
	tt = settingsTester(a)
	click(t, tt, "检查更新")
	if !tt.HasText("检查更新失败：连接 GitHub 超时") {
		t.Fatal("no 检查更新失败：连接 GitHub 超时")
	}

	// A version found before stays found when a later check fails.
	found := &release{Version: "9.9.0"}
	a, _ = updateApp(found, nil)
	a.checkUpdate(false)
	a.upd.check = func(context.Context) (*release, error) { return nil, errors.New("offline") }
	a.checkUpdate(false)
	if a.upd.stage != updateAvailable || a.upd.found != found {
		t.Fatalf("stage %v, found %v after a failed check", a.upd.stage, a.upd.found)
	}
}

// A copy that cannot update itself points to the releases, and checks
// nothing.
func TestUpdateDisabled(t *testing.T) {
	a, _ := updateApp(&release{Version: "9.9.0"}, nil)
	a.upd.enabled = false
	a.checkUpdate(true)
	a.watchUpdates()
	if a.upd.stage != updateIdle || !a.upd.checked.IsZero() {
		t.Fatal("a copy that cannot update checked")
	}
	tt := settingsTester(a)
	for _, s := range []string{"这一份不能自己更新", "打开下载页"} {
		if !tt.HasText(s) {
			t.Fatalf("no %q", s)
		}
	}
	if tt.HasText("自动检查更新") {
		t.Fatal("自动检查更新 shows where nothing updates")
	}
}

// 自动检查更新 is saved as it is switched, and on by default, also for
// settings kept before there was one.
func TestAutoUpdateSetting(t *testing.T) {
	a, _ := updateApp(nil, nil)
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	tt := settingsTester(a)
	// The switch, at the end of its row: its label is the row's too.
	label, ok1 := tt.Find("自动检查更新")
	desc, ok2 := tt.Find("打开时和之后每 12 小时问一次 GitHub，只提示，不自己装")
	if !ok1 || !ok2 {
		t.Fatal("no row for 自动检查更新")
	}
	tt.ClickAt(560-28-16-18, (label.Y+desc.Y+desc.H)/2)
	tt.Frame()
	b, err := os.ReadFile(a.settings.path)
	if err != nil {
		t.Fatal(err)
	}
	var saved settings
	if err := json.Unmarshal(b, &saved); err != nil || saved.AutoUpdate {
		t.Fatalf("saved AutoUpdate %v, %v", saved.AutoUpdate, err)
	}

	old := defaultSettings()
	if err := json.Unmarshal([]byte(`{"cacheMB": 512, "volume": 0.5}`), &old); err != nil || !old.AutoUpdate {
		t.Fatalf("settings of 0.3.0 load with AutoUpdate %v, %v", old.AutoUpdate, err)
	}
}

func TestNoteLines(t *testing.T) {
	got := noteLines("## 0.4.0\n\n- 不登录也能**逛**\n* `⌘,` 打开设置\n\n一段话\n- 四\n- 五", 4)
	want := []string{"· 不登录也能逛", "· ⌘, 打开设置", "一段话", "…"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}
