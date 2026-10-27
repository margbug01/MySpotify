<div align="center">

<img src="docs/icon.png" width="112" alt="MySpotify">

# MySpotify

Windows 上的 Spotify 播放器：Go 写的原生界面，免费账号也能整首播放。

![Windows 10/11](https://img.shields.io/badge/Windows-10%20%7C%2011-3a6ea5?style=flat-square) ![Go](https://img.shields.io/badge/Go-1.27-00add8?style=flat-square) ![MyGo](https://img.shields.io/badge/UI-MyGo-6a717c?style=flat-square) ![AGPL-3.0](https://img.shields.io/badge/license-AGPL--3.0-c2410c?style=flat-square)

<img src="docs/screenshots/hero.webp" width="860" alt="首页，浅色和深色">

</div>

> [!IMPORTANT]
> MySpotify 是个人项目，和 Spotify 没有任何关系，也没有得到它的认可。它用的是 Spotify 网页播放器自己的内部接口，不是公开 API，Spotify 随时可能改掉；这样用 Spotify 很可能不符合它的用户条款，账号有被限制的风险。只用你自己的账号，后果自负。详见下面的[「风险和边界」](#风险和边界)。

## 为什么用它

**像个桌面软件。** 界面是 [MyGo](https://github.com/egoist/mygo) 画的原生界面，不是套一层网页：侧栏、播放栏、全屏播放页、逐行滚动的歌词、律动舞台、系统媒体键、浅色和深色主题都有。只浏览时整个程序约 190MB，放歌时多一个约 60MB 的解密进程。

**免费账号整首放。** 登录走 Spotify 官方的登录页，MySpotify 只是借用网页播放器拿到的身份。音频由 WebView2 自带的 Widevine 模块解密，和你在浏览器里听是同一条路，128k AAC。

**Spotify 有的大部分都在。** 首页推荐和每日推荐、排行榜、浏览全部分类、歌单和专辑、艺人页、单曲页（制作人员、播放次数、相似歌曲）、个人主页、播客、搜索（歌曲、艺人、专辑、歌单、播客）、喜欢、关注、收藏、自建歌单的增删改、下一首播放和待播清单、从歌曲或艺人开电台、最近播放。

**出错不会卡死。** 许可证过期就自动换身份重试；被 Spotify 限流时播放栏会写「Spotify 暂时限流，稍后重试」，按它要求的时间等完再从原位置接着放；网络或解码出错会先在页面里重试两次。Spotify 更新网页播放器、旧的查询失效时，MySpotify 会自己去线上脚本里找新的。

**听过的不用再下。** 放过的歌（加密分段，原样存下）留在本机，再放更快、不耗流量；上限可以在设置里选，也能一键清除。

**数据只在本机。** 设置、播放进度、本机播放记录在 `%APPDATA%\Vinyl`，登录态在 `%LOCALAPPDATA%\Vinyl\WebView2`。除了 Spotify 和 Gravatar（取账号邮箱的头像），MySpotify 不连别的服务器，也没有统计和自动更新。

## 截图

<table>
  <tr>
    <td width="50%"><img src="docs/screenshots/playing.webp" alt="播放页和歌词"></td>
    <td width="50%"><img src="docs/screenshots/artist.webp" alt="艺人页"></td>
  </tr>
  <tr>
    <td><img src="docs/screenshots/search.webp" alt="搜索"></td>
    <td><img src="docs/screenshots/browse.webp" alt="浏览全部"></td>
  </tr>
  <tr>
    <td><img src="docs/screenshots/podcast.webp" alt="播客"></td>
    <td><img src="docs/screenshots/track.webp" alt="单曲页"></td>
  </tr>
</table>

<details>
<summary>更多截图</summary>

<table>
  <tr>
    <td width="50%"><img src="docs/screenshots/album.webp" alt="专辑"></td>
    <td width="50%"><img src="docs/screenshots/category.webp" alt="分类页"></td>
  </tr>
  <tr>
    <td><img src="docs/screenshots/profile.webp" alt="个人主页"></td>
    <td><img src="docs/screenshots/playing-dark.webp" alt="播放页，深色"></td>
  </tr>
</table>

</details>

## 开始使用

1. 需要 Windows 10 或 11，以及 [WebView2 运行时](https://developer.microsoft.com/microsoft-edge/webview2/)（Windows 11 自带，Windows 10 大多也已经装了）。
2. 按下面「从源码构建」编出 `MySpotify.exe`，双击打开。
3. 点「登录 Spotify」，在弹出的 Spotify 官方登录页里登录。登录态会留在本机，下次打开不用再登。

快捷键：空格 播放/暂停，Ctrl+← / Ctrl+→ 上一首/下一首，Ctrl+D 喜欢正在放的歌，Ctrl+L 打开播放页，Ctrl+F 搜索，Ctrl+N 新建歌单，Ctrl+, 设置，鼠标侧键前进后退。

## 风险和边界

- **不是官方客户端。** 数据走网页播放器的 Pathfinder GraphQL 和 spclient 接口，这些都没有公开文档，Spotify 改了 MySpotify 就会坏。
- **条款。** Spotify 的用户条款不允许用第三方程序访问它的服务。用 MySpotify 可能导致账号被限制或封禁。只用你自己的账号，别拿它做任何商业用途。
- **广告。** 网页播放器的免费版会插广告，MySpotify 不放。这一点 Spotify 很可能不喜欢；如果你在意，请用官方客户端或订阅 Premium。
- **不碰密钥。** MySpotify 不破解 DRM：解密在 WebView2 的 Widevine 模块里完成，程序拿不到密钥，也拿不到解密后的音频。缓存里存的是 CDN 给的加密分段，每次播放仍要向 Spotify 申请许可证，不能离线。
- **歌词来源。** 补歌词用的网易云接口没有公开文档，频繁搜索会被它限流，哪天也可能改掉；它和 LRCLIB 都只收到歌名、歌手、专辑和时长。
- **别频繁起播。** 许可证接口按设备限流，短时间里反复起播（比如半小时十几次）会被 429 一阵子。

## 做不了的

| 功能 | 为什么 |
| --- | --- |
| 256k / 无损音质 | 免费号申请 256k 的许可证会被拒（403）；无损只在官方桌面和手机 App 里 |
| 逐字歌词 | 网页播放器的身份只拿得到逐行时间 |
| 离线下载 | 需要 Premium，而且要官方客户端 |
| Spotify Connect（遥控别的设备）、Jam、DJ | 需要 Premium 或手机端 |
| 视频播客 | 只能放音频 |
| 频谱 | 音频在解密模块里，拿不到；播放中的小音柱是按位置算的动画 |
| 好友动态、本地文件、Blend | 暂时没做 |

## 常见问题

<details>
<summary>为什么点了播放显示「这首歌暂时无法播放」？</summary>

这首歌在你所在地区不可播，或者 Spotify 拒绝了许可证（403）。同一首在网页版也放不了的话，就是版权问题。日志在 `%APPDATA%\Vinyl\vinyl.log`，可以看到具体原因。
</details>

<details>
<summary>播放栏写「Spotify 暂时限流，稍后重试」怎么办？</summary>

什么都不用做。MySpotify 会按 Spotify 要求的时间等完，再从刚才的位置接着放。限流对象是 MySpotify 这个设备，浏览器里的网页版不受影响。
</details>

<details>
<summary>「每日推荐」是空的？</summary>

新账号要听一阵，Spotify 才会生成 Daily Mix，在那之前这一页没有内容。首页照 Spotify 网页版排：筛选（全部/音乐/播客）、最近听过的快捷方块，下面是 Spotify 给的各个分区，标题也用它的。
</details>

<details>
<summary>为什么还要联网，缓存了不是可以离线吗？</summary>

缓存的是加密的音频，解密要用 Spotify 当场发的许可证。缓存省的是流量和缓冲时间。
</details>

<details>
<summary>代理怎么设？</summary>

MySpotify 用 Windows 的系统代理（设置 › 网络和 Internet › 代理），环境变量 `HTTPS_PROXY` 优先。
</details>

<details>
<summary>怎么退出登录或者换账号？</summary>

左下角头像 › 退出登录，会清掉 WebView 里的 Spotify 登录态。
</details>

<details>
<summary>关掉窗口怎么还在放歌？怎么真正退出？</summary>

关窗口只是收进托盘，音乐照常放，点托盘图标或者再打开一次 MySpotify 就回来。要退出：右键托盘图标 › 退出 MySpotify。
</details>

播放 WebView 只在首次实际加载歌曲时创建。暂停 30 秒后停止继续加载分段，连续闲置 5 分钟后保存播放位置并关闭播放窗口、本地服务和命令循环；再次播放时重新加载并恢复位置，可能需要重新申请许可证。短暂停与切歌不会反复重建播放环境。缓存管理独立于播放窗口，未播放时也能查看或清理缓存。Windows 的 WebView2 使用软件渲染，减少隐藏音频页面的 GPU 开销；临时登录页共用这个设置，原生 MyGo 主界面仍保留 GPU 加速。

收藏列表会在切回主窗口时检查更新（5 秒内不重复请求），并每 30 秒后台检查一次。Spotify 网页或其他设备新增、删除、收藏的歌单以及关注的艺人会随列表更新；这是轮询而非实时推送，网络失败时保留上次列表，下次再试。这里只同步收藏列表，不会自动替换当前播放队列或重新加载已打开歌单里的歌曲。

歌词按顺序找，有时间轴的先用：Spotify 自己的同步歌词 → [网易云音乐](https://music.163.com/)（非官方网页接口，不登录；按歌名和主艺人搜索，取同名、时长差不超过 2 秒里最接近的一首，它的时长和 Spotify 基本精确到毫秒；文字是简体）→ [LRCLIB](https://lrclib.net/) 精确查询（歌名、主艺人、专辑、时长都要对上）→ LRCLIB 搜索同名同歌手、时长差不超过 2 秒的带时间轴版本 → 都没有时间轴才显示纯文本（静态显示，不可点击跳转）。只发送歌名、主艺人、专辑和时长，不发送 Spotify 凭据。Connect 给的歌手名是外文拼写或缺失时，先换成 Spotify 的中文名再查。站点忙或限流（LRCLIB 503、网易云 405/406）时隔 3 秒、10 秒各再试一次；当前这首的歌词到手后预取下一首。不做模糊猜配，都没有或版本对不上仍显示暂无歌词，播客不查询。

### Spotify Connect（实验性）

登录后自动注册 `MySpotify · 电脑名`，底部音量旁的「设备」可选择同账号在线的手机、网页或本机。设备播放时，MySpotify 显示远端歌曲与进度，播放/暂停、上一首/下一首、拖动进度、音量及随机/循环操作发给活动设备；转回本机则由原有 Widevine 播放。播放转走或接收连接丢失时暂停本机，避免两个端争抢声音。这里是单活动设备的接力，不是多设备同时出声。

协议结构参考 [Flutify](https://github.com/is-hp-is-mad/Flutify)：两个独立 Dealer 长连接，`connect-state` 负责观察/遥控，`track-playback` 注册本机并处理状态机。不使用 Premium Web Playback SDK。免费账号的双通道注册已实测通过；手机/网页与 MySpotify 的完整音频接力、长时间重连仍需实机验证，不能把注册成功等同于完整兼容。状态机只带当前和下一首，待播清单因此按 connect-state 的 `next_tracks` 显示，与手机一致（隐藏的循环副本不显示，只有 URI 的曲目用 `decorateContextTracks` 补全）；点其中一首发 `skip_next` 带该曲 uid，在原上下文里跳过去。本机插队和电台续队尚未接入 Connect；不支持的内容类型会暂停并提示，不会擅自越过。

`NM_SHOTS=<已有目录> NM_SCENE=connect` 运行不播放音频的注册探针，结束后注销测试设备；使用不带 `-H windowsgui` 的构建查看输出。离线测试覆盖双连接注册/注销、推送解码、远端状态与控制路由、控制权释放、设备菜单以及本机音频停止。

## 怎么做到的

- **登录**：一个平时隐藏的窗口打开 open.spotify.com，`internal/spotify/capture.js` 记下网页播放器自己拿到的 token 和 client-token；没登录就把这个窗口显示成 Spotify 的登录页。token 快过期时在后台重新加载一次。
- **数据**：Pathfinder GraphQL 的持久化查询（哈希在 `internal/spotify/spotify.go` 的 `ops` 里）加 spclient（歌词、播放清单、歌单改动、电台、最近播放、个人主页）。公开 Web API 对这个身份一律 429，不用。哈希失效时（412「Invalid query hash」）`internal/spotify/ops.go` 重读主脚本和按需加载的分块脚本，换上新哈希重试一次，存到 `%APPDATA%\Vinyl\queries.json`，10 分钟内不重复读。
- **播放**：另一个隐藏的 WebView 页面（`internal/player/web/player.html`）用 hls.js + Widevine 播加密的 MP4。清单、分段和许可证请求都经本机 127.0.0.1 的小服务转发，分段在这里顺手存进缓存，键是 CDN 路径加字节范围（CDN 的签名每天变，不进键）。播客的单集和歌曲走同一条路。
- **出错**：许可证 401 换身份再要一次；429 按 Retry-After 等（没给就从 30 秒起翻倍），等完从原位置重来；403 停在这首。页面里的网络错误（`startLoad`，清单出错就重新 `loadSource`）和媒体错误（`recoverMediaError`）每首各重试 2 次，不行再跳下一首。
- **头像**：账号的头像依次用 Spotify 上设的、账号邮箱在 Gravatar 上的、名字首字母（底色用 Spotify 给的颜色）。邮箱是带着 WebView 的 cookie 问 `www.spotify.com/api/account-settings/v1/profile` 拿的。

## 从源码构建

需要 Go 1.27.3 以上，不需要 cgo。

```
go test ./...
CGO_ENABLED=0 go build -trimpath -ldflags="-H windowsgui -s -w" -o outputs/MySpotify.exe .
```

图标都由 `tools/icons.py` 画（Windows，要 Pillow，用 Edge 无头模式渲染 SVG）：`resources/icon.svg`/`icon.png` 是程序图标，`icon-small.svg` 是 16–24 像素用的简化版（两道纹、更粗），`icon.ico` 把两者按尺寸合在一起；`internal/app/tray.png` 是托盘和 Mac 菜单栏的模板图标（只有黑色和透明，Windows 深色任务栏上自动画成白色，macOS 自己染色）；`AppIcon.icon` 是 Mac Dock 用的 Icon Composer 文档（浅色/深色两种底，`mygo.json` 的 `macos.icon`，要在装了 Xcode 26 的 Mac 上 `mygo build` 才会编进去，否则退回 `icon.png` 生成的 `.icns`）。改了图标先跑 `python tools/icons.py`，再重新生成图标、版本信息和 manifest 所在的 `rsrc_windows_amd64.syso`：

```
go run github.com/tc-hib/go-winres@v0.3.3 simply --icon resources/icon.ico --manifest gui --product-name MySpotify --file-description MySpotify --product-version 0.1.0 --file-version 0.1.0 --original-filename MySpotify.exe --arch amd64 --out rsrc
```

日志（含崩溃时的堆栈）写在 `%APPDATA%\Vinyl\vinyl.log`，超过 1MB 挪成 `vinyl.log.1` 重新开始。

### 调试

- `NM_SHOTS=<目录>`：走一遍首页、排行榜、歌单、播放页、待播清单、搜索，每步截图（静音）。
- `NM_SHOTS=<目录> NM_SCENE=once`：静音放排行榜里的一首歌 10 秒（`NM_PLAY=<歌单/专辑/播客 URI>` 换成放那张列表的第一首），只要一次许可证，打印播放状态和缓存大小。验证真播放先用它。
- `NM_SHOTS=<目录> NM_SCENE=play`：完整验证播放（起播、拖到结尾接下一首、暂停继续、下一首），要好几次许可证，别连着跑。
- `NM_SHOTS=<目录> NM_SCENE=readme`：用真数据截 README 里的这些图，账号名显示成 Alex，不播放。
- `NM_SHOTS=<目录> NM_SCENE=identity`：把当前登录身份导出成 `identity.json`，给下面的 LIVE 测试用。
- `LIVE=<identity.json> go test -run TestLive ./internal/spotify`：用真身份把每个接口打一遍，只读。
- `LIVE=<identity.json> LIVE_EDIT=1 go test -run TestLiveEdit ./internal/spotify`：在这个账号上建歌单、加歌、删歌、改名、删歌单，收藏专辑/歌单/播客、点喜欢、关注艺人，每步读回来核对，最后全部恢复原样。会真改账号。
- `LIVE=1 go test -run TestLiveScriptHashes ./internal/spotify`：检查现在用的查询哈希是否都还能在线上脚本里找到。

## 致谢

- 界面来自 [egoist/mygo-cloudmusic](https://github.com/egoist/mygo-cloudmusic)（一个网易云音乐客户端），数据和播放换成了 Spotify。仓库保留了上游历史，可以 `git merge upstream/main` 拉作者的界面更新。
- [MyGo](https://github.com/egoist/mygo)：Go 的原生界面框架。
- [hls.js](https://github.com/video-dev/hls.js)（Apache-2.0），在 `internal/player/web/hls.min.js`。

## 许可证

[AGPL-3.0](LICENSE)，随上游。图标是 MySpotify 自己画的（`tools/icons.py`，「转动的唱片」：一张黑胶，三道纹路接住光）。Spotify 是 Spotify AB 的商标，截图里的封面和照片版权归各自的权利人。
