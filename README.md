# Vinyl

Windows 上的 Spotify 播放器，Go + [MyGo](https://github.com/egoist/mygo) 原生界面，免费账号也能整首播放。

界面来自 [egoist/mygo-cloudmusic](https://github.com/egoist/mygo-cloudmusic)（一个网易云音乐客户端，AGPL-3.0），数据和播放换成了 Spotify。仓库保留了上游历史（remote `upstream`），以后可以 `git merge upstream/main` 拉作者的界面更新。

## 怎么做到的

- 登录：一个平时隐藏的窗口打开 open.spotify.com，`internal/spotify/capture.js` 记下网页播放器自己拿到的 token 和 client-token。没登录就把这个窗口显示成 Spotify 官方登录页。token 快过期时在后台重新加载一次。账号头像依次用：Spotify 上设的头像 → 账号邮箱在 Gravatar 上的头像 → 名字首字母（底色是 Spotify 给的 `avatarBackgroundColor`，和网页版一样）。邮箱是带着 WebView 的 cookie 问 `www.spotify.com/api/account-settings/v1/profile` 拿的，设置页也显示它。
- 数据：网页播放器用的 Pathfinder GraphQL（首页、排行榜、歌单、专辑、搜索、资料库、喜欢）+ spclient（歌词、播放清单）。公开 Web API 对这个身份一律 429，不用。查询哈希写死在 `internal/spotify/spotify.go` 的 `ops` 里，Spotify 更新网页播放器后可能失效（报 PersistedQueryNotFound），到时从 open.spotify.com 的 JS 包里重新取。
- 播放：另一个隐藏的 WebView 页面（`internal/player/web/player.html`）用 hls.js + Widevine 播加密的 MP4，解密在浏览器的 CDM 里。清单、分段、许可证都经本机 127.0.0.1 的小服务转发。队列、随机、循环、淡入淡出、会话恢复沿用上游。
- 播放出错：许可证 401 先换身份再要一次；429（限流）按 Retry-After 等（没给就 30 秒起翻倍，最多 5 分钟），播放栏写「Spotify 暂时限流，稍后重试」，等完自动从原位置重来；403 停在这首。页面里的网络错误（hls.js `startLoad`，清单出错就重新 `loadSource`）和媒体错误（`recoverMediaError`）各首最多重试 2 次，还不行就跳到下一首。

## 构建

```
go test ./...
CGO_ENABLED=0 go build -trimpath -ldflags="-H windowsgui -s -w" -o outputs/Vinyl.exe .
```

图标、版本信息和 manifest 在 `rsrc_windows_amd64.syso`。改了 `resources/icon.svg` 后，先渲染成 1024px 透明 PNG（Edge 无头模式：`msedge --headless=new --default-background-color=00000000 --window-size=1024,1024 --screenshot=resources/icon.png resources/icon.svg`），再重新生成：

```
go run github.com/tc-hib/go-winres@v0.3.3 simply --icon resources/icon.png --manifest gui --product-name Vinyl --product-version 0.1.0 --file-version 0.1.0 --original-filename Vinyl.exe --arch amd64 --out rsrc
```

需要系统装有 WebView2 运行时（Windows 10/11 一般自带）。数据在 `%APPDATA%\Vinyl`，WebView 登录态在 `%LOCALAPPDATA%\Vinyl\WebView2`。日志（含崩溃时的堆栈）写在 `%APPDATA%\Vinyl\vinyl.log`，超过 1MB 挪成 `vinyl.log.1` 重新开始。

## 开发

- `NM_SHOTS=<目录>`：自动走一遍首页 → 排行榜 → 歌单 → 播放 → 播放页 → 待播清单 → 搜索，每步截图（静音）。
- `NM_SHOTS=<目录> NM_SCENE=play`：静音验证播放：起播、拖到结尾自动下一首、暂停/继续、下一首，过程打到 stderr。用不带 `-H windowsgui` 编的版本看输出。
- `LIVE=<身份文件> go test -run TestLive ./internal/spotify`：用真 token 把每个接口打一遍。身份文件是 `window.__vinyl` 的 JSON。

## 和上游比少了什么

评论和间奏热评、私人 FM 和心动模式、歌单编辑（新建/重命名/删除/加歌）、音质档位、音量均衡、歌曲缓存、跨设备续播、自动更新。Spotify 没有对应的，或者暂时没接。

频谱拿不到（音频在 CDM 里解密），播放中的小音柱是按播放位置算的动画。

## 许可证

AGPL-3.0，随上游。图标是 Vinyl 自己画的（`resources/icon.svg`，「落日唱片」），`resources/icon.png` 由它渲染。`internal/player/web/hls.min.js` 是 hls.js 1.7.3（Apache-2.0）。
