# 开发说明

**简体中文** · [English](DEVELOPMENT.en.md)

MySpotify 的实现细节、构建、发版和调试。普通使用看 [README](README.md) 就够了。

## 怎么做到的

- **登录**：一个平时隐藏的窗口打开 open.spotify.com，`internal/spotify/capture.js` 记下网页播放器自己拿到的 token 和 client-token；没登录就把这个窗口显示成 Spotify 的登录页。登录后把这个窗口的 cookie 加密存一份（`internal/app/account_store.go`），token 快过期时直接拿它和一次性 TOTP 码问 `/api/token`，跟网页播放器自己续期一样，不用再开窗口；cookie 失效了才退回去重新加载那个窗口。不登录时同样问 `/api/token`，拿到的是访客身份。
- **数据**：Pathfinder GraphQL 的持久化查询（哈希在 `internal/spotify/spotify.go` 的 `ops` 里）加 spclient（歌词、播放清单、歌单改动、电台、最近播放、个人主页）。公开 Web API 对这个身份一律 429，不用。哈希失效时（412「Invalid query hash」）`internal/spotify/ops.go` 重读主脚本和按需加载的分块脚本，换上新哈希重试一次，存到 `%APPDATA%\Vinyl\queries.json`，10 分钟内不重复读。收藏列表在切回窗口时（5 秒内不重复）和每 30 秒检查一次，网络失败时保留上次的。
- **播放**：另一个隐藏的 WebView 页面（`internal/player/web/player.html`）播加密的 MP4。Windows 用 hls.js + Widevine；macOS 照 Safari 版网页播放器，要 cbcs 格式的文件，用 WebKit 原生 HLS + FairPlay（`skd://` 加文件 ID，`fairplay-license/v1`）。清单、分段和许可证请求都经本机 127.0.0.1 的小服务转发，分段在这里顺手存进缓存，键是 CDN 路径加字节范围（CDN 的签名每天变，不进键）。播客的单集能走这条路就走这条路，走不了的见下面「播客」。这个页面第一次真放歌时才建，闲置 5 分钟就关掉，下次放再从原位置接上；它不用 GPU，主界面照常用 GPU。
- **出错**：许可证 401 换身份再要一次；429 按 Retry-After 等（没给就从 30 秒起翻倍），等完从原位置重来；403 停在这首。页面里的网络错误（`startLoad`，清单出错就重新 `loadSource`）和媒体错误（`recoverMediaError`）每首各重试 2 次，不行再跳下一首。结束事件没送到时，音频停在最后 0.25 秒、尾部已缓冲完、连续约 4 秒不动，才算播完。
- **歌词**：Spotify 的同步歌词 → 网易云（`/api/cloudsearch/pc` 按歌名和主艺人搜，取同名、时长差不超过 2 秒里最接近的一首，它的时长和 Spotify 基本精确到毫秒；文字是简体）→ LRCLIB 精确查询（歌名、主艺人、专辑、时长都要对上）→ LRCLIB 搜索同名同歌手、时长差 2 秒内带时间轴的那条 → 都没有时间轴才用纯文本。Connect 给的歌手名缺失或是外文拼写（Yitai Wang）时，先换成 Spotify 给的中文名（王以太）再查。站点忙或限流（LRCLIB 503、网易云 405/406）时隔 3 秒、10 秒各再试一次，播客不查。外语歌拿到带时间的歌词后，再问一次网易云（`tv=-1`）：它有中文翻译就换用它的原文 + 翻译（两边逐行对得上）。中文歌词按 OpenCC 单字表（`internal/lyrics/TSCharacters.txt`）转成简体，系统首选中文是繁体（zh-Hant/TW/HK）的不转。
- **Connect**：协议结构参考 [Flutify](https://github.com/is-hp-is-mad/Flutify)，不用 Premium 才能用的 Web Playback SDK。两条 Dealer 长连接：`connect-state` 用一个隐藏的观察者设备看和遥控，`track-playback` 把本机注册成能放歌的设备并处理状态机。状态机只带当前和下一首，所以待播清单按 connect-state 的 `next_tracks` 显示（隐藏的循环副本不显示，只有 URI 的曲目用 `decorateContextTracks` 补上名字）；点其中一首发 `skip_next` 带那首的 uid。「下一首播放」「添加到待播清单」发 `add_to_queue`，排进 Spotify 的待播队列（已有手动排队的歌时排在它们后面）。电台续队还没接进 Connect。
- **MV**：`track-playback/v1/media` 多要一种 `manifest_ids_video`，返回里的 `video_version_uri` 就是这首的 MV。视频清单转成 HLS，在主窗口里一个看得见的 WebView 里放（Windows Widevine、macOS FairPlay），改放 MV 自带的音轨。原生控件盖在视频上：一层透明的框接鼠标，控制栏和字幕都画在 WebView 上面。歌单、专辑、艺人页、喜欢的歌的数据里本来就有 `associationsV3.videoAssociations`，列表的「MV」小标靠它，不用多发请求。
- **播客**：单集也走 `track-playback/v1/media`，多要一种 `file_urls_external`。平台能放的加密音频优先；macOS 上 Spotify 几乎不给单集 cbcs（只有给 Widevine 的 cenc），这时视频播客改放视频清单里的音轨——视频播客的视频不加密（H.264 + AAC 的 fMP4），两个平台都不用许可证，切到视频时时间轴完全一致，截图也不黑；纯音频的改放 `file_urls_external` 给的播客原地址（RSS 里的那个 MP3），经本机小服务按范围转发，跟完一次跳转就记住最后的地址（BBC 的要跳 3 次，每次两三秒）。只剩 Spotify 自己托管、只有 Widevine 加密音频的节目在 macOS 上放不了，会提示并停下，不再一集一集往下试。字幕是 `transcript-read-along/v2/episode/<id>` 给的逐字稿（网页播放器取逐字稿也是这个接口）：每句带开始时间，每个词带开始时间和字数；说话人、章节标题和「Music playing」不算一句；中文稿每两个字之间都有空格，显示时去掉。单集的逐字稿常有几千句，正在播放页只建看得见的几十行，其余用量过的行高（没量过的按文字长度估）占位。
- **头像**：账号的头像依次用 Spotify 上设的、账号邮箱在 Gravatar 上的、名字首字母（底色用 Spotify 给的颜色）。邮箱是带着 WebView 的 cookie 问 `www.spotify.com/api/account-settings/v1/profile` 拿的。

## 界面语言

文案在 `internal/l10n/zh-Hans.json`、`en.json` 和 `it.json`，通过 MyGo 的 `i18n.Catalog` 读取；应用自己写的文案用 `l10n.T`，带数量的消息用 `l10n.Count` 的 ICU 单复数。歌曲名、用户歌单名等内容不要送进翻译。语言选择保存在设置中，交给 `mygo.App.SetLocale` 应用；`MYGO_LOCALE=en` 可临时预览英文，不改保存的语言。

新增或修改文案时同步所有目录并保留格式参数；加新语言时还要加进设置里的语言选项、`settings.go` 的校验和 `mygo.json` 的 `CFBundleLocalizations`。`go test ./internal/l10n` 检查参数和数量格式，`go test ./internal/app -run TestLanguageSelection -count=5` 验证语言选择；`NM_FRAMES=<目录> go test ./internal/app -run TestEnglishFrames` 保存普通与最窄窗口的实际渲染。

## 从源码构建

需要 Go 1.27.3 以上，不需要 cgo。

```
go test ./...
CGO_ENABLED=0 go build -trimpath -ldflags="-H windowsgui -s -w" -o outputs/MySpotify.exe .
```

macOS 的 app 和 dmg 用 MyGo 自带的打包命令（在 Mac 上跑，产物在 `build.noindex/darwin-arm64/`，加 `-skip-dmg` 只出 app）：

```
go tool mygo build
```

图标都由 `tools/icons.py` 画（Windows，要 Pillow，用 Edge 无头模式渲染 SVG）：`resources/icon.svg`/`icon.png` 是程序图标，`icon-small.svg` 是 16–24 像素用的简化版（两道纹、更粗），`icon.ico` 把两者按尺寸合在一起；`internal/app/tray.png` 是托盘和 Mac 菜单栏的模板图标（只有黑色和透明，Windows 深色任务栏上自动画成白色，macOS 自己染色）；`AppIcon.icon` 是 Mac Dock 用的 Icon Composer 文档（浅色/深色两种底，`mygo.json` 的 `macos.icon`，要在装了 Xcode 26 的 Mac 上 `mygo build` 才会编进去，否则退回 `icon.png` 生成的 `.icns`）。改了图标或版本号，先跑 `python tools/icons.py`，再重新生成图标、版本信息和 manifest 所在的 `rsrc_windows_amd64.syso`（版本号同时改 `mygo.json`）：

```
go run github.com/tc-hib/go-winres@v0.3.3 simply --icon resources/icon.ico --manifest gui --product-name MySpotify --file-description MySpotify --product-version 0.4.26 --file-version 0.4.26 --original-filename MySpotify.exe --arch amd64 --out rsrc
```

### 发版

macOS arm64 / amd64、Windows amd64 和 Linux amd64 在 Mac 上一次打出来。Intel Mac 为实验支持，首次构建已通过交叉编译和安装包检查，尚待真机验证登录、音乐 / MV 播放和应用内更新；Linux 也为实验支持，依赖安装与验收范围见 [LINUX.md](LINUX.md)。应用内更新只认用更新私钥签过名的包，所以发版必须用 `mygo build` 并带上私钥；`go build` 出来的版本不能自己更新。

1. 改了 `go.mod` 的话，跟着改 `licenses/`（第三方许可证，随程序发出去）和里面的 `README.txt`；漏了的话 `go test` 会提醒。
2. 改版本号：`mygo.json` 的 `version`，上面 go-winres 命令里的两个版本号（重新生成 syso）。「设置 › 关于」显示的版本直接读 `mygo.json`。
3. 在 [`CHANGELOG.md`](CHANGELOG.md) 写好这个版本的一节，「设置 › 关于」显示的更新说明就是它；缺这一节 `mygo build` 会停下。
4. 在干净的工作区里打包：

   ```sh
   MYGO_UPDATER_PRIVATE_KEY="$(cat "$HOME/Library/Application Support/mygo/update-keys/MySpotify/mygo-update.key")" \
     go tool mygo build -platform darwin/arm64,darwin/amd64,windows/amd64,linux/amd64
   ```

5. 在 GitHub 上建 `v<版本>` 的 Release，先存成草稿，把附件都传上去，确认后再发布并设为 Latest。应用读的是最新 Release 里的清单，草稿对它不可见，不会出现清单已经能读到、压缩包还没传完的情况。
   - `build.noindex/darwin-arm64/`：dmg（改名为 `MySpotify-<版本>-macos-arm64.dmg`）、`vinyl-<版本>-darwin-arm64.tar.gz`、`*.delta`、`update-darwin-arm64.json`
   - `build.noindex/darwin-amd64/`：dmg（改名为 `MySpotify-<版本>-macos-amd64.dmg`）、`vinyl-<版本>-darwin-amd64.tar.gz`、`*.delta`（有旧版时）、`update-darwin-amd64.json`
   - `build.noindex/windows-amd64/`：把这个文件夹里的程序文件和一份 `README.txt` 放进 `MySpotify-<版本>-windows-amd64/` 打成同名 zip；再加上 `vinyl-<版本>-windows-amd64.tar.gz`、`*.delta`、`update-windows-amd64.json`

Linux 附件包括 `.deb`、`.tar.gz`、`update-linux-amd64.json` 及 `install-linux-webview.sh`；安装脚本也随程序资源附带。依赖组件需普通桌面用户单独安装，不在安装 deb 时自动下载。

`*.delta` 是从前几个版本到这一版的增量包，`mygo build` 会读已发布的版本自动做出来，应用更新时优先下载它。

公钥写在 `mygo.json` 的 `updates` 里。私钥只放在发版的那台 Mac 上，权限 600，不进仓库。私钥丢了，已经装上的应用就收不到后续更新；被别人拿到，他就能给所有用户推任意代码。一定要另外在密码管理器里存一份。

日志（含崩溃时的堆栈）写在设置文件夹里的 `vinyl.log`，超过 1MB 挪成 `vinyl.log.1` 重新开始。

### 调试

- `NM_SHOTS=<目录>`：走一遍首页、排行榜、歌单、播放页、待播清单、搜索，每步截图（静音）。
- `NM_SHOTS=<目录> NM_SCENE=once`：静音放排行榜里的一首歌 10 秒（`NM_PLAY=<歌单/专辑/播客 URI>` 换成放那张列表的第一首），只要一次许可证，打印播放状态和缓存大小。验证真播放先用它。加 `NM_VIDEO=1` 放有视频的那首的视频（MV、视频播客），停在正在播放页。
- `NM_SHOTS=<目录> NM_SCENE=play`：完整验证播放（起播、拖到结尾接下一首、暂停继续、下一首），要好几次许可证，别连着跑。
- `NM_SHOTS=<目录> NM_SCENE=readme`：用真数据截 README 里的这些图，账号名显示成 Alex（首页标题里的也换），不播放；再跑 `python tools/readme_shots.py <目录> docs/screenshots` 出圆角投影的 WebP。这个场景退出时会写设置和播放会话，想不碰自己的就把 `APPDATA` 指到别的目录再跑。
- `NM_SHOTS=<目录> NM_SCENE=connect`：Connect 注册探针，不放音频，结束后注销测试设备。
- `NM_SHOTS=<目录> NM_SCENE=identity`：把当前登录身份导出成 `identity.json`，给下面的 LIVE 测试用。
- `NM_SHOTS=<目录> NM_SCENE=renewal`：只验证登录迁移和续期，不导出身份、不请求媒体许可证；输出 `identity ready true` 后退出。
- `LIVE=<identity.json> go test -run TestLive ./internal/spotify`：用真身份把每个接口打一遍，只读。
- `LIVE=<identity.json> LIVE_EDIT=1 go test -run TestLiveEdit ./internal/spotify`：在这个账号上建歌单、加歌、删歌、改名、删歌单，收藏专辑/歌单/播客、点喜欢、关注艺人，每步读回来核对，最后全部恢复原样。会真改账号。
- `LIVE=1 go test -run TestLiveScriptHashes ./internal/spotify`：检查现在用的查询哈希是否都还能在线上脚本里找到。

不带 `-H windowsgui` 编出来的版本能在控制台看到这些场景的输出。

## 官网

官网从 `main` 分支的 `docs/` 发布，不需要构建。`docs/index.html` 是中文，`docs/en.html` 是英文，两页结构一样，共用 `docs/style.css` 和 `docs/site.js`。改功能说明时同步改两个页面和两份 README（`README.md`、`README.en.md`）。
