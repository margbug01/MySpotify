package qtweb

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"vinyl/internal/l10n"
)

//go:embed host.py
var hostScript []byte

func enabled() bool {
	if backend := os.Getenv("MYSPOTIFY_LINUX_WEBVIEW"); backend != "" {
		return backend == "qt"
	}
	return runtime.GOARCH == "amd64" && os.Getenv("MYSPOTIFY_LINUX_CHROME") != "1"
}

func hostCommand() (*exec.Cmd, error) {
	if runtime.GOARCH != "amd64" {
		return nil, errors.New(l10n.T("Qt WebEngine 音频后端目前仅支持已安装官方 Widevine 的 Linux amd64"))
	}
	flatpak, err := exec.LookPath("flatpak")
	if err != nil {
		return nil, errors.New(l10n.T("缺少 Qt WebEngine 运行时，请先运行 Linux 音频组件安装脚本"))
	}
	cdm := "/opt/google/chrome/WidevineCdm/_platform_specific/linux_x64/libwidevinecdm.so"
	if _, err := os.Stat(cdm); err != nil {
		return nil, errors.New(l10n.T("缺少官方 Widevine 组件，请先安装 Google Chrome；播放时不会启动 Chrome 浏览器"))
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	profile, err := ProfileDir()
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(hostScript)
	scriptDir := filepath.Join(cache, "MySpotify", "qt-webengine", hex.EncodeToString(digest[:8]))
	for _, dir := range []string{scriptDir, profile} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, err
		}
		if err := os.Chmod(dir, 0700); err != nil {
			return nil, err
		}
	}
	script := filepath.Join(scriptDir, "host.py")
	kept, _ := os.ReadFile(script)
	if sha256.Sum256(kept) != digest {
		// The digest directory is owned by this user and its content is replaceable.
		if err := os.WriteFile(script, hostScript, 0600); err != nil {
			return nil, err
		}
	}
	flags := "--autoplay-policy=no-user-gesture-required --widevine-path=" + cdm
	if os.Getenv("LIBGL_ALWAYS_SOFTWARE") == "1" {
		flags += " --disable-gpu"
	}
	args := []string{
		"run", "--user", "--command=python3", "--share=network", "--share=ipc",
		"--socket=wayland", "--socket=fallback-x11", "--socket=pulseaudio", "--talk-name=org.freedesktop.Flatpak", "--talk-name=org.freedesktop.secrets",
		"--filesystem=" + scriptDir + ":ro", "--filesystem=" + profile,
		"--filesystem=" + filepath.Dir(filepath.Dir(filepath.Dir(cdm))) + ":ro",
		"--env=PYTHONPATH=/app/lib/python/site-packages", "--env=QTWEBENGINEPROCESS_PATH=/app/bin/QtWebEngineProcess",
		"--env=QTWEBENGINE_CHROMIUM_FLAGS=" + flags,
	}
	if os.Getenv("GDK_BACKEND") == "x11" {
		args = append(args, "--env=QT_QPA_PLATFORM=xcb")
	}
	if os.Getenv("LIBGL_ALWAYS_SOFTWARE") == "1" {
		args = append(args, "--env=LIBGL_ALWAYS_SOFTWARE=1", "--env=QT_QUICK_BACKEND=software")
	}
	if os.Getenv("PULSE_SERVER") == "unix:/mnt/wslg/PulseServer" {
		args = append(args, "--filesystem=/mnt/wslg:ro", "--env=PULSE_SERVER=unix:/mnt/wslg/PulseServer")
	}
	for _, name := range []string{"http_proxy", "https_proxy", "all_proxy", "no_proxy", "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY"} {
		if value := os.Getenv(name); value != "" {
			args = append(args, "--env="+name+"="+value)
		}
	}
	args = append(args, "com.riverbankcomputing.PyQt.BaseApp//6.11", script, "--profile", profile)
	return exec.Command(flatpak, args...), nil
}
