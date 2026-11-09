#!/bin/sh
# Install the tested AAC-enabled Qt WebEngine runtime for the current desktop user.
set -eu
if [ "$(uname -s)" != Linux ] || [ "$(uname -m)" != x86_64 ]; then
  printf '%s\n' '此音频组件目前只验收 Linux amd64。' >&2
  exit 1
fi
if [ "$(id -u)" = 0 ]; then
  printf '%s\n' '请以运行 MySpotify 的普通桌面用户执行；系统依赖安装会单独调用 sudo。' >&2
  exit 1
fi
cdm=/opt/google/chrome/WidevineCdm/_platform_specific/linux_x64/libwidevinecdm.so
if [ ! -r "$cdm" ]; then
  printf '%s\n' '请先安装官方 Google Chrome，以提供 Widevine 组件。播放时不会启动 Chrome 浏览器。' >&2
  exit 1
fi
if ! command -v flatpak >/dev/null 2>&1; then
  if ! command -v apt-get >/dev/null 2>&1; then
    printf '%s\n' '请先通过发行版包管理器安装 Flatpak，再运行本脚本。' >&2
    exit 1
  fi
  sudo apt-get update
  sudo apt-get install --no-install-recommends -y flatpak
fi
flatpak remote-add --user --if-not-exists flathub https://flathub.org/repo/flathub.flatpakrepo
flatpak install --user --assumeyes flathub com.riverbankcomputing.PyQt.BaseApp//6.11
printf '%s\n' 'Qt WebEngine 组件已安装。运行：./vinyl（Linux amd64 默认使用 Qt WebEngine）' '首次下载含共享运行时，约占数 GiB 磁盘空间；这不是运行内存。'
