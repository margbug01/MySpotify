# Linux 发版（实验性）

当前源码版本为 0.4.16，不代表已发布 Linux 版本。优先 amd64；MyGo 也支持 arm64。以下 2026-11-02 的记录来自 0.4.5，当前版本的复测另行记录。
现有平台分支已经能编译 Linux。Ubuntu 22.04 / WSLg 已验证原生窗口，以及可选 Qt WebEngine 后端的已有登录迁移、重启续期和真实加密音乐播放。从 0.4.16 起 amd64 默认选择 Qt；原有 WebKitGTK 在该环境中没有所需 EME 接口；Linux 仍为实验支持，完整桌面验收尚未完成。

## 构建与交付

在仓库根目录使用 go.mod 指定的 Go 工具链和 MyGo（不要单独安装最新版）：

```sh
go test ./internal/...
go tool mygo build -platform linux/amd64
# 可选：同时构建两种架构
go tool mygo build -platform linux/amd64,linux/arm64
```

不需要在 macOS 上安装 GTK 开发包或交叉 C 编译器。CLI 使用 Linux 后端、纯 Go 动态库绑定跨编译，再直接生成 Debian 包，不要求宿主安装 dpkg-deb。不要添加 `-upload`。

输出在 `build/linux-amd64/`（arm64 对应 `linux-arm64`）：

- `vinyl`、`vinyl.png`、`vinyl.desktop`，以及 LICENSE 和 licenses；
- `vinyl_0.4.16_amd64.deb`；
- `vinyl-0.4.16-linux-amd64.tar.gz`；
- `install.sh`、Arch Linux 的 `PKGBUILD`，以及输出根目录的 `SHA256SUMS`。

`mygo.json` 的 Linux 配置声明 AudioVideo/Audio/Player 分类、`vinyl` 命令和 WebView 依赖。Debian 包自动依赖 `libgtk-3-0 | libgtk-3-0t64` 和 `libwebkit2gtk-4.1-0`。本项目登录、音频和视频都用 WebView，不能为减少依赖设为 false。其他发行版需自行准备 GTK3、WebKitGTK 4.1 及其字体、图形和媒体运行库。此处不承诺所有发行版兼容，也不提供 AppImage/RPM。

在 Debian/Ubuntu 桌面本机可用 `sudo apt install ./vinyl_0.4.16_amd64.deb` 安装。非 Debian 环境可解压 tar.gz，保留同目录资源后运行 `./vinyl`；压缩包不包含 GTK/WebKitGTK。`install.sh` 是 MyGo 生成的安装器，执行前应检查内容和下载来源，不在此自动执行。

## 运行能力和限制

| 部分 | 当前实现 / 验收重点 |
| --- | --- |
| 原生 UI | MyGo `internal/linux` 的 GTK3 后端；需图形桌面会话，跨编译不能验证 X11/Wayland、字体、GPU。 |
| 音频 / DRM | `internal/player/drm_other.go` 使用 `com.widevine.alpha`；Linux WebView 是 WebKitGTK，不是 Windows 的 Edge/WebView2。普通发行版的 WebKitGTK 不应被当作已带可用 Widevine 的 Chromium；必须实际验证 EME key-system、许可证交换和持续播放。安装 GStreamer 编解码器或 Chrome 不等于给嵌入式 WebKitGTK 增加 Widevine。若运行时不支持该 key system，本机加密音乐无法播放，需要框架/媒体栈提供可用 CDM 后才能宣称完整支持；本适配不捆绑、下载或绕过 DRM。 |
| 播客 / MV | 仍由 WebKitGTK 的 HTML media / MediaSource 解码，受系统媒体插件、格式和 DRM 支持限制，不能因 UI 能打开就标记通过。 |
| 登录 / 凭据 | 沿用非 macOS 的 UA 和登录/token 分支，实际 WebKitGTK 登录、续期、重启后恢复需验证。`internal/app/seal_other.go` 明确拒绝额外 cookie 副本的加密保存，不会降级为明文；WebView 自身 cookie 与额外加密副本是两回事，不承诺 Linux 已有 DPAPI/Keychain 等价物。 |
| 代理 | `internal/spotify/proxy_other.go` 使用 `http.ProxyFromEnvironment`，Go 请求读取 HTTP_PROXY、HTTPS_PROXY、NO_PROXY（及对应小写形式）；WebKitGTK 自有网络栈不能据此视为同步配置完成。分别验证登录页、API、歌词、音频/许可证，在需要代理的环境尤其如此。 |
| 托盘 | MyGo `internal/linux/menu.go` 用 AppIndicator / StatusNotifierItem，依赖桌面宿主与指示器库；GNOME 等环境可能需要扩展。创建托盘失败时现有 `internal/app/app.go` 会在关窗时退出而非隐藏；创建成功但宿主不展示图标的情况仍需实测，可先开启“关闭时退出”。 |
| 后台 / awake | `internal/player/awake_other.go` 是 no-op，不具备 macOS WebKit 的专用保活，也不代表阻止系统休眠。测试最小化、隐藏、锁屏、休眠恢复，勿承诺后台不断播。 |
| 系统媒体 / 更新 | Linux 后端与已有更新配置沿用；需实测媒体键、桌面媒体控制、退出和重启。CLI 未配置私钥时明确跳过更新签名，普通包可生成；无签名更新产物不得当作已支持自动更新发出。不要在代码或文档中保存私钥。 |

## 可复核依据

当前模块固定为 `github.com/egoist/mygo v0.8.1`；旧实测记录使用的模块为 `v0.6.5-0.20261025230045-be164835449b`。核对实际模块以 go.mod 为准：

```sh
go list -m github.com/egoist/mygo
go tool mygo build -h
go env GOMODCACHE
```

在模块目录检查 `cmd/mygo/build.go`（Linux desktop/tar/deb 打包）、`cmd/mygo/deb.go`（Linux 配置、架构与依赖）、`internal/linux/webview.go` 和 `internal/linux/ffi.go`（WebKitGTK 动态加载），以及 `internal/linux/menu.go`（托盘）。框架确实支持 Linux，不存在“整个 MyGo 不支持 Linux”的阻碍；真正未确认的是 Spotify 所需的运行时媒体能力。

## Ubuntu WSLg 实测记录

2026-11-02，在 MSI 的 Ubuntu 22.04 x86_64 / WSL2 + WSLg 上重新构建并启动 0.4.5：

- 安装 WebKitGTK 4.1（`libwebkit2gtk-4.1-0` 2.50.4）、GStreamer good/bad/libav、字体及图形观察工具；GTK3 3.24.33 已有。没有安装 deb 到系统，运行的是解压后的 tar.gz。
- 未发现普通登录用户，建立 `myspotify-test`（uid 1000），没有更改 WSL 默认 root 登录。用独立 HOME 和 `~/myspotify-isolated/{config,data,cache,runtime}`，未读取或复制其他 Spotify 登录数据，未设置禁用 WebKit 沙箱的变量。
- 使用 X11 和 `LIBGL_ALWAYS_SOFTWARE=1`；测试用户无 `/dev/dri` 访问权，日志确认原生 UI 走 llvmpipe。已通过 X11 窗口树及窗口截图确认 MySpotify 界面和“登录 Spotify”按钮真实出现，不是仅有编译结果。
- 主界面显示“连不上 Spotify”；曾产生 WebKit 的 `WebLoaderStrategy.cpp(618)` 内部错误。curl 能访问 accounts/open.spotify.com 不能证明嵌入式 WebKit 网络、登录或令牌获取正常。未操作嵌入式网页登录，交由用户从测试窗口点击登录继续。
- WebKitNetworkProcess 与主程序均为 uid 1000；观察到网络进程 Seccomp=2，但这不是完整 WebKit 沙箱审计。托盘缺少 libayatana-appindicator3，portal 报 PipeWire/RealtimeKit 不可用；这些能力未验收。
- 测试目录是 Windows `F:\AA\d\MySpotify-Linux-test`（WSL `/mnt/f/AA/d/MySpotify-Linux-test`），保留包、资源、隔离启动脚本、日志和截图。可用 `wsl -d Ubuntu -- bash /mnt/f/AA/d/MySpotify-Linux-test/start-isolated.sh` 重开；保持该终端运行。登录、续期、音乐/DRM、播客/MV及 Wayland 均未通过验收。

## 登录后播放故障复核

2026-11-02，用户已在 MSI 测试实例登录，但歌曲持续缓冲并自动跳歌：

- 原 PID 16 在调查开始时已不存在，无法再从 `/proc/16/fd/1` 证明它的标准输出路径。现有 `interactive-app.log` 中发现 18 条 `player: track N: keySystemError keySystemNoAccess`，最后一条为 13:51:48、track 18。该错误表示 hls.js 无法取得所请求的 EME key-system access，尚不是许可证 HTTP 错误或音频解码失败；不能据此断言代理的所有媒体请求都已正常。
- 普通歌曲非 macOS 分支请求 cenc AAC（`file_ids_mp4` / `file_ids_mp4_dual`）；MV 非 macOS 分支也生成 Widevine PSSH/KEYID 的 cenc MP4 清单。两者均需要 `com.widevine.alpha`。播客直连文件走 `kind=file` 原生媒体分支，不应被一概判成 DRM 故障，尚未验证真实播放。
- 远端已安装 WebKitGTK 2.50.4、GStreamer bad/libav 1.20.3。安装这些解码器并未解决实际 key-system access 拒绝。当前 WebKitGTK 实例不能提供 Spotify 请求的 Widevine 能力，不能宣称 Linux 原生加密音乐可用；没有下载 CDM、绕过 DRM 或禁用沙箱。
- 最小修复：key-system access / EME 不可用时停止自动播放并保留 DRM 提示，MV 同类错误不再回退到同样需要 Widevine 的音频；其余连续失败最多三首后停止，401/403 停止和 429 退避保持不变。回归测试经真实 `/event` → 输出错误 → 队列回调链验证，而非只检查提示文案。
- 已只替换隔离测试目录中的 `vinyl`，未改 Windows 版或登录数据；恢复后从进程 fd 确认 stdout/stderr 均为 `/mnt/f/AA/d/MySpotify-Linux-test/playback-fix-runtime.log`。无鼠标截图确认个性化首页恢复，登录仍在。真实音频输出及修复后的人工点播尚未验收；不能用重开窗口或旧日志错误数量代替新包的实际播放验证。

## EME 独立探测与后端选择

2026-11-02，以 `myspotify-test` 运行独立 WebKitGTK 4.1 测试页（不加载应用凭据），显式设置 `enable-encrypted-media=true`，页面基地址 `http://localhost/`：`isSecureContext=true`，但 `typeof navigator.requestMediaKeySystemAccess` 为 `undefined`。因此本机结果不只是 hls.js 请求参数不匹配或默认设置未开启；给当前应用打开该设置不足以修复播放。此结论限定于实测的 Ubuntu WebKitGTK 构建，不代表所有 WebKit 派生产品。

最小验证路径是先在同一 Linux 环境测试正式发行的、支持 Widevine 的浏览器，先验证 EME access，再验证完整 Spotify 播放，均通过后才接入应用。Google 官方平台表列出 Chrome/Linux、Chromium、CEF/Electron，但这不意味着任意 Chromium/CEF 构建已含获授权且可用的 CDM；不能直接从另一个浏览器复制 CDM 到 WebKit。参考 https://developers.google.com/widevine/drm/overview 。

现有音频页已经通过本机 HTTP 服务接收媒体、许可证并上报 `/event`；控制命令仍由 MyGo `Page.EvalContext` 注入。因此外部浏览器原型可以复用服务层，但需要替换命令通道、管理进程生命周期和隔离页面访问；不是把 URL 用浏览器打开就能直接工作。MV 还需要单独处理内嵌视图，不能把外部音频原型当作完整 MV 支持。当前不切换默认后端，也不新增未经播放验证的浏览器运行依赖。

已增加仅 Linux、显式 `MYSPOTIFY_LINUX_CHROME=1` 启用的音频原型，要求系统已有 `google-chrome-stable`。Chrome 使用独立临时 profile 和 headless 模式，不导入登录、不关闭沙箱；媒体与许可证仍由应用已有服务提供。控制通道仅监听原有 loopback 服务，使用每次启动随机 256-bit key 鉴权，key 通过 fragment 传入后立即从页面地址移除；退出时清理 Chrome 和临时 profile。从 0.4.16 起 Linux amd64 默认使用下文的 Qt 后端；其他平台保留原有后端。此原型不提供 Linux MV 内嵌播放支持。

同台 Ubuntu 的官方 Chrome 156.0.8078.79 已以非 root、沙箱开启的 headless 模式实际取得 `com.widevine.alpha` access（cenc AAC、temporary session）；这证明 CDM 能力可用，但不等于 Spotify 许可证请求和真实音频输出已通过。随后使用项目现成的 `NM_SCENE=once` 与 `NM_SCENE=play`，在 Xvfb 独立显示下进行静音端到端测试（`MYSPOTIFY_LINUX_CHROME=1`，沿用隔离实例的登录）。`once` 中实际播放进度从 0 增至 7.5 秒；`play` 中跳至曲尾后收到真实 media ended，自动续播下一首；暂停 3 秒位置保持 3.8 秒，恢复后推进至 6.9 秒；手动下一首和身份续期后的下一首也正常推进，日志无播放器错误。Chrome profile 在退出后清理，无需 WSLg 窗口投递即可复现。测试为静音、短时抽样加曲尾跳转，不是整首自然播放或实际有声音频输出验收；MV、长时间播放及原生 Linux 桌面仍未验收。日志位于 MSI 测试目录的 `chrome-playback-selftest.log` 和 `chrome-transport-selftest.log`。

## Chrome 实验音频后端的使用

系统已安装官方 Google Chrome（命令为 `google-chrome-stable`）后，显式开启原型：

```sh
MYSPOTIFY_LINUX_CHROME=1 ./vinyl
```

也可在现有隔离启动器前设置此变量。显式设置 `MYSPOTIFY_LINUX_WEBVIEW=webkit` 可使用旧后端，但已测的 Ubuntu WebKitGTK 无法播放 Widevine 加密音乐。缺少 Chrome 时音频后端会启动失败，应用仍可浏览内容；浏览器需自行安装。

Chrome 异常退出会停止当前歌曲并保留位置，提示点击播放重试；重试重新启动独立 Chrome 进程。该路径不支持内嵌 MV，仍需手动关闭 MV。当前不将其设为默认后端。

## 2026-11-09 当前版本复测

使用源码快照 `c9f2d11`（0.4.14、MyGo 0.8.1、Go 1.27.3），在同台 Ubuntu 22.04.1 / WSL2、官方 Chrome 156.0.8078.79 下，以原有 `myspotify-test` 非 root 用户、沙箱开启、独立 Xvfb 显示运行新包：

- `NM_SCENE=once`：真实歌曲位置由 0 推进至 7.6 秒，无播放器错误。
- `NM_SCENE=play`：从曲尾前 5 秒收到真实 `media ended` 并续播下一首；暂停 3 秒维持 4.4 秒，恢复后推进至 7.4 秒；手动下一首、强制 Spotify identity 过期后续期并切歌均正常推进。两个场景正常退出，未留下本次 Chrome 临时 profile。
- 新增 Chrome 异常退出处理：故障发生在打开音轨前、注册回调前或正在播放时，都能停止等待；播放队列停在原曲，重试重建音频后端并恢复位置。控制鉴权的共享状态也已加锁。
- `go test ./internal/...`、`go test -race ./internal/player` 通过；Linux 专用进程正常退出/崩溃清理、晚到音轨、鉴权、播放重试及连续失败回归在真实 Linux 非 root 用户下运行 5 次，全部通过。子进程故障测试使用替身可执行文件，不等同于真实 Chrome 崩溃后的端到端验收。
- amd64 与 arm64 的 `.deb`、`.tar.gz` 均由干净快照成功生成；arm64 仅验证构建，没有实机运行。

新包与运行日志保存在 MSI 的 `F:\AA\d\MySpotify-Linux-test\2026-11-09`，未覆盖旧测试包。可用 `wsl -d Ubuntu -- bash /mnt/f/AA/d/MySpotify-Linux-test/2026-11-09/run-wsl.sh play` 重跑静音测试。该脚本仅使用已有隔离登录，未改 Windows 版、系统代理或默认播放后端。

这轮仍是短时静音播放与曲尾跳转，未新增整首自然播放、有声音频输出、MV、原生 Linux 桌面或 Wayland 验收。两种架构包均未签署自动更新，也未发布。

## 2026-11-09 内存定位与第一轮优化

原先汇报的 877 MiB PSS 来自 `NM_SCENE=play`：该入口原先保留所有封面像素，不能直接当作正常运行内存。现在 `once` / `play` / `perf` 与 `mem` 一样使用正常封面缓存，截图场景仍保留像素。校正后的最终包遇到 Spotify 429，尚未获得其真实音乐播放总内存，不能用失败或停在 0 秒时的占用替代。

在同一台 WSL / Chrome、同一成功完成的 `play` 场景中，保持封面像素策略一致，仅更换 Chrome 启动方式，得到以下对照（PSS，排除 Xvfb；各行中位数或代表样本不可混合相加）：

| 测量 | 原始启动方式 | 应用模式 |
| --- | ---: | ---: |
| 整组进程中位数 | 881.6 MiB | 791.8 MiB |
| Chrome 代表样本 | 545.8 MiB / 18 进程 | 449.3 MiB / 13 进程 |
| 本地音频 + EME 初始化，Chrome 中位数 | 502.2 MiB | 420.6 MiB |

启动器已改用 `--app=<音频页>`，关闭扩展、后台组件扩展及浏览器后台网络，避免为音频页启动完整浏览器界面和扩展渲染进程。保持沙箱、进程隔离及系统官方 Chrome 的 Widevine 路径。关闭软件光栅、缩小视口和禁用备用渲染进程的附加试验只多省约 18 MiB，未纳入代码。

新增不请求 Spotify 许可证的真实 Chrome 验证：

```sh
# 在有官方 google-chrome-stable 的 Linux 上运行，音频静音。
LIVE_LINUX_CHROME=1 go test ./internal/player -run '^TestLinuxBrowserLocalAudio$' -v
```

它通过现有播放器页面解码本机 WAV，并调用 Widevine 的 key-system access / createMediaKeys；不创建 DRM session、不请求 Spotify 许可证，不能替代完整 Spotify DRM 音乐验收。该测试的新旧启动器对照均通过，音频进度持续推进。全套 `CGO_ENABLED=0 go test ./...`、`go tool mygo vet .` 及最终 Linux 故障回归 5 次通过。

最终代码与双架构包绑定源码快照 `a2682f4`。应用模式在较早的真实音乐完整控制场景中验证成功；最后一次单曲复测在 22:40 仍被 Spotify 429 拒绝，未继续请求。降低缓存后的精确总内存、完整最终包的长时间音乐播放仍待限流解除后验收。

这些改动只减少了部分开销，当前没有达到轻量桌面客户端的内存目标。Linux 方案同时保留登录用 WebKit 与音频用完整 Chrome；若要显著接近 Windows/macOS，需要另行验证支持 Widevine 的轻量宿主，并让登录和播放复用同一个引擎。不能把任意 CEF/Chromium 构建当作已具备可用 Widevine，也不应通过关闭沙箱省内存。

## Qt WebEngine 后端（2026-11-10）

从 0.4.16 起 Linux amd64 默认让登录和音频复用一个 Qt WebEngine 宿主。播放时不启动 Chrome 浏览器，也不需要额外的 WebKit 登录进程；仍通过官方 Chrome 已安装的 Widevine 组件解密。Qt 本身基于 Chromium，并非系统已经自带一个与 WebView2 等价、可直接播放 DRM 的轻量组件。

当前已验收的组合是 Flathub `com.riverbankcomputing.PyQt.BaseApp//6.11`（Qt 6.11.2 / PyQt 6.11.0，含 AAC 编解码支持）和官方 Chrome 156 的 amd64 Widevine。发行版 Qt 5 的 CDM 在该 WSL 环境崩溃，另一套 PySide6 构建缺少 AAC，不能替换成任意 Qt 包便宣称支持。

先安装官方 Google Chrome，使下面的组件路径可读，再以运行应用的普通桌面用户执行安装脚本：

```sh
# 组件路径：/opt/google/chrome/WidevineCdm/_platform_specific/linux_x64/libwidevinecdm.so
sh scripts/install-linux-webview.sh
./vinyl
```

脚本为当前用户安装 Flatpak Qt 运行时；缺少 Flatpak 时在 apt 系统通过 sudo 安装。共享运行时、图形库和语言包首次约需数 GiB 磁盘空间，这与运行内存是两回事。应用仍保留 GTK / WebKitGTK 包依赖，包管理器不会自动运行此脚本。未安装运行时或 CDM 时会明确报错。无需环境变量即可启动 Qt；`MYSPOTIFY_LINUX_CHROME=1` 可切换旧 Chrome 音频方案，`MYSPOTIFY_LINUX_WEBVIEW=webkit` 可切换旧 WebKitGTK。显式 `MYSPOTIFY_LINUX_WEBVIEW=qt` 优先于 Chrome 变量。Qt 路径目前只支持 amd64，arm64 没有此运行时验收。

宿主使用私有标准输入输出通信，媒体页仍通过带随机鉴权的 loopback 服务通信。保留渲染沙箱；没有启用远程调试、单进程或关闭沙箱参数。仅授权读取宿主脚本和 CDM，写入应用专属 profile。Qt Cookie 保存在用户配置目录的 `MySpotify/qt-webengine`；首次从应用原有登录迁移一次，退出登录会清除 Qt 会话并防止再次导入旧会话。重启续期优先让精简 token API 页面使用浏览器自身的 HttpOnly Cookie，不再为此加载完整 Spotify 网页。登录页关闭后释放页面；宿主有使用者时保持运行，最后一个使用者释放后 20 秒回收，应用退出时关闭。

快照 `62f586a` 已在相同 WSL / Xvfb / 软件渲染环境通过已有登录迁移、重启续期和单曲 75 秒真实 DRM 播放，播放位置推进至 1 分 12.9 秒，无播放器错误或 429。Qt 整组进程 PSS 中位数为 654.4 MiB，范围 648.7～660.5 MiB；45 秒后中位数为 658.1 MiB。统计包括本次新增辅助服务，扣除测试用 Xvfb；未出现 Chrome / WebKit 子进程。同快照、同歌曲、同 75 秒时长的 Chrome 方案中位数为 799.4 MiB（780.5～804.2 MiB），Qt 约降低 18.1%；两者均使用正常封面缓存。Qt 宿主本身与 Chrome 音频进程仍各占约 450 MiB，本轮主要省去了第二套登录引擎。此前完整 Spotify 登录页会额外留下约 300～400 MiB 渲染进程，精简续期后已消除该短时峰值。此前汇报的 877 MiB 测量还包含调试封面缓存，不能直接用来计算本轮生产内存降幅。

内部回归、无 CGO 全套测试、Qt / 播放器 / Spotify 并发检查及 MyGo 静态检查通过。真实 Qt 窗口、Cookie 属性持久化、重启后清理会话的回归运行 5 次通过；窗口截图已人工查看。Qt Cookie 事件不回报 SameSite，测试直接核对合成 Cookie 的持久化属性，没有读取用户 Cookie 值。首次手动登录的完整 OAuth、可听音频、整首自然播放、MV、原生 X11 / Wayland、锁屏及长时间运行尚未验收，仍不能称为与 Windows / macOS 相同的轻量方案。

当前源码、运行日志和数据见 `outputs/linux-qt-2026-11-10/VALIDATION.md`。上述测量用的 0.4.14 快照未发布；0.4.16 发布包启用默认 Qt，另行构建并签署更新。

## 发布门槛

先作为实验构建交付测试者，不把 Linux 加入稳定支持矩阵。至少在 Ubuntu/Debian amd64 桌面完成：安装/卸载、启动、登录/续期/退出登录、重启恢复、无代理和代理、完整音乐播放与拖动/切歌、播客/MV、托盘关闭/恢复/退出、媒体控制、隐藏/锁屏后继续播放和日志检查。X11 与 Wayland 分别记录结果，arm64 单独验收；不满足 DRM 则明确记录不可本机播放，不用跨编译成功替代验收。

MyGo 的 `SHA256SUMS` 使用文件名而非架构子目录路径，不能直接在 build 根目录执行校验；应将待发布的 deb、tar.gz、install.sh 与校验文件收集到同一目录后校验。本次逐架构目录核对所有条目均匹配，两个架构的 install.sh 内容相同。

正式发布前人工检查 deb 的 control、desktop、图标、命令链接、许可证，tar.gz 内容和 SHA256SUMS；签名更新需要维护者在安全环境配置原有密钥并独立验收。后续正式发布应在干净提交上构建，核对更新签名和公开下载摘要后再交付。
