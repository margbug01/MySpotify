package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"

	"vinyl/internal/l10n"
	"vinyl/internal/player"
)

// cacheLimits are the sizes the cache of tracks may take, in megabytes.
var cacheLimits = []struct {
	mb    int
	label string
}{
	{0, "关闭缓存"},
	{512, "512 MB"},
	{1024, "1 GB"},
	{2048, "2 GB"},
	{5120, "5 GB"},
}

// newPartCache returns the cache of the parts of tracks played, in dir or,
// with none chosen, the app's cache directory; nil without one.
func newPartCache(mb int, dir string) *player.Cache {
	if dir == "" {
		dir = defaultCacheDir()
	}
	if dir == "" {
		return nil
	}
	return player.NewCache(dir, int64(mb)<<20)
}

// defaultCacheDir is where the parts are kept unless the listener chose a
// folder: %LOCALAPPDATA%\Vinyl\tracks on Windows.
func defaultCacheDir() string {
	dir, err := mygo.App.Path(mygo.PathCache)
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "tracks")
}

// chooseCacheDir asks for the folder to keep the parts in, and moves the
// cache there.
func (a *app) chooseCacheDir() {
	if a.partCache() == nil {
		return
	}
	go func() {
		// Open blocks until the listener chooses, waiting on the main thread.
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: a.win, Title: l10n.T("选择缓存位置"), Directory: true, CreateDirectories: true,
			DefaultPath: a.partCache().Dir(),
		})
		if err != nil || len(paths) == 0 {
			return
		}
		a.post(func() { a.moveCache(paths[0]) })
	}()
}

// moveCache keeps the parts in dir from now on, "" for the default, if
// the app can write there; those kept so far are dropped.
func (a *app) moveCache(dir string) {
	cache := a.partCache()
	if cache == nil {
		return
	}
	to := dir
	if to == "" {
		to = defaultCacheDir()
	}
	if to == "" || filepath.Clean(to) == filepath.Clean(cache.Dir()) {
		return
	}
	if err := writable(to); err != nil {
		a.cacheDirErr = l10n.T("这个文件夹写不进去，换一个")
		return
	}
	a.cacheDirErr = ""
	a.settings.CacheDir = dir
	a.settings.save()
	a.cacheUsed = -1
	go func() {
		cache.Move(to)
		a.measureCache()
	}()
}

// writable reports whether files can be made in dir, making it if need be.
func writable(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "*.tmp")
	if err != nil {
		return err
	}
	f.Close()
	return os.Remove(f.Name())
}

func (a *app) partCache() *player.Cache {
	if a.pl == nil {
		return nil
	}
	return a.pl.Cache
}

// applyCacheLimit gives the cache the limit the settings say.
func (a *app) applyCacheLimit() {
	if cache := a.partCache(); cache != nil {
		limit := int64(a.settings.CacheMB) << 20
		go func() {
			cache.SetLimit(limit)
			a.measureCache()
		}()
	}
}

// measureCache finds how much the cache takes, on a goroutine, for the
// settings to show.
func (a *app) measureCache() {
	cache := a.partCache()
	if cache == nil {
		return
	}
	go func() {
		n := cache.Size()
		a.post(func() { a.cacheUsed = n })
	}()
}

func (a *app) clearCache() {
	if cache := a.partCache(); cache != nil {
		a.cacheUsed = 0
		go func() {
			cache.Clear()
			a.measureCache()
		}()
	}
}

// cacheUsage says how much the cache takes, once measured.
func (a *app) cacheUsage() string {
	switch n := a.cacheUsed; {
	case n < 0:
		return l10n.T("正在计算已用空间…")
	case n >= 1<<30:
		return fmt.Sprintf(l10n.T("已用 %.1f GB"), float64(n)/(1<<30))
	default:
		return fmt.Sprintf(l10n.T("已用 %d MB"), n>>20)
	}
}
