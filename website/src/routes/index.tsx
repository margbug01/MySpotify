import { Link, createFileRoute } from '@tanstack/solid-router'
import { For, createSignal, type JSX } from 'solid-js'
import { Layout } from '../components/Layout'
import { AppWindow } from '../components/AppWindow'
import { GitHubMark, Icon, type IconName } from '../components/Icon'
import { ThemeSwitch } from '../theme'
import { mygo, repo } from '../site'

export const Route = createFileRoute('/')({
  component: Home,
})

function Home() {
  return (
    <Layout>
      <Hero />
      <Features />
    </Layout>
  )
}

function Hero() {
  return (
    <section class="hero">
      <div class="grooves" aria-hidden="true" />
      <div class="hero-text">
        <p class="eyebrow">
          <span class="pill">开源</span>第三方网易云音乐桌面客户端
        </p>
        <h1 class="display hero-title">
          <span>原生地，</span>
          <span>安静地听歌。</span>
        </h1>
        <p class="lede">
          用 Go 与 <a href={mygo}>MyGo</a> 写成，原生界面，没有 WebView。启动快，占用少，闲下来就安安静静，不打扰你的电脑。
        </p>
        <div class="actions">
          <Link class="btn btn-primary" to="/download">
            <Icon name="download" size={17} />
            下载
          </Link>
          <a class="btn btn-ghost" href={repo}>
            <GitHubMark size={17} />
            查看源代码
          </a>
        </div>
        <p class="fine">免费 · 以 AGPL-3.0 开源 · 扫码登录你的网易云账号</p>
      </div>
      <div class="hero-window">
        <AppWindow />
        <p class="hint">试试点一下播放，或者喜欢这首歌。</p>
      </div>
    </section>
  )
}

type Feature = { icon: IconName; title: string; body: string; wide?: boolean; extra?: () => JSX.Element }

const features: Feature[] = [
  {
    icon: 'wave',
    title: '边下边播，直到 Hi-Res',
    body: '不必等整首下载完。标准、较高、极高、无损（FLAC）与 Hi-Res，按你的耳朵和网速选。',
    wide: true,
    extra: () => <Quality />,
  },
  {
    icon: 'disk',
    title: '听过的，存在本机',
    body: '听过的歌缓存在本机，再次播放无需等待。',
  },
  {
    icon: 'history',
    title: '从上次停下的地方',
    body: '退出后再打开，从上次的播放队列与进度继续。',
  },
  {
    icon: 'volume',
    title: '温柔的音量',
    body: '音量均衡，暂停与继续时淡出淡入。',
    extra: () => <Fade />,
  },
  {
    icon: 'radio',
    title: '私人 FM 与心动模式',
    body: '推荐、熟悉、探索三种口味；或者开启心动模式，从你喜欢的歌出发。',
  },
  {
    icon: 'calendar',
    title: '每日推荐',
    body: '每天 6:00 更新。还有推荐歌单、排行榜、我的歌单与搜索。',
  },
  {
    icon: 'qr',
    title: '扫码登录',
    body: '用网易云音乐 App 扫一扫。登录态保存在系统凭据存储中。',
  },
  {
    icon: 'notch',
    title: '刘海播放器',
    body: '在屏幕顶部的刘海处显示迷你播放器，指针移过去就展开。在设置中开启。',
  },
  {
    icon: 'lyrics',
    title: '逐字歌词与「律动」',
    body: '逐字高亮与翻译同屏；「律动」视图里，每个字随演唱浮起、发光、落定。还能读歌曲评论。',
    wide: true,
  },
  {
    icon: 'sun',
    title: '浅色、深色、跟随系统',
    body: '和这个网站一样——试试看。',
    extra: () => <ThemeSwitch labels class="feature-theme" />,
  },
]

function Features() {
  return (
    <section id="features" class="section">
      <div class="section-head">
        <p class="eyebrow">功能</p>
        <h2 class="display">
          <span>该有的，</span>
          <span>都在。</span>
        </h2>
        <p class="lede">从发现到播放，每一处都为桌面重新做过。喜欢、播放队列、随机与循环，一样不少。</p>
      </div>
      <div class="features">
        <For each={features}>
          {(f) => (
            <article class="feature" classList={{ wide: f.wide }}>
              <div class="feature-icon">
                <Icon name={f.icon} size={20} />
              </div>
              <h3>{f.title}</h3>
              <p>{f.body}</p>
              {f.extra?.()}
            </article>
          )}
        </For>
      </div>
    </section>
  )
}

const qualities = [
  { name: '标准', spec: '128 kbps · MP3' },
  { name: '较高', spec: '192 kbps · MP3' },
  { name: '极高', spec: '320 kbps · MP3' },
  { name: '无损', spec: '最高 48 kHz / 16 bit · FLAC' },
  { name: 'Hi-Res', spec: '最高 192 kHz / 24 bit · FLAC' },
]

// The settings' choice of quality, as the app shows it.
function Quality() {
  const [q, setQ] = createSignal(3)
  return (
    <div class="quality">
      <div class="segmented" role="radiogroup" aria-label="音质">
        <For each={qualities}>
          {(item, i) => (
            <button
              type="button"
              role="radio"
              aria-checked={q() === i()}
              classList={{ on: q() === i() }}
              onClick={() => setQ(i())}
            >
              <span>{item.name}</span>
            </button>
          )}
        </For>
      </div>
      <p class="quality-spec">{qualities[q()].spec}</p>
    </div>
  )
}

// A wave fading out at a pause and back in, drawn.
function Fade() {
  const bars = Array.from({ length: 44 }, (_, i) => {
    const env = i < 16 ? 1 : i < 22 ? 1 - (i - 16) / 6 : i < 26 ? 0 : Math.min((i - 26) / 6, 1)
    return Math.max(0.06, env * (0.45 + 0.55 * Math.abs(Math.sin(i * 1.7) * Math.cos(i * 0.6))))
  })
  return (
    <div class="fade" aria-hidden="true">
      <For each={bars}>{(h) => <i style={{ height: `${h * 100}%` }} />}</For>
      <span class="fade-mark">暂停</span>
    </div>
  )
}
